# PNut — Monitor SAI con NUT

> Puente de **solo lectura** entre NUT y HTTP/JSON · Dashboard web · Sin dependencias · ARM64

```
Orange Pi Zero Plus  ──USB──  SAI (UPS)
        │
    sai-monitor (Go · 6 MB · systemd)
        │  HTTP :49152 + SSDP LAN
        ▼
   Dashboard (HTML vanilla · Mac/móvil)
```

---

## Instalación rápida

**3 comandos desde el Mac:**

```bash
# 1. Copiar archivos a la Pi
scp -r pi/deploy/firstboot pi/deploy/systemd pi/bridge/sai-monitor-arm64 root@<IP_PI>:/tmp/

# 2. Configurar e instalar en la Pi (SSH)
echo 'BRIDGE_NAME=SAI Salón
BRIDGE_ENROLLMENT_PASSWORD=mipassword123' > /boot/sai-bootstrap.conf
cd /tmp/firstboot && ./install.sh && systemctl reboot

# 3. Abrir dashboard en el Mac (tras reinicio)
python3 client/scripts/serve.py
```

`http://localhost:5500` → Buscar SAIs en la red → Conectar con contraseña

Ver **[INSTALACION.md](INSTALACION.md)** para la guía completa con verificación y troubleshooting.

---

## Características

| | |
|---|---|
| **Binario estático** | Go stdlib pura · ~6 MB · sin intérprete ni Docker |
| **Dashboard sin build** | HTML + CSS + JS vanilla · sin npm · sin dependencias de red |
| **Autodescubrimiento SSDP** | Las Pis se anuncian en LAN · el dashboard las encuentra sin teclear IP |
| **Enrollment con contraseña** | Una contraseña → token automático · sin copiar tokens a mano |
| **Seguridad** | Token Bearer · comparación en tiempo constante · rate limiting · CORS restringido |
| **Alertas visuales** | Batería baja · carga elevada · batería dañada |
| **Modo claro/oscuro** | Responsive · funciona sin internet |

---

## Cómo funciona el autodescubrimiento

```
Pi ──SSDP multicast UDP 1900──▶ LAN
         ▲
Dashboard "Buscar SAIs" escucha
         │
         └─ Muestra: "SAI Salón — http://192.168.x.x:49152"
                        ↓
              Conectar con contraseña  →  token automático
              Añadir manualmente       →  pegar BRIDGE_TOKEN
```

> SSDP encuentra la URL. El token Bearer sigue siendo obligatorio en cada petición HTTP.
> Cada Pi tiene su propio token generado automáticamente con `openssl rand -hex 32`.

---

## Estructura

```
orange-pi/
├── pi/                          ← VA A LA ORANGE PI
│   ├── bridge/
│   │   ├── main.go              Puente NUT→HTTP (Go, stdlib pura)
│   │   └── sai-monitor-arm64    Binario precompilado ARM64
│   └── deploy/
│       ├── firstboot/           Bootstrap automático (detecta SAI en cada arranque)
│       ├── systemd/             Unit file con hardening
│       ├── network/             Firewall nftables opcional
│       └── tls/                 TLS autofirmado opcional
│
├── client/                      ← CORRE EN EL MAC / NAVEGADOR
│   ├── dashboard/index.html     Dashboard (HTML + CSS + JS vanilla)
│   └── scripts/serve.py         Servidor local
│
├── INSTALACION.md               Guía completa con errores reales y soluciones
└── PRODUCT.md                   Decisiones de diseño
```

---

## Hardware verificado

| Componente | Valor |
|---|---|
| Placa | Orange Pi Zero Plus (Allwinner H5 · quad-core A53) |
| Arquitectura | aarch64 (ARM64) |
| SO | Armbian 24.8.1 Bookworm minimal (stable) |
| SAI | Salicru SPS 850 (`vendorid 2E66` · `productid 0300`) |
| Driver NUT | `usbhid-ups` |
| Puerto puente | 49152 (rango privado IANA) |

> Para **Raspberry Pi 3/4/5** o servidores x86 con UI React completa: rama [`raspberry-pi`](../../tree/raspberry-pi).

---

## Múltiples SAIs en la red

Cada Pi corre su propio puente. El dashboard descubre todos automáticamente.

```
LAN
├── Orange Pi #1 ── SAI Salón  ──▶ http://192.168.x.10:49152
├── Orange Pi #2 ── SAI Rack   ──▶ http://192.168.x.11:49152
└── Orange Pi #3 ── SAI Garaje ──▶ http://192.168.x.12:49152
         ▲
Dashboard "Buscar SAIs" los lista todos
```

Solo cambia `BRIDGE_NAME` y `BRIDGE_ENROLLMENT_PASSWORD` en `/boot/sai-bootstrap.conf` por Pi.

---

## Recompilar el binario

Solo necesario si modificas `bridge/main.go`:

```bash
cd pi/bridge
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o sai-monitor-arm64 .
```

---

## Licencia

MIT
