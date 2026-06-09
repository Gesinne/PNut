# Monitor SAI con NUT · Guía de instalación

Guía probada paso a paso sobre **Orange Pi Zero Plus + Armbian + Salicru SPS 850**. Incluye los comandos exactos, ejemplos de salida real, y los errores que pueden aparecer con su solución.

---

## Tabla de contenido

- [Hardware verificado](#hardware-verificado)
- [Imagen de Armbian](#imagen-de-armbian)
- [Paso 0 — Diagnóstico inicial](#paso-0--diagnóstico-inicial)
- [Paso 1 — Primera actualización del sistema](#paso-1--primera-actualización-del-sistema)
- [Paso 2 — Instalar y configurar NUT](#paso-2--instalar-y-configurar-nut)
- [Paso 3 — Subir el binario del puente a la Pi](#paso-3--subir-el-binario-del-puente-a-la-pi)
- [Paso 4 — Instalar el puente en la Pi](#paso-4--instalar-el-puente-en-la-pi)
- [Paso 5 — Servicio systemd](#paso-5--servicio-systemd)
- [Paso 6 — Verificar el puente](#paso-6--verificar-el-puente)
- [Paso 7 — Dashboard en el Mac](#paso-7--dashboard-en-el-mac)
- [Errores conocidos y soluciones](#errores-conocidos-y-soluciones)
- [Limitaciones del Salicru SPS 850](#limitaciones-del-salicru-sps-850)
- [Segunda Pi en adelante](#segunda-pi-en-adelante)

---

## Hardware verificado

| Componente | Valor |
|---|---|
| Placa | Orange Pi Zero Plus (Allwinner H5, quad-core A53) |
| Arquitectura | aarch64 (ARM64) |
| SO | Armbian 24.8.1 Bookworm minimal (stable) |
| Kernel | 6.6.44 LTS |
| SAI | Salicru SPS 850 Home (`vendorid 2E66`, `productid 0300`) |
| Driver NUT | `usbhid-ups` |
| Interfaz red | `end0` (Ethernet, nomenclatura moderna de Armbian) |

---

## Imagen de Armbian

**Usar siempre la imagen stable del archivo**, nunca nightly/rolling/trunk:

- Archivo: https://archive.armbian.com/orangepizeroplus/archive/
- Fichero: `Armbian_24.8.1_Orangepizeroplus_bookworm_current_6.6.44_minimal.img.xz`
- Tamaño: ~226 MB
- Flashear con Balena Etcher o `dd`

> **Error crítico**: la imagen *Rolling Release* (`26.x-trunk`) tiene `apt` roto, da `Illegal instruction` al instalar paquetes. Si lo ves, reflashea con la imagen stable.

> **SD card**: una tarjeta defectuosa hace que `apt upgrade` tarde eternidades, corte el SSH y monte el filesystem en solo lectura. Síntomas en `dmesg`: `mmcblk0: recovery failed` y `EXT4-fs: Remounting filesystem read-only`. Solución: SD nueva.

---

## Paso 0 — Diagnóstico inicial

Antes de tocar nada, comprueba que la imagen es la correcta:

```bash
cat /etc/armbian-release | grep -E "VERSION|IMAGE_TYPE"
uname -m       # debe ser aarch64
uname -r       # debe ser 6.6.x
```

Si `IMAGE_TYPE=nightly` o `VERSION` contiene `trunk`, reflashea antes de continuar.

---

## Paso 1 — Primera actualización del sistema

```bash
apt update && apt upgrade -y
```

Si falla con `Illegal instruction`, la imagen está rota. Si tarda demasiado o corta SSH, la SD está mal.

---

## Paso 2 — Instalar y configurar NUT

### 2.1 Instalar NUT

```bash
apt install -y nut
```

### 2.2 Identificar el SAI

```bash
lsusb
```

Salida esperada (busca tu SAI entre las líneas):
```
Bus 005 Device 002: ID 2e66:0300 1   850
```

Anota el `vendorid` y `productid`. Para el Salicru 850 son `2E66` y `0300`.

```bash
nut-scanner -U
```

Salida esperada:
```
[nutdev1]
        driver = "usbhid-ups"
        port = "auto"
        vendorid = "2E66"
        productid = "0300"
        product = " 850"
        vendor = "1"
        bus = "005"
```

Los avisos `Cannot load SNMP/XML/AVAHI library` son normales — el escáner intenta otros protocolos que no necesitamos.

### 2.3 Crear los 4 ficheros de configuración de NUT

Genera primero una contraseña interna para que el cliente NUT lea al `upsd`. **Esta NO es la del token del puente, es solo interna de NUT**:

```bash
NUT_PASS=$(openssl rand -hex 16)
echo "NUT_PASS: $NUT_PASS"
```

Anota ese valor (la usarás dentro de `upsd.users`).

Luego crea los 4 configs en bloque:

```bash
# nut.conf: dice a NUT que actúe como servidor de red
echo 'MODE=netserver' > /etc/nut/nut.conf

# upsd.conf: el daemon NUT escucha SOLO en localhost (no expuesto a red)
cat > /etc/nut/upsd.conf << 'EOF'
LISTEN 127.0.0.1 3493
MAXAGE 15
EOF

# upsd.users: usuario monitor con permisos de solo lectura
cat > /etc/nut/upsd.users << EOF
[monitor]
    password = ${NUT_PASS}
    upsmon primary
EOF
chmod 640 /etc/nut/upsd.users
chown root:nut /etc/nut/upsd.users

# ups.conf: define el SAI por vendorid+productid (no por puerto USB)
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

> **¿Por qué `vendorid + productid` en vez de un puerto USB?** Si reordenas cables o reinicias, el puerto USB puede cambiar; el ID de fabricante y producto no. Con esto el SAI siempre se identifica sin importar el orden de conexión.

### 2.4 Arrancar NUT y verificar

```bash
systemctl restart nut-server
sleep 5
systemctl status nut-server --no-pager
```

Salida esperada (clave: `Connected to UPS [sai1]`):
```
● nut-server.service - Network UPS Tools - power devices information server
     Active: active (running) since ...
   Main PID: ...
   ...
   nut-server[...]: Connected to UPS [sai1]: usbhid-ups-sai1
   nut-server[...]: listening on 127.0.0.1 port 3493
```

Comprueba que NUT lee datos del SAI:

```bash
upsc sai1
```

Debe listar variables como `battery.charge: 100`, `ups.status: OL`, `input.voltage: 218.5`. Si ves `ups.status: OL` significa "On Line" (funcionando con red eléctrica) — perfecto.

> **Error frecuente**: `upsc sai1` da `Error: Driver not connected` justo tras arrancar. El driver tarda unos segundos. Espera 5 y repite. Si persiste:
> ```bash
> systemctl list-units | grep nut
> # nut-driver@sai1.service debe estar 'active running'
> ```

---

## Paso 3 — Subir el binario del puente a la Pi

El proyecto incluye `sai-monitor-arm64` precompilado. No hace falta instalar Go ni compilar nada.

**En el Mac**, desde la carpeta del repo:

```bash
cd ~/Documents/PNut/orange-pi
scp pi/bridge/sai-monitor-arm64 root@IP_DE_LA_PI:/tmp/
```

Sustituye `IP_DE_LA_PI` por la IP real de tu Orange Pi (la misma que usas para SSH).

> **Error frecuente en scp**: `WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED`. Ocurre al reflashear la SD (la Pi tiene clave SSH nueva). Solución:
> ```bash
> ssh-keygen -R IP_DE_LA_PI
> scp pi/bridge/sai-monitor-arm64 root@IP_DE_LA_PI:/tmp/
> ```

### Si necesitas recompilar (solo si modificas `main.go`)

```bash
brew install go      # solo si no lo tienes
cd ~/Documents/PNut/orange-pi/pi/bridge
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o sai-monitor-arm64 .

file sai-monitor-arm64
# Debe decir: ELF 64-bit LSB executable, ARM aarch64, statically linked
```

---

## Paso 4 — Instalar el puente en la Pi

### 4.1 Instalar el binario y crear usuario sin privilegios

En la Pi (SSH como root):

```bash
install -m 0755 /tmp/sai-monitor-arm64 /usr/local/bin/sai-monitor
useradd --system --no-create-home --shell /usr/sbin/nologin -g nut saibridge
mkdir -p /etc/sai-monitor
```

> **Error frecuente**: `useradd: user 'saibridge' already exists`. Comprueba con `id saibridge`; si el `gid` es `nut`, ya está bien y continúa.

### 4.2 Generar token y password de enrollment

El puente tiene dos credenciales independientes:

| Credencial | Para qué |
|---|---|
| `BRIDGE_TOKEN` | Token HTTP Bearer que el dashboard usa en cada petición |
| `BRIDGE_ENROLLMENT_PASSWORD` | Contraseña que tú eliges. El dashboard la pide una vez tras descubrir el SAI; si es correcta, el puente devuelve el token automáticamente |

Genera ambos:

```bash
TOKEN=$(openssl rand -hex 32)
ENROLL_PASS="cambia-esta-password"   # mínimo 8 caracteres, SIN comillas
echo "TOKEN: $TOKEN"
echo "ENROLL_PASS: $ENROLL_PASS"
```

> **¡Cuidado con las comillas tipográficas!** Si copias `"123456789"` desde una app del Mac con autocorrect, las comillas pueden ser curvas Unicode (`“ ”`) en vez de rectas ASCII (`" "`). En bash quedan como parte del valor. Resultado: la password real almacenada incluye las comillas y nunca matchea cuando la escribes en el dashboard. Solución: **no uses comillas** o escribe el valor directamente en la terminal.

Guarda el TOKEN en un sitio seguro por si quieres usar el método manual más adelante.

### 4.3 Crear el fichero de entorno

```bash
cat > /etc/sai-monitor/sai-monitor.env << EOF
BRIDGE_LISTEN=:49152
BRIDGE_TOKEN=${TOKEN}
BRIDGE_NAME=SAI Salón
BRIDGE_ENROLLMENT_PASSWORD=${ENROLL_PASS}
NUT_ADDR=127.0.0.1:3493
BRIDGE_CACHE_TTL=1s
BRIDGE_ORIGINS=http://localhost:5500
EOF

chmod 600 /etc/sai-monitor/sai-monitor.env
chown root:nut /etc/sai-monitor/sai-monitor.env
cat /etc/sai-monitor/sai-monitor.env
```

**Qué hace cada variable:**

| Variable | Función |
|---|---|
| `BRIDGE_LISTEN` | Puerto HTTP del puente. `49152` está en el rango privado IANA, raramente bloqueado por proxies |
| `BRIDGE_TOKEN` | Token Bearer para autenticación HTTP. 64 hex chars |
| `BRIDGE_NAME` | Nombre que el SAI muestra en el autodescubrimiento. Pon uno descriptivo si tienes varios SAIs (ej. "SAI Salón", "SAI Rack") |
| `BRIDGE_ENROLLMENT_PASSWORD` | Opcional. Si está, habilita el botón "Conectar con contraseña" del dashboard. Si está vacía, solo método manual |
| `NUT_ADDR` | Dirección del `upsd` local. Siempre `127.0.0.1:3493` |
| `BRIDGE_CACHE_TTL` | Cuánto cachea el puente cada respuesta. 1 segundo es suficiente |
| `BRIDGE_ORIGINS` | Orígenes CORS permitidos. `http://localhost:5500` cubre el `serve.py` del Mac |

---

## Paso 5 — Servicio systemd

### 5.1 Subir el unit file

En el Mac:

```bash
scp pi/deploy/systemd/sai-monitor.service root@IP_DE_LA_PI:/etc/systemd/system/
```

### 5.2 Habilitar y arrancar

En la Pi:

```bash
systemctl daemon-reload
systemctl enable --now sai-monitor
sleep 2
systemctl status sai-monitor --no-pager
```

Salida esperada:
```
● sai-monitor.service - Puente NUT->HTTP (solo lectura) para monitor de SAI
     Active: active (running)
     Memory: 2.2M (max: 64.0M ...)
     ...
   sai-monitor[...]: escuchando HTTP en :49152 (NUT=127.0.0.1:3493) — sin TLS
   sai-monitor[...]: ssdp: anunciando "SAI Salón" en http://192.168.0.X:49152
```

> **Error frecuente**: el log muestra `ssdp: no se detectó IP LAN, autodescubrimiento desactivado` aunque la Pi tenga IP. Causa: el unit file restringe `AF_NETLINK`, que es lo que Go usa internamente para listar interfaces de red.
>
> Solución (ya aplicada en el unit del repo desde junio 2026):
> ```bash
> sed -i 's|RestrictAddressFamilies=AF_INET AF_INET6|RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK|' /etc/systemd/system/sai-monitor.service
> systemctl daemon-reload
> systemctl restart sai-monitor
> journalctl -u sai-monitor -n 5 --no-pager
> ```
> Debe aparecer `ssdp: anunciando "..." en http://IP:49152`.

> **Error frecuente**: `Failed to enable unit: Unit file is masked`. El servicio quedó enmascarado de una instalación anterior:
> ```bash
> systemctl unmask sai-monitor
> # repetir el scp y luego daemon-reload + enable
> ```

> **Error sobre `ProtectSystem=strict`**: el `.env` queda en `/etc` en solo lectura para el proceso. Para editarlo con el servicio corriendo:
> ```bash
> systemctl stop sai-monitor
> # editar /etc/sai-monitor/sai-monitor.env
> systemctl start sai-monitor
> ```

---

## Paso 6 — Verificar el puente

En la Pi:

```bash
curl -s -o /dev/null -w "healthz: %{http_code}\n" http://127.0.0.1:49152/healthz
curl -s -o /dev/null -w "sin token: %{http_code}\n" http://127.0.0.1:49152/api/ups
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:49152/api/ups
```

Salidas esperadas:
```
healthz: 200
sin token: 401
[{"name":"sai1","description":"Salicru 850"}]
```

Si todo da estos resultados, el puente funciona y el sistema está listo.

---

## Paso 7 — Dashboard en el Mac

En el Mac, desde la carpeta del proyecto:

```bash
cd ~/Documents/PNut/orange-pi
python3 client/scripts/serve.py
```

Se abre el navegador en `http://localhost:5500`. En el dashboard:

1. Pestaña **Equipos**
2. Pulsa **"Buscar SAIs en la red"**
3. Aparece tu Pi: `SAI Salón` con la URL `http://192.168.0.X:49152`
4. Dos opciones:
   - **"Conectar con contraseña"** (recomendado): introduces la `BRIDGE_ENROLLMENT_PASSWORD` que pusiste en el `.env` y el token se obtiene automáticamente
   - **"Añadir manualmente"**: pegas el `BRIDGE_TOKEN` a mano (útil si no configuraste enrollment)
5. El SAI aparece en "SAIs configurados"
6. Pestaña **Monitorización**: ves los datos en vivo (carga batería, autonomía, tensión, etc.) actualizándose cada 5 segundos

> **Error frecuente**: `Puerto 5500 ocupado`. El script te da el comando exacto para liberarlo. Manual:
> ```bash
> kill $(lsof -ti:5500)
> python3 client/scripts/serve.py
> ```

> **Error frecuente al "Conectar con contraseña"**: "Contraseña incorrecta" aunque pongas la correcta. Revisa si en el `.env` quedó con comillas tipográficas Unicode. Solución:
> ```bash
> sed -i 's/BRIDGE_ENROLLMENT_PASSWORD=.*/BRIDGE_ENROLLMENT_PASSWORD=tu-password-limpia/' /etc/sai-monitor/sai-monitor.env
> systemctl restart sai-monitor
> ```

---

## Errores conocidos y soluciones

| Error | Causa | Solución |
|---|---|---|
| `Illegal instruction` en `apt` | Imagen Armbian nightly/trunk rota | Reflashear con la stable 24.8.1 |
| `upsd disabled, please adjust the configuration` | `nut.conf` vacío o con `MODE=none` | `echo 'MODE=netserver' > /etc/nut/nut.conf` |
| `Error: Driver not connected` en `upsc` | Driver NUT inicializándose | Esperar 5 segundos y repetir |
| Binario Go da `SIGSEGV` | Imagen rota o `MemoryDenyWriteExecute=true` en armv7 | Reflashear o quitar la línea del unit |
| `ssdp: no se detectó IP LAN` aunque la Pi tenga IP | Unit restringía `AF_NETLINK` | Añadir `AF_NETLINK` a `RestrictAddressFamilies` (ya corregido en el repo) |
| "Contraseña incorrecta" en el dashboard con la password correcta | Comillas tipográficas Unicode en el `.env` | Reescribir el valor sin comillas |
| `chown: invalid group: 'root:saibridge'` | Bug en Armbian con usuarios de sistema | Usar GID numérico: `id saibridge` y `chown 0:GID fichero` |
| `sed -i: Read-only file system` editando el `.env` | `ProtectSystem=strict` activo | `systemctl stop sai-monitor` antes de editar |
| `Unit file is masked` | Servicio enmascarado de instalación anterior | `systemctl unmask sai-monitor` |
| `WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED` en scp | SD reflasheada, clave SSH nueva | `ssh-keygen -R IP_DE_LA_PI` |
| `Address already in use` en `python3 serve.py` | Puerto 5500 ya ocupado | `kill $(lsof -ti:5500)` |
| SD card en solo lectura | SD defectuosa | Reemplazar SD |

---

## Limitaciones del Salicru SPS 850

- **`ups.realpower` no reportado**: el firmware no expone los vatios reales, solo `ups.realpower.nominal` (490W máximo). El dashboard estima el consumo como `ups.load% × 490W`.
- **`ups.load = 0` durante carga de batería**: en estado `OL CHRG` el driver reporta 0% de carga aunque haya consumo. Normal; se recupera al terminar la carga.
- **`ups.serial` vacío**: el firmware no lo expone.
- **`output.voltage` reporta ~14V**: es la tensión interna de la batería (12V plomo-ácido), no los 230V de salida. Limitación del firmware.

---

## Segunda Pi en adelante

El proceso es idéntico. Tres cosas que **deben** cambiar entre Pis:

1. **Token distinto** por Pi (`openssl rand -hex 32`): así puedes revocar uno sin afectar a los demás
2. **Password de enrollment distinta** (opcional pero recomendado)
3. **`BRIDGE_NAME` distinto** (ej. "SAI Salón", "SAI Rack", "SAI Garaje"): aparece en el autodescubrimiento del dashboard

El nombre `sai1` dentro de `ups.conf` puede repetirse — cada Pi es un dominio independiente.

Si el dashboard se sirve desde el mismo origen (`http://localhost:5500`), `BRIDGE_ORIGINS` es idéntico en todas las Pis.
