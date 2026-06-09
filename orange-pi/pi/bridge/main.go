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
	"crypto/sha1"
	"crypto/sha256"
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
	listen          string
	token           string
	origins         []string
	nutAddr         string
	tlsCert         string
	tlsKey          string
	cacheTTL        time.Duration
	nutTO           time.Duration
	enrollPassHash  []byte // SHA-256 de BRIDGE_ENROLLMENT_PASSWORD; nil = enrollment desactivado
}

func loadConfig() (config, error) {
	c := config{
		listen:   env("BRIDGE_LISTEN", ":49152"),
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
	// Enrollment opcional: si está presente, debe tener >= 8 caracteres.
	// La contraseña en claro NO se guarda; solo su SHA-256.
	if pw := os.Getenv("BRIDGE_ENROLLMENT_PASSWORD"); pw != "" {
		if len(pw) < 8 {
			return c, errors.New("BRIDGE_ENROLLMENT_PASSWORD demasiado corta (mínimo 8 caracteres)")
		}
		sum := sha256.Sum256([]byte(pw))
		c.enrollPassHash = sum[:]
		// Borrar la variable de entorno para reducir exposición en /proc.
		_ = os.Unsetenv("BRIDGE_ENROLLMENT_PASSWORD")
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
	cfg        config
	nut        *nutClient
	c          *cache
	lim        *limiter
	enrollLim  *ipLimiter // rate limit por IP para /api/enroll
}

// ---------------------------------------------------------------------------
// Rate limiter por IP (anti-fuerza-bruta de enrollment)
// ---------------------------------------------------------------------------

type ipLimiter struct {
	mu      sync.Mutex
	rps     float64
	burst   float64
	buckets map[string]*limiter
}

func newIPLimiter(rps, burst float64) *ipLimiter {
	return &ipLimiter{rps: rps, burst: burst, buckets: map[string]*limiter{}}
}

func (i *ipLimiter) allow(ip string) bool {
	i.mu.Lock()
	l, ok := i.buckets[ip]
	if !ok {
		l = newLimiter(i.rps, i.burst)
		i.buckets[ip] = l
	}
	i.mu.Unlock()
	return l.allow()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ups", s.handleList)
	mux.HandleFunc("/api/ups/", s.handleUPS)
	mux.HandleFunc("/api/enroll", s.handleEnroll)
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		// Preflight: responder antes de exigir token
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Métodos permitidos por path:
		//   GET  → todos los endpoints excepto /api/enroll
		//   POST → solo /api/enroll
		isEnroll := r.URL.Path == "/api/enroll"
		switch {
		case isEnroll && r.Method != http.MethodPost:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		case !isEnroll && r.Method != http.MethodGet:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Rate limiting global
		if !s.lim.allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		// Endpoints sin token: /healthz y /api/enroll (este último usa contraseña)
		if r.URL.Path != "/healthz" && !isEnroll && !s.tokenValido(r) {
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

func (s *server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	// Si no hay contraseña configurada, el endpoint no existe.
	if s.cfg.enrollPassHash == nil {
		http.NotFound(w, r)
		return
	}
	// Rate limit por IP: 3 intentos/min en ráfaga máxima 3.
	if !s.enrollLim.allow(ip) {
		log.Printf("enroll: rate-limit %s", ip)
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	// Body acotado a 256 bytes para evitar abuse.
	r.Body = http.MaxBytesReader(w, r.Body, 256)
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256([]byte(body.Password))
	body.Password = "" // descartar de memoria del handler
	if subtle.ConstantTimeCompare(sum[:], s.cfg.enrollPassHash) != 1 {
		time.Sleep(500 * time.Millisecond) // anti-fuerza-bruta
		log.Printf("enroll: FAIL %s", ip)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	log.Printf("enroll: OK %s", ip)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp, _ := json.Marshal(map[string]string{"token": s.cfg.token})
	w.Write(resp)
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
// SSDP — autodescubrimiento en LAN (stdlib pura, sin dependencias externas)
// ---------------------------------------------------------------------------

const (
	ssdpMulticast  = "239.255.255.250:1900"
	ssdpST         = "urn:schemas-pnut:device:SaiMonitor:1"
	ssdpNotifyFreq = 30 * time.Minute
)

// deviceUUID genera un UUID estable derivado del hostname (sin fichero de estado).
func deviceUUID() string {
	hostname, _ := os.Hostname()
	h := sha1.New()
	h.Write([]byte("pnut-sai-monitor-" + hostname))
	b := h.Sum(nil) // 20 bytes
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// lanIP devuelve una IPv4 LAN no-loopback y no link-local.
// Prioriza eth0 (cableada) sobre wlan0 (WiFi) sobre cualquier otra.
func lanIP() string {
	ifaces, _ := net.Interfaces()
	score := func(name string) int {
		switch {
		case strings.HasPrefix(name, "eth"), strings.HasPrefix(name, "enp"), strings.HasPrefix(name, "en"):
			return 3
		case strings.HasPrefix(name, "wlan"), strings.HasPrefix(name, "wlp"):
			return 2
		case strings.HasPrefix(name, "docker"), strings.HasPrefix(name, "br-"), strings.HasPrefix(name, "veth"):
			return 0
		default:
			return 1
		}
	}
	bestIP := ""
	bestScore := -1
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			s := score(iface.Name)
			if s > bestScore {
				bestScore = s
				bestIP = ip4.String()
			}
		}
	}
	return bestIP
}

func ssdpNotifyPkt(location, uuid, name string) []byte {
	return []byte("NOTIFY * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"LOCATION: " + location + "\r\n" +
		"NT: " + ssdpST + "\r\n" +
		"NTS: ssdp:alive\r\n" +
		"SERVER: Linux/armv8 UPnP/1.0 PNut/1.0\r\n" +
		"USN: uuid:" + uuid + "::" + ssdpST + "\r\n" +
		"X-PNUT-NAME: " + name + "\r\n\r\n")
}

func ssdpResponsePkt(location, uuid, name string) []byte {
	return []byte("HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"DATE: " + time.Now().UTC().Format(http.TimeFormat) + "\r\n" +
		"EXT:\r\n" +
		"LOCATION: " + location + "\r\n" +
		"SERVER: Linux/armv8 UPnP/1.0 PNut/1.0\r\n" +
		"ST: " + ssdpST + "\r\n" +
		"USN: uuid:" + uuid + "::" + ssdpST + "\r\n" +
		"X-PNUT-NAME: " + name + "\r\n\r\n")
}

// ssdpSender envía NOTIFY alive al grupo multicast cada ssdpNotifyFreq.
func ssdpSender(location, uuid, name string) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpMulticast)
	if err != nil {
		log.Printf("ssdp sender: %v", err)
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Printf("ssdp sender: %v", err)
		return
	}
	defer conn.Close()
	pkt := ssdpNotifyPkt(location, uuid, name)
	// Burst inicial de 3 NOTIFYs separados 200ms (recomendación UPnP)
	// para que dispositivos que están escuchando reciban al menos uno.
	for i := 0; i < 3; i++ {
		if _, werr := conn.Write(pkt); werr != nil {
			log.Printf("ssdp sender: write: %v", werr)
		}
		time.Sleep(200 * time.Millisecond)
	}
	for {
		time.Sleep(ssdpNotifyFreq)
		if _, werr := conn.Write(pkt); werr != nil {
			log.Printf("ssdp sender: write: %v", werr)
		}
	}
}

// ssdpListener responde a M-SEARCH desde el grupo multicast.
func ssdpListener(location, uuid, name string) {
	gaddr, err := net.ResolveUDPAddr("udp4", ssdpMulticast)
	if err != nil {
		log.Printf("ssdp listener: %v", err)
		return
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, gaddr)
	if err != nil {
		log.Printf("ssdp listener: no se pudo unirse al multicast (¿otro proceso en UDP 1900?): %v", err)
		return
	}
	defer conn.Close()
	buf := make([]byte, 2048)
	for {
		n, src, rerr := conn.ReadFromUDP(buf)
		if rerr != nil {
			log.Printf("ssdp listener: read: %v — saliendo", rerr)
			return
		}
		msg := string(buf[:n])
		if !strings.HasPrefix(msg, "M-SEARCH") {
			continue
		}
		if !strings.Contains(msg, ssdpST) && !strings.Contains(msg, "ssdp:all") {
			continue
		}
		conn.WriteToUDP(ssdpResponsePkt(location, uuid, name), src)
	}
}

// ssdpLoop arranca el sender y el listener en goroutines independientes.
func ssdpLoop(location, uuid, name string) {
	go ssdpSender(location, uuid, name)
	go ssdpListener(location, uuid, name)
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
		cfg:       cfg,
		nut:       &nutClient{addr: cfg.nutAddr, timeout: cfg.nutTO},
		c:         newCache(cfg.cacheTTL),
		lim:       newLimiter(10, 20),         // 10 req/s sostenido, ráfaga 20
		enrollLim: newIPLimiter(0.05, 3),      // 3 intentos/min por IP (0.05 rps), ráfaga 3
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

	// Autodescubrimiento SSDP en LAN
	if ip := lanIP(); ip != "" {
		_, port, _ := net.SplitHostPort(cfg.listen)
		location := "http://" + ip + ":" + port
		uuid := deviceUUID()
		name := env("BRIDGE_NAME", "SAI Monitor")
		go ssdpLoop(location, uuid, name)
		log.Printf("ssdp: anunciando %q en %s", name, location)
	} else {
		log.Printf("ssdp: no se detectó IP LAN, autodescubrimiento desactivado")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("apagado limpio")
}
