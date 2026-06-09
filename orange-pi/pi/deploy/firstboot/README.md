# Bootstrap automático en la Orange Pi

Instala el puente PNut y configura NUT **automáticamente** cada vez que arranca la Pi. Detecta el SAI USB conectado, escribe la configuración correcta de NUT y arranca los servicios. Si el SAI no cambia entre arranques, no hace nada. Si conectas un SAI distinto y reinicias, reescribe la configuración solo.

Pensado para hardware con poca RAM: nada corre en segundo plano. Todo el trabajo ocurre una vez por arranque y termina en segundos.

---

## Qué hace exactamente

En cada arranque, `sai-bootstrap.service` (oneshot) ejecuta `/usr/local/sbin/sai-bootstrap.sh`. El script:

1. Instala `nut`, `usbutils`, `openssl` si faltan (silencioso si ya están).
2. Crea el usuario `saibridge` (idempotente).
3. Genera **una sola vez** y guarda en `/etc/sai-monitor/.creds`:
   - `BRIDGE_TOKEN` — token Bearer para el dashboard
   - `BRIDGE_ENROLLMENT_PASSWORD` — contraseña para el "Conectar con contraseña" del dashboard (o usa la que pongas en `/boot/sai-bootstrap.conf`)
   - `NUT_PASS` — contraseña interna de NUT
4. Lanza `nut-scanner -U` durante hasta 30 s esperando que aparezca el SAI USB.
5. Calcula una huella `vendorid:productid:driver` y la compara con `/var/lib/sai-monitor/sai.fingerprint`:
   - **Igual al previo** → sale en milisegundos. Nada que hacer.
   - **Distinto o primera vez** → reescribe los 4 ficheros de NUT con los IDs detectados, escribe `/etc/sai-monitor/sai-monitor.env`, reinicia `nut-server` y `sai-monitor`, actualiza la huella.
   - **Sin SAI tras 30 s** → log y exit 0. No marca nada, así el siguiente reinicio vuelve a intentarlo.

Las credenciales (token, password de enrollment) **persisten** entre cambios de SAI. Cambiar de SAI no te obliga a re-emparejar el dashboard.

---

## Instalación (3 pasos)

### 1. Flashea Armbian en la SD

Usa la imagen `Armbian_24.8.1_Orangepizeroplus_bookworm_current_6.6.44_minimal.img.xz`. **No uses nightly/trunk** — `apt` viene roto. Detalles en [`../../INSTALACION.md`](../../INSTALACION.md#imagen-de-armbian).

### 2. Copia este directorio a la Pi y ejecuta `install.sh`

Arranca la Pi, conéctate por SSH como root y, desde el Mac:

```bash
cd ~/Documents/Sai\ Orange\ Pi/orange-pi
scp -r pi/deploy/firstboot pi/bridge/sai-monitor-arm64 root@IP_PI:/tmp/
ssh root@IP_PI 'cd /tmp/firstboot && ./install.sh'
```

`install.sh` copia el script, el binario y el unit a sus rutas, habilita `sai-bootstrap.service` y `sai-monitor.service`.

### 3. (Opcional) Preconfigura nombre y password

Si no quieres que la password de enrollment se genere aleatoria, créala antes del primer arranque:

```bash
ssh root@IP_PI 'cp /tmp/firstboot/sai-bootstrap.conf.example /boot/sai-bootstrap.conf && nano /boot/sai-bootstrap.conf'
```

Edita `BRIDGE_NAME`, `BRIDGE_ENROLLMENT_PASSWORD`, `BRIDGE_ORIGINS`. **No uses comillas tipográficas Unicode** (`"` `"` `'` `'`) — el script las elimina pero mejor evitarlas.

### 4. Reinicia (o lanza ya) el bootstrap

```bash
ssh root@IP_PI 'systemctl reboot'
# o, sin reiniciar:
# ssh root@IP_PI 'systemctl start sai-bootstrap.service'
```

Cuando vuelva, el SAI debe aparecer en el dashboard al pulsar "Buscar SAIs en la red".

---

## Ver qué pasó

```bash
ssh root@IP_PI
journalctl -u sai-bootstrap.service -n 100 --no-pager
cat /var/log/sai-bootstrap.log
cat /etc/sai-monitor/.creds        # token + password (modo 600)
upsc sai1                          # variables del SAI vía NUT
systemctl status sai-monitor
```

---

## Casos de uso

| Situación | Qué ocurre |
|---|---|
| Primer arranque, SAI ya conectado | Detecta → escribe configs → arranca todo |
| Primer arranque, SAI no conectado | Log "no se detectó SAI". Conecta y reinicia |
| Reinicios siguientes con el mismo SAI | Bootstrap sale en < 1 s sin tocar nada |
| Cambias de SAI y reinicias | Detecta huella distinta → reescribe `ups.conf` → reinicia NUT + puente |
| Cambias de SAI **sin** reiniciar | No detectado hasta el próximo reinicio (por diseño: menos RAM, sin polling) |
| Reflasheas la SD | Vuelves a ejecutar `install.sh`. Las credenciales nuevas obligan a re-emparejar el dashboard |

---

## Reinstalar / desinstalar

```bash
# desinstalar
systemctl disable --now sai-bootstrap.service sai-monitor.service nut-server
rm -f /usr/local/sbin/sai-bootstrap.sh \
      /usr/local/lib/sai-monitor/sai-monitor.bin \
      /usr/local/bin/sai-monitor \
      /etc/systemd/system/sai-bootstrap.service \
      /etc/systemd/system/sai-monitor.service
rm -rf /etc/sai-monitor /var/lib/sai-monitor
systemctl daemon-reload
```

Si solo quieres regenerar credenciales o que el bootstrap "olvide" el SAI:

```bash
rm /etc/sai-monitor/.creds          # fuerza regenerar token y passwords
rm /var/lib/sai-monitor/sai.fingerprint  # fuerza reescribir configs en el próximo boot
systemctl restart sai-bootstrap.service
```

---

## Si algo va mal

Mira [`../../INSTALACION.md` § Errores conocidos y soluciones](../../INSTALACION.md#errores-conocidos-y-soluciones). Los mismos errores aplican aquí (`Illegal instruction`, `Driver not connected`, `Unit file is masked`, comillas Unicode, etc.).
