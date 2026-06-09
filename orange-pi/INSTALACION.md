# Monitor SAI con NUT · Guía de instalación real

Guía basada en la instalación real sobre **Orange Pi Zero Plus + Armbian + Salicru 850**.
Incluye todos los errores encontrados y sus soluciones.

---

## Tabla de contenido

- [Hardware verificado](#hardware-verificado)
- [Imagen de Armbian correcta](#imagen-de-armbian-correcta)
- [Paso 0 — Diagnóstico inicial](#paso-0--diagnóstico-inicial-ejecutar-antes-de-tocar-nada)
- [Paso 1 — Primera actualización del sistema](#paso-1--primera-actualización-del-sistema)
- [Paso 2 — Instalar NUT](#paso-2--instalar-nut)
  - [2.1 Identificar el SAI](#21-identificar-el-sai)
  - [2.2 Configurar los ficheros de NUT](#22-configurar-los-ficheros-de-nut)
  - [2.3 Arrancar y verificar](#23-arrancar-y-verificar)
- [Paso 3 — Compilar el puente](#paso-3--compilar-el-puente)
- [Paso 4 — Instalar el puente en la Pi](#paso-4--instalar-el-puente-en-la-pi)
- [Paso 5 — Verificar el puente](#paso-5--verificar-el-puente)
- [Paso 6 — Dashboard](#paso-6--dashboard)
- [Limitaciones conocidas del Salicru 850](#limitaciones-conocidas-del-salicru-850)
- [Modificar la configuración del puente](#modificar-la-configuración-del-puente)
- [Verificación rápida de salud](#verificación-rápida-de-salud)
- [Errores conocidos y soluciones (resumen)](#errores-conocidos-y-soluciones-resumen)
- [Segunda Pi en adelante](#segunda-pi-en-adelante)
- [Modificar el dashboard desde Claude Code](#modificar-el-dashboard-desde-claude-code)

---

## Hardware verificado

| Componente | Valor real |
|---|---|
| Placa | Orange Pi Zero Plus (H5, quad-core A53) |
| Arquitectura | **aarch64 (ARM64)** — no ARMv7 |
| SO | Armbian 24.8.1 Bookworm minimal (stable) |
| Kernel | 6.6.44 LTS |
| SAI | Salicru 850 (vendorid `2E66`, productid `0300`) |
| Driver NUT | `usbhid-ups` |

> El `lsusb` muestra el SAI como `ID 2e66:0300  1   850`. NUT lo identifica
> como "Salicru HID" internamente aunque la etiqueta física diga otra cosa.

---

## Imagen de Armbian correcta

**Usar siempre la imagen stable del archivo**, nunca nightly/rolling/trunk.

- Archivo: https://archive.armbian.com/orangepizeroplus/archive/
- Fichero: `Armbian_24.8.1_Orangepizeroplus_bookworm_current_6.6.44_minimal.img.xz`
- Tamaño: ~226 MB
- Flashear con Balena Etcher o `dd`

> **Error crítico:** la imagen *Rolling Release* (`26.x-trunk`) tiene `apt` roto —
> da `Illegal instruction` al intentar instalar cualquier paquete. Si el sistema
> base falla así, reflashea con la imagen stable del archivo.

> **SD card:** una tarjeta en mal estado hace que `apt upgrade` tarde eternidades,
> corte el SSH y acabe con el sistema montado en solo lectura. Síntomas en `dmesg`:
> `mmcblk0: recovery failed` y `EXT4-fs: Remounting filesystem read-only`.
> Solución: SD nueva.

---

## Paso 0 — Diagnóstico inicial (ejecutar antes de tocar nada)

```bash
cat /etc/armbian-release | grep -E "VERSION|IMAGE_TYPE"  # confirmar que es stable, no trunk
uname -m                                                   # debe ser aarch64
uname -r                                                   # kernel (debe ser 6.6.x)
dpkg -l nut* 2>/dev/null | grep ^ii                        # NUT instalado?
ss -tlnp | grep 3493                                       # upsd corriendo?
lsusb                                                      # SAI detectado?
```

Si `IMAGE_TYPE=nightly` o `VERSION` contiene `trunk`: reflashea antes de continuar.

---

## Paso 1 — Primera actualización del sistema

```bash
apt update && apt upgrade -y
```

Si este comando falla con `Illegal instruction`: la imagen está rota, reflashea.
Si tarda demasiado o corta el SSH: la SD está en mal estado, reemplázala.

---

## Paso 2 — Instalar NUT

```bash
apt install -y nut
```

> En Armbian Bookworm, `nut-scanner` viene incluido en el paquete `nut`.
> No existe como paquete separado — `apt install nut-scanner` da
> `Unable to locate package`.

### 2.1 Identificar el SAI

```bash
nut-scanner -U
```

Los avisos de `Cannot load SNMP/XML/AVAHI library` son **normales**.
Lo relevante es el bloque `[nutdev1]` con `driver`, `vendorid` y `productid`.

### 2.2 Configurar los ficheros de NUT

**`/etc/nut/nut.conf`** — el más importante. Sin esto, `upsd` no arranca:

```bash
echo 'MODE=netserver' > /etc/nut/nut.conf
```

> **Error frecuente:** si `upsd` arranca y se para con `upsd disabled, please
> adjust the configuration`, es porque `nut.conf` tiene `MODE=none` o está vacío.
> Solución: el comando de arriba.

**`/etc/nut/upsd.conf`** — solo localhost, nunca en red:

```bash
cat > /etc/nut/upsd.conf << 'EOF'
LISTEN 127.0.0.1 3493
MAXAGE 15
EOF
```

**`/etc/nut/upsd.users`** — usuario de solo lectura:

```bash
cat > /etc/nut/upsd.users << 'EOF'
[monitor]
    password = CAMBIA_ESTA_CLAVE
    upsmon primary
EOF
chmod 640 /etc/nut/upsd.users
chown root:nut /etc/nut/upsd.users
```

**`/etc/nut/ups.conf`** — usa `vendorid` + `productid` para identificar el SAI
de forma inequívoca:

```bash
cat > /etc/nut/ups.conf << 'EOF'
maxretry = 3
pollinterval = 2

[sai1]
    driver = usbhid-ups
    port = auto
    vendorid = 2E66
    productid = 0300
    desc = "Salicru 850"
EOF
```

> `pollinterval = 2` hace que el driver consulte el hardware cada 2 s.

### 2.3 Arrancar y verificar

```bash
systemctl restart nut-server
sleep 2
systemctl status nut-server
upsc sai1
```

> **Error frecuente:** `upsc sai1` da `Error: Driver not connected` justo tras
> el arranque. El driver de NUT en Bookworm tarda unos segundos en inicializarse.
> Espera 5 segundos y repite `upsc sai1` — suele resolverse solo.
> Comprueba el estado del driver con:
> ```bash
> systemctl list-units | grep nut
> # El servicio relevante es nut-driver@sai1.service — debe estar 'active running'
> ```

---

## Paso 3 — Subir el binario a la Pi

El proyecto ya incluye `sai-monitor-arm64` compilado para **ARM64** (Orange Pi Zero Plus).
No necesitas instalar Go ni compilar nada — solo súbelo:

```bash
scp pi/bridge/sai-monitor-arm64 root@IP_DE_LA_PI:/tmp/
```

> **Error frecuente en scp:** `WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED`
> Ocurre al reflashear la SD — la Pi tiene una clave SSH nueva. Solución:
> ```bash
> ssh-keygen -R IP_DE_LA_PI
> scp pi/bridge/sai-monitor-arm64 root@IP_DE_LA_PI:/tmp/
> ```

> **Error conocido:** con Armbian *nightly/trunk*, el binario da `Segmentation fault`
> al ejecutarse. No es un problema del binario — es la imagen rota. Reflashea.

### Si necesitas recompilar (solo si modificas main.go)

```bash
# En el Mac:
brew install go   # si no lo tienes
cd pi/bridge
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o sai-monitor-arm64 .

# Verificar
file sai-monitor-arm64
# Debe decir: ELF 64-bit LSB executable, ARM aarch64, statically linked

scp sai-monitor-arm64 root@IP_DE_LA_PI:/tmp/
```

---

## Paso 4 — Instalar el puente en la Pi

### 4.1 Instalar el binario

```bash
install -m 0755 /tmp/sai-monitor-arm64 /usr/local/bin/sai-monitor
```

### 4.2 Crear usuario y configuración

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin -g nut saibridge
mkdir -p /etc/sai-monitor
TOKEN=$(openssl rand -hex 32)
echo "TOKEN: $TOKEN"   # guárdalo
cat > /etc/sai-monitor/sai-monitor.env << EOF
BRIDGE_LISTEN=:49152
BRIDGE_TOKEN=${TOKEN}
BRIDGE_NAME=SAI Salón
NUT_ADDR=127.0.0.1:3493
BRIDGE_CACHE_TTL=1s
BRIDGE_ORIGINS=http://localhost:5500
EOF
chmod 600 /etc/sai-monitor/sai-monitor.env
```

> **`BRIDGE_NAME`** es el nombre con el que la Pi se anuncia en la LAN vía SSDP.
> Si tienes varios SAIs en la misma red, ponles nombres distintos
> (ej. "SAI Salón", "SAI Rack", "SAI Garaje") para distinguirlos en el descubrimiento.

> **`BRIDGE_ENROLLMENT_PASSWORD`** (opcional): habilita la obtención automática
> del token desde el dashboard. Cuando esta variable está definida, en el dashboard
> tras "Buscar SAIs en la red" aparecerá un botón "Conectar con contraseña". El
> usuario introduce esta contraseña una vez y el dashboard recibe el token sin
> tener que copiarlo a mano. Mínimo 8 caracteres. Si se omite, el método de
> enrollment queda desactivado y solo es posible copiar el token por SSH.
>
> Añadirla en el `.env`:
> ```
> BRIDGE_ENROLLMENT_PASSWORD=la-contraseña-que-quieras
> ```
> El proceso del puente la convierte a un hash SHA-256 al arrancar y borra la
> variable de entorno: la contraseña en claro no queda en memoria del proceso.

> **Error frecuente:** `useradd: user 'saibridge' already exists` — ya existía.
> Verifica con `id saibridge`. Si el `gid` es `nut`, está bien y continúa.

> **Error frecuente:** `chown: invalid group: 'root:saibridge'` aunque el usuario
> exista. Usa el GID numérico:
> ```bash
> id saibridge       # anota el gid (p. ej. 987)
> chown 0:987 /etc/sai-monitor/sai-monitor.env
> ```

> **Error frecuente:** al usar `&&` encadenado, si el primer comando falla
> (p. ej. `useradd` porque el usuario ya existe), los siguientes no se ejecutan.
> Si esto pasa, ejecuta los comandos restantes por separado.

### 4.3 Instalar el servicio systemd

```bash
cat > /etc/systemd/system/sai-monitor.service << 'EOF'
[Unit]
Description=Puente NUT->HTTP (solo lectura) para monitor de SAI
After=network-online.target nut-server.service
Wants=network-online.target

[Service]
User=saibridge
Group=nut
EnvironmentFile=/etc/sai-monitor/sai-monitor.env
ExecStart=/usr/local/bin/sai-monitor
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
RestrictNamespaces=true
LockPersonality=true
MemoryDenyWriteExecute=false
MemoryMax=64M
TasksMax=32

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now sai-monitor
systemctl status sai-monitor
```

> **`MemoryDenyWriteExecute=false`** es obligatorio en ARM64 con Go. Con `true`
> el proceso muere con SIGSEGV. No es un fallo de seguridad: Go necesita esto
> para el planificador de goroutines.

> **`ProtectSystem=strict`** monta `/etc` en solo lectura para el proceso.
> Para editar el `.env` con el servicio corriendo usa `sed -i` — dará
> `Read-only file system`. Procedimiento correcto:
> ```bash
> systemctl stop sai-monitor
> # editar /etc/sai-monitor/sai-monitor.env
> systemctl start sai-monitor
> ```

> **Error frecuente:** `Failed to enable unit: Unit file is masked`
> Ocurre si el servicio quedó enmascarado de una instalación anterior.
> Solución:
> ```bash
> systemctl unmask sai-monitor
> # y luego repetir el cat > ... << 'EOF' del servicio
> ```

> **Error frecuente al pegar bloques largos:** el terminal puede cortar el `cat`
> antes del `EOF` y quedarse esperando input. Si el prompt no vuelve, pulsa
> `Ctrl+C` y pega el bloque en dos partes: primero el `cat << 'EOF' ... EOF`
> y luego los comandos `systemctl` por separado.

---

## Paso 5 — Verificar el puente

```bash
# Debe dar 401 (rechaza sin token)
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:49152/api/ups

# Debe devolver JSON con la lista de SAIs
curl -s -H "Authorization: Bearer TU_TOKEN" http://127.0.0.1:49152/api/ups

# Debe devolver todas las variables del SAI
curl -s -H "Authorization: Bearer TU_TOKEN" http://127.0.0.1:49152/api/ups/sai1
```

> **Error frecuente:** el curl devuelve `000` (sin respuesta) en lugar de `401`.
> El servicio no está escuchando. Comprueba:
> ```bash
> systemctl status sai-monitor
> journalctl -u sai-monitor -n 20
> ```

---

## Paso 6 — Dashboard

Desde la carpeta del proyecto:

```bash
python3 client/scripts/serve.py
```

Abre el navegador automáticamente en `http://localhost:5500`. Ctrl+C para parar.

> **Autodescubrimiento**: en la pestaña Equipos, pulsa "Buscar SAIs en la red".
> Cada Pi con el puente corriendo aparecerá automáticamente (sin teclear IP).
> El token sigue siendo obligatorio — el descubrimiento solo encuentra la URL.

> **Error frecuente:** `Puerto 5500 ocupado`
> El script lo detecta y te da el comando exacto para liberarlo.
> Si prefieres hacerlo manual:
> ```bash
> kill $(lsof -ti:5500)
> python3 client/scripts/serve.py
> ```

Pulsa "Añadir equipo":

| Campo | Valor |
|---|---|
| Etiqueta | Salicru 850 (o lo que quieras) |
| URL | `http://IP_DE_LA_PI:49152` |
| Token | el generado con `openssl rand -hex 32` |

> **Error frecuente en Firefox:** `Uncaught ReferenceError: f_label is not defined`
> Firefox no expone los elementos del DOM como variables globales por su `id`.
> Solución: usar el `index.html` corregido que usa `document.getElementById()`
> y `addEventListener` en lugar de referencias globales.

> **Error frecuente:** la gráfica aparece unos segundos y desaparece en cada
> refresco. Ocurre porque el panel se reconstruye con `innerHTML` destruyendo
> el contenedor de uPlot. Solución: usar el `index.html` corregido donde la
> estructura del panel se crea una sola vez y solo se actualizan los datos.

> **Nota sobre `BRIDGE_ORIGINS`:** el origen debe coincidir exactamente con la
> URL desde donde sirves el dashboard. Si sirves desde el Mac con
> `python3 -m http.server 5500`, el origen es `http://localhost:5500`. Si lo
> abres con la IP del Mac, debe ser `http://IP_MAC:5500`. Deben coincidir.
> Para cambiar el origen en la Pi:
> ```bash
> systemctl stop sai-monitor
> # editar BRIDGE_ORIGINS en /etc/sai-monitor/sai-monitor.env
> systemctl start sai-monitor
> ```

---

## Limitaciones conocidas del Salicru 850

- **No reporta `ups.realpower`** (vatios reales). Solo `ups.realpower.nominal`
  (490W, potencia máxima). El dashboard estima el consumo como
  `ups.load% × 490W` y lo muestra como "Potencia (est.)".
- **Serial vacío** — el firmware no lo expone.
- **`output.voltage`** reporta el voltaje de la batería interna (12V), no el
  voltaje de salida de red. Es una limitación del firmware.

---

## Modificar la configuración del puente

```bash
systemctl stop sai-monitor

cat > /etc/sai-monitor/sai-monitor.env << 'EOF'
BRIDGE_LISTEN=:49152
BRIDGE_TOKEN=TU_TOKEN
NUT_ADDR=127.0.0.1:3493
BRIDGE_CACHE_TTL=1s
BRIDGE_ORIGINS=http://localhost:5500
EOF

systemctl start sai-monitor
systemctl status sai-monitor
```

---

## Verificación rápida de salud

```bash
# SD card: busca errores de I/O
dmesg | grep -E "I/O error|mmcblk|EXT4-fs" | tail -10

# NUT leyendo el SAI
upsc sai1 | grep -E "ups.status|battery.charge|ups.load"

# Puente respondiendo
curl -s -H "Authorization: Bearer TU_TOKEN" http://127.0.0.1:49152/api/ups

# Logs del puente en tiempo real
journalctl -u sai-monitor -f
```

---

## Errores conocidos y soluciones (resumen)

| Error | Causa | Solución |
|---|---|---|
| `Illegal instruction` en apt | Imagen nightly/trunk rota | Reflashear con stable 24.8.1 |
| `upsd disabled` al arrancar | `nut.conf` vacío o `MODE=none` | `echo 'MODE=netserver' > /etc/nut/nut.conf` |
| `Error: Driver not connected` | Driver NUT tardando en iniciar | Esperar 5s y repetir `upsc sai1` |
| `SEGV` en el binario Go | Imagen rota o `MemoryDenyWriteExecute=true` | Reflashear o cambiar a `false` en el `.service` |
| `chown: invalid group` | Bug en Armbian con usuarios de sistema | Usar GID numérico: `chown 0:GID fichero` |
| `sed -i: Read-only file system` | `ProtectSystem=strict` activo | Parar el servicio antes de editar |
| `Unit file is masked` | Servicio enmascarado de instalación anterior | `systemctl unmask sai-monitor` |
| `WARNING: REMOTE HOST IDENTIFICATION` | SD reflasheada, clave SSH nueva | `ssh-keygen -R IP_DE_LA_PI` |
| `Address already in use` en python | Puerto 5500 ya ocupado | `serve.py` lo detecta; o `kill $(lsof -ti:5500)` |
| `f_label is not defined` en Firefox | Firefox no expone IDs como variables globales | Usar index.html corregido con `getElementById` |
| Gráfica desaparece al refrescar | `innerHTML` destruye el contenedor uPlot | Usar index.html corregido con estructura fija |
| SD card: sistema de ficheros ro | SD en mal estado | SD nueva |
| `Call to Reboot failed` | Sesión degradada por fallo de SD | `echo b > /proc/sysrq-trigger` o desconectar alimentación |

---

## Segunda Pi en adelante

El proceso es idéntico. Lo único que cambia:

1. Token distinto por Pi (`openssl rand -hex 32`) — así puedes revocar uno sin afectar a los demás.
2. El nombre `sai1` en `ups.conf` puede repetirse — cada Pi es independiente.
3. Añade la nueva IP en el dashboard con su token.
4. `BRIDGE_ORIGINS` es el mismo en todas las Pis si el dashboard se sirve desde el mismo sitio.

---

## Modificar el dashboard desde Claude Code

Para hacer cambios en `index.html` sin copiar y pegar código:

```bash
# Instalar Claude Code
npm install -g @anthropic/claude-code

# Entrar en la carpeta del proyecto
cd ~/ruta/al/proyecto

# Arrancar Claude Code
claude
```

Dentro describes el cambio en lenguaje natural y Claude Code edita el fichero directamente.
