# PNut — NUT UPS Monitor (Raspberry Pi / hardware potente)

> Carpeta `raspberry-pi/`: variante para **Raspberry Pi 3/4/5 y hardware potente** (Python + Flask + React).
> Para Orange Pi Zero Plus u otro hardware ligero ARM64, mira la carpeta [`orange-pi/`](../orange-pi/).

Monitor web para SAI **Salicru SPS 850 Home** usando [Network UPS Tools](https://networkupstools.org/).
Flask API + React frontend con polling en tiempo real.

## Stack

- **Backend**: Python 3 + Flask + flask-cors
- **Frontend**: React + Vite (sin framework CSS, inline styles)
- **NUT driver**: `usbhid-ups` con `Salicru HID 0.4`
- **Hardware**: Raspberry Pi (Debian), SAI conectado por USB

## Estructura en la Pi

```
/home/guille/ups-monitor/
├── api.py                  # Flask API
└── frontend/
    ├── src/
    │   └── App.jsx         # Componente React principal
    ├── package.json
    └── dist/               # Build de producción (servido por Flask)
```

## Requisitos

```bash
# NUT
sudo apt install nut nut-client

# Python
pip install flask flask-cors

# Node (para build del frontend)
node >= 18
```

## Configuración NUT

**`/etc/nut/ups.conf`**
```ini
[salicru]
  driver = usbhid-ups
  port = auto
  desc = "Salicru SPS 850 Home"
```

**`/etc/nut/nut.conf`**
```ini
MODE=standalone
```

**`/etc/nut/upsd.conf`** — añadir si se quiere acceso desde red:
```ini
LISTEN 0.0.0.0 3493
```

Verificar que el SAI responde:
```bash
upsc salicru@localhost
```

## Instalación y arranque

```bash
# 1. Clonar
git clone https://github.com/Marsdix/PNut.git
cd PNut

# 2. Build frontend
cd frontend
npm install
npm run build
cd ..

# 3. Arrancar API (puerto 5000)
python3 api.py
```

Acceder en `http://<ip-raspberry>:5000`

## API endpoints

| Endpoint | Descripción |
|---|---|
| `GET /api/ups` | Datos procesados del SAI |
| `GET /api/ups/raw` | Volcado completo de variables NUT |

### Respuesta `/api/ups`

```json
{
  "battery_charge": 95,
  "battery_voltage": 13.6,
  "battery_runtime": 94,
  "input_voltage": 219.2,
  "input_frequency": 49.8,
  "output_voltage": 14.4,
  "output_frequency": 49.8,
  "ups_load": 0,
  "ups_realpower_nom": 490,
  "ups_status": "OL CHRG",
  "ups_temperature": null,
  "ups_model": "850",
  "ups_mfr": "1",
  "ups_firmware": "Salicru HID 0.4",
  "ups_serial": null,
  "battery_charge_low": 10,
  "battery_runtime_low": 5
}
```

## Variables NUT relevantes del Salicru SPS 850

| Variable NUT | Descripción |
|---|---|
| `ups.load` | Carga en % (0–100). Reporta 0 durante carga de batería (`OL CHRG`) |
| `ups.realpower.nominal` | Potencia nominal: 490 W |
| `ups.status` | Flags: `OL` online, `OB` en batería, `CHRG` cargando, `LB` batería baja |
| `battery.charge` | % carga batería |
| `battery.runtime` | Autonomía en segundos |
| `battery.voltage` | Tensión batería (12V plomo-ácido) |
| `input.voltage` | Tensión red AC |

## Comportamiento conocido del driver

- **`ups.load = 0` durante `OL CHRG`**: El driver `usbhid-ups` con firmware Salicru HID 0.4 reporta 0% de carga mientras la batería está cargando. Es normal — los valores reales se recuperan cuando termina la carga.
- **`ups.temperature`**: Este modelo no reporta temperatura (`null` siempre).
- **`output.voltage`**: Reporta ~14V (tensión batería interna), no 230V.

## Despliegue como servicio (systemd)

```ini
# /etc/systemd/system/ups-monitor.service
[Unit]
Description=UPS Monitor Flask API
After=network.target

[Service]
User=guille
WorkingDirectory=/home/guille/ups-monitor
ExecStart=/usr/bin/python3 api.py
Restart=always

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable ups-monitor
sudo systemctl start ups-monitor
```

## Acceso remoto NUT (puerto 3493)

Consultar variables directamente:
```bash
# Listar SAIs disponibles
printf "LIST UPS\n" | nc -w3 <ip> 3493

# Listar todas las variables
printf "LIST VAR salicru\n" | nc -w3 <ip> 3493
```

## Notas de desarrollo

- `API_URL` en `App.jsx` apunta a `http://192.168.0.144:5000/api/ups` — cambiar según IP de la Pi.
- El frontend arranca con datos mock mientras no hay conexión real (badge naranja `MOCK`).
- `everConnected` ref: una vez establecida la primera conexión real, la UI muestra carga aunque `ups_load` sea null puntualmente.
