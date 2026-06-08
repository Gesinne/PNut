# PNut — Monitor SAI con NUT

Puente de **solo lectura** entre NUT (`upsd`) y HTTP/JSON, con dashboard web para monitorizar SAIs (UPS) conectados por USB. Sin dependencias externas: binario Go estático + HTML vanilla.

> Esta rama (`main`) está optimizada para **hardware ligero**: Orange Pi Zero Plus, Orange Pi Zero 2, Raspberry Pi Zero 2 W y similares (ARM64, menos de 512 MB RAM).
> Para Raspberry Pi 3/4/5 o servidores x86 con interfaz React completa, ve a la rama [`raspberry-pi`](../../tree/raspberry-pi).

---

## Variantes disponibles

| Rama | Hardware | Stack | Uso |
|---|---|---|---|
| **`main`** (esta) | Orange Pi Zero Plus, ARM64 | Go + HTML vanilla | Homelab con recursos limitados |
| [`raspberry-pi`](../../tree/raspberry-pi) | Raspberry Pi 3/4/5, x86 | Python Flask + React | Hardware más potente, UI completa |

---

## Características

- Binario Go estático (~6 MB), sin intérprete ni Docker
- Dashboard en una sola página HTML (sin build, sin npm)
- **Autodescubrimiento SSDP en LAN**: las Pis se anuncian solas, el dashboard las encuentra sin teclear IP
- Puerto privado IANA (49152) en lugar de 8080 para evitar bloqueos de proxies/firewalls
- Autenticación por token Bearer con comparación en tiempo constante
- Rate limiting, caché TTL, CORS restringido a orígenes concretos
- Alertas visuales: batería baja, carga elevada, batería dañada
- Modo claro/oscuro, responsive, sin dependencias de red para funcionar

---

## Autodescubrimiento en LAN

Cada puente SAI se anuncia en la red local vía **SSDP multicast** (UDP 1900). El dashboard tiene un botón "Buscar SAIs en la red" que los lista automáticamente — solo tienes que pegar el token.

> **Importante**: el descubrimiento NO sustituye al token. SSDP solo encuentra la URL del puente. El token Bearer sigue siendo obligatorio para cada petición HTTP. Cada Pi tiene su propio token generado con `openssl rand -hex 32`.

Identifica cada SAI con `BRIDGE_NAME` en `/etc/sai-monitor/sai-monitor.env`. Si tienes varios SAIs en la red, cada uno aparecerá con su nombre.

---

## Estructura del repositorio

```
├── bridge/
│   ├── main.go              # Puente NUT→HTTP (Go, stdlib pura)
│   ├── go.mod
│   └── sai-monitor-arm64    # Binario compilado para ARM64
├── dashboard/
│   └── index.html           # Dashboard web (HTML + CSS + JS vanilla)
├── deploy/
│   ├── systemd/
│   │   └── sai-monitor.service   # Servicio systemd con hardening
│   ├── nut/                      # Plantillas de configuración NUT
│   ├── network/
│   │   └── nftables-sai.conf     # Firewall por subred
│   └── tls/
│       └── gen-cert.sh           # TLS autofirmado (opcional)
├── scripts/
│   └── serve.py             # Servidor local del dashboard (Mac/Linux)
└── INSTALACION.md           # Guía con todos los errores reales encontrados
```

---

## Instalación rápida

Ver **[INSTALACION.md](INSTALACION.md)** para la guía completa paso a paso.

```bash
# 1. Subir el binario compilado a la Pi
scp bridge/sai-monitor-arm64 root@IP_PI:/tmp/

# 2. Instalar y configurar NUT + servicio systemd en la Pi
#    (ver INSTALACION.md, Pasos 2-4)

# 3. Abrir el dashboard en el Mac
python3 scripts/serve.py
```

---

## Hardware verificado

| Componente | Valor |
|---|---|
| Placa | Orange Pi Zero Plus (H5, quad-core A53) |
| Arquitectura | aarch64 (ARM64) |
| SO | Armbian 24.8.1 Bookworm minimal (stable) |
| SAI | Salicru 850 (vendorid 2E66, productid 0300) |
| Driver NUT | usbhid-ups |

---

## Descargas

Ver [Releases](../../releases) para descargar los binarios compilados directamente.

---

## Recompilar el binario

Solo necesario si modificas `bridge/main.go`:

```bash
cd bridge
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o sai-monitor-arm64 .
```
