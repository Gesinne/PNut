// sai-monitor: puente de SOLO LECTURA entre NUT (upsd) y HTTP/JSON.
//
// Diseño de seguridad:
//   - Solo implementa lectura (LIST). No existe código capaz de enviar
//     comandos (INSTCMD/SET) a NUT: el peor caso (apagar un SAI) es
//     imposible aunque el atacante controle la petición HTTP.
//   - Autenticación por token Bearer con comparación en tiempo constante.
//   - Fail-closed: sin token configurado, el proceso no arranca.
//   - Validación estricta del nombre del SAI: evita inyección de CRLF
//     en el protocolo de texto de NUT.
//   - upsd debe escuchar solo en 127.0.0.1; este puente le habla en local.
//   - CORS restringido a una lista de orígenes (nunca "*").
//   - Rate limiting global (token bucket) anti-fuerza-bruta / anti-DoS.
//   - Cache TTL para que upsd reciba como mucho 1 consulta/seg por clave.
//
// Sin dependencias externas: solo librería estándar.
package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// Configuración (vía variables de entorno; encaja con EnvironmentFile de systemd)
// ---------------------------------------------------------------------------

type config struct {
	listen   string
	token    string
	origins  []string
	nutAddr  string
	tlsCert  string
	tlsKey   string
	cacheTTL time.Duration
	nutTO    time.Duration
}

func loadConfig() (config, error) {
	c := config{
		listen:   env("BRIDGE_LISTEN", ":8080"),
		token:    os.Getenv("BRIDGE_TOKEN"),
		nutAddr:  env("NUT_ADDR", "127.0.0.1:3493"),
		tlsCert:  os.Getenv("BRIDGE_TLS_CERT"),
		tlsKey:   os.Getenv("BRIDGE_TLS_KEY"),
		cacheTTL: envDur("BRIDGE_CACHE_TTL", time.Second),
		nutTO:    envDur("BRIDGE_NUT_TIMEOUT", 3*time.Second),
	}
	for _, o := range strings.Split(env("BRIDGE_ORIGINS", ""), ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			c.origins = append(c.origins, o)
		}
	}
	if len(c.token) < 16 {
		return c, errors.New("BRIDGE_TOKEN ausente o demasiado corto (mínimo 16 caracteres)")
	}
	if (c.tlsCert == "") != (c.tlsKey == "") {
		return c, errors.New("BRIDGE_TLS_CERT y BRIDGE_TLS_KEY deben ir juntos")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// ---------------------------------------------------------------------------
// Cliente NUT (solo lectura)
// ---------------------------------------------------------------------------

type upsInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type upsData struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Vars        map[string]string `json:"vars"`
	UpdatedAt   string            `json:"updated_at"`
}

// nombreSAIValido: blindaje contra inyección de CRLF en el protocolo NUT.
var nombreSAIValido = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type nutClient struct {
	addr    string
	timeout time.Duration
}

func (c *nutClient) dial() (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(c.timeout))
	return conn, bufio.NewReader(conn), nil
}

func (c *nutClient) listUPS() ([]upsInfo, error) {
	conn, r, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := fmt.Fprint(conn, "LIST UPS\n"); err != nil {
		return nil, err
	}
	var out []upsInfo
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "END LIST UPS":
			fmt.Fprint(conn, "LOGOUT\n")
			return out, nil
		case strings.HasPrefix(line, "BEGIN"):
			continue
		case strings.HasPrefix(line, "ERR"):
			return nil, fmt.Errorf("nut: %s", line)
		case strings.HasPrefix(line, "UPS "):
			name, desc := splitNameQuoted(line[len("UPS "):])
			out = append(out, upsInfo{Name: name, Description: desc})
		}
	}
}

func (c *nutClient) listVars(ups string) (upsData, error) {
	if !nombreSAIValido.MatchString(ups) {
		return upsData{}, errors.New("nombre de SAI inválido")
	}
	conn, r, err := c.dial()
	if err != nil {
		return upsData{}, err
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(conn, "LIST VAR %s\n", ups); err != nil {
		return upsData{}, err
	}
	data := upsData{Name: ups, Vars: map[string]string{}}
	prefix := "VAR " + ups + " "
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return upsData{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "END LIST VAR"):
			fmt.Fprint(conn, "LOGOUT\n")
			data.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			return data, nil
		case strings.HasPrefix(line, "BEGIN"):
			continue
		case strings.HasPrefix(line, "ERR"):
			return upsData{}, fmt.Errorf("nut: %s", line)
		case strings.HasPrefix(line, prefix):
			rest := line[len(prefix):]
			sp := strings.IndexByte(rest, ' ')
			if sp < 0 {
				continue
			}
			key := rest[:sp]
			val := unquote(strings.TrimSpace(rest[sp+1:]))
			data.Vars[key] = val
		}
	}
}

func splitNameQuoted(s string) (name, quoted string) {
	sp := strings.IndexByte(s, ' ')
	if sp < 0 {
		return s, ""
	}
	return s[:sp], unquote(strings.TrimSpace(s[sp+1:]))
}

func unquote(s string) string {
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// ---------------------------------------------------------------------------
// Cache con TTL (protege a upsd de ráfagas de peticiones)
// ---------------------------------------------------------------------------

type cacheItem struct {
	data    []byte
	expires time.Time
}

type cache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]cacheItem
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl, items: map[string]cacheItem{}}
}

// getOrLoad devuelve JSON cacheado o lo genera con loader.
func (c *cache) getOrLoad(key string, loader func() (any, error)) ([]byte, error) {
	c.mu.Lock()
	if it, ok := c.items[key]; ok && time.Now().Before(it.expires) {
		c.mu.Unlock()
		return it.data, nil
	}
	c.mu.Unlock()

	v, err := loader()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.items[key] = cacheItem{data: b, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return b, nil
}

// ---------------------------------------------------------------------------
// Rate limiting (token bucket global, stdlib)
// ---------------------------------------------------------------------------

type limiter struct {
	mu     sync.Mutex
	tokens float64
	max    float64
	refill float64
	last   time.Time
}

func newLimiter(rps, burst float64) *limiter {
	return &limiter{tokens: burst, max: burst, refill: rps, last: time.Now()}
}

func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.tokens += now.Sub(l.last).Seconds() * l.refill
	if l.tokens > l.max {
		l.tokens = l.max
	}
	l.last = now
	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Servidor HTTP + middleware de seguridad
// ---------------------------------------------------------------------------

type server struct {
	cfg config
	nut *nutClient
	c   *cache
	lim *limiter
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ups", s.handleList)
	mux.HandleFunc("/api/ups/", s.handleUPS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	return s.middleware(mux)
}

func (s *server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cabeceras de seguridad
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// CORS restringido a orígenes concretos
		origin := r.Header.Get("Origin")
		if origin != "" && s.originPermitido(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		// Preflight: responder antes de exigir token
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Solo lectura: ningún método mutante
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Rate limiting
		if !s.lim.allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		// /healthz no requiere token (sondas locales)
		if r.URL.Path != "/healthz" && !s.tokenValido(r) {
			time.Sleep(300 * time.Millisecond) // ralentiza fuerza bruta
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) originPermitido(o string) bool {
	for _, allowed := range s.cfg.origins {
		if allowed == o {
			return true
		}
	}
	return false
}

func (s *server) tokenValido(r *http.Request) bool {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, p) {
		return false
	}
	got := []byte(strings.TrimPrefix(h, p))
	want := []byte(s.cfg.token)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (s *server) handleList(w http.ResponseWriter, _ *http.Request) {
	b, err := s.c.getOrLoad("__list__", func() (any, error) {
		return s.nut.listUPS()
	})
	s.writeJSON(w, b, err)
}

func (s *server) handleUPS(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/ups/")
	if !nombreSAIValido.MatchString(name) {
		http.Error(w, "invalid ups name", http.StatusBadRequest)
		return
	}
	b, err := s.c.getOrLoad("ups:"+name, func() (any, error) {
		return s.nut.listVars(name)
	})
	s.writeJSON(w, b, err)
}

func (s *server) writeJSON(w http.ResponseWriter, b []byte, err error) {
	if err != nil {
		log.Printf("error nut: %v", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuración inválida: %v", err)
	}

	s := &server{
		cfg: cfg,
		nut: &nutClient{addr: cfg.nutAddr, timeout: cfg.nutTO},
		c:   newCache(cfg.cacheTTL),
		lim: newLimiter(10, 20), // 10 req/s sostenido, ráfaga 20
	}

	srv := &http.Server{
		Addr:              cfg.listen,
		Handler:           s.routes(),
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 14, // 16 KB
	}

	go func() {
		var err error
		if cfg.tlsCert != "" {
			log.Printf("escuchando HTTPS en %s (NUT=%s)", cfg.listen, cfg.nutAddr)
			err = srv.ListenAndServeTLS(cfg.tlsCert, cfg.tlsKey)
		} else {
			log.Printf("escuchando HTTP en %s (NUT=%s) — sin TLS", cfg.listen, cfg.nutAddr)
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("servidor: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("apagado limpio")
}
