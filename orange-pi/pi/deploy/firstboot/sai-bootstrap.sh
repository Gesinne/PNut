#!/usr/bin/env bash
# PNut Orange Pi — bootstrap automático del SAI.
# Corre en cada arranque vía sai-bootstrap.service. Idempotente:
#   - SAI USB ausente  -> log + exit 0 (sin tocar nada)
#   - SAI igual al previo -> exit 0 (sin tocar nada)
#   - SAI distinto / primera vez -> reescribe ups.conf, reinicia NUT + puente
# Credenciales (token, password enrollment, password interna NUT) se generan
# una sola vez y persisten en /etc/sai-monitor/.creds para sobrevivir a
# cambios de SAI sin romper el pairing del dashboard.

set -euo pipefail

LOG=/var/log/sai-bootstrap.log
STATE_DIR=/var/lib/sai-monitor
FP_FILE="${STATE_DIR}/sai.fingerprint"
CREDS=/etc/sai-monitor/.creds
ENV_FILE=/etc/sai-monitor/sai-monitor.env
BOOT_CONF=/boot/sai-bootstrap.conf
BIN_SRC=/usr/local/lib/sai-monitor/sai-monitor.bin
BIN_DST=/usr/local/bin/sai-monitor
LOCK=/var/lock/sai-bootstrap.lock
USB_WAIT_MAX=30
USB_WAIT_STEP=3

log() { printf '%s [sai-bootstrap] %s\n' "$(date -Is)" "$*" | tee -a "$LOG" >&2; }
die() { log "ERROR: $*"; exit 1; }

[[ $EUID -eq 0 ]] || die "requiere root"

# Lock para evitar carrera si systemd dispara dos veces.
exec 9>"$LOCK"
flock -n 9 || { log "ya hay otro bootstrap en curso, saliendo"; exit 0; }

mkdir -p "$STATE_DIR" /etc/sai-monitor /usr/local/lib/sai-monitor
chmod 750 /etc/sai-monitor

# Sanea valor de variables que vienen de /boot/sai-bootstrap.conf:
# elimina comillas envolventes y comillas tipográficas Unicode (bug
# documentado en INSTALACION.md).
sanitize() {
  local v="${1-}"
  v="${v//$'\r'/}"
  v="${v//$'\xe2\x80\x98'/}"  # '
  v="${v//$'\xe2\x80\x99'/}"  # '
  v="${v//$'\xe2\x80\x9c'/}"  # "
  v="${v//$'\xe2\x80\x9d'/}"  # "
  v="${v#\"}"; v="${v%\"}"
  v="${v#\'}"; v="${v%\'}"
  printf '%s' "$v"
}

load_boot_conf() {
  BRIDGE_NAME_DEFAULT="SAI Orange Pi"
  BRIDGE_ORIGINS_DEFAULT="http://localhost:5500"
  CONF_BRIDGE_NAME=""
  CONF_BRIDGE_ENROLLMENT_PASSWORD=""
  CONF_BRIDGE_ORIGINS=""
  if [[ -r "$BOOT_CONF" ]]; then
    log "leyendo $BOOT_CONF"
    local line k v
    while IFS= read -r line; do
      k="${line%%=*}"
      v="${line#*=}"
      k="${k// /}"
      [[ -z "$k" || "${k:0:1}" == "#" ]] && continue
      v="$(sanitize "$v")"
      case "$k" in
        BRIDGE_NAME) CONF_BRIDGE_NAME="$v" ;;
        BRIDGE_ENROLLMENT_PASSWORD) CONF_BRIDGE_ENROLLMENT_PASSWORD="$v" ;;
        BRIDGE_ORIGINS) CONF_BRIDGE_ORIGINS="$v" ;;
      esac
    done < "$BOOT_CONF"
  fi
}

ensure_packages() {
  local missing=()
  for p in nut usbutils openssl; do
    dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p")
  done
  if (( ${#missing[@]} )); then
    log "instalando paquetes: ${missing[*]}"
    # Bloquea arranque de servicios en postinst (deadlock si corre dentro de systemd)
    echo "exit 101" > /usr/sbin/policy-rc.d
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${missing[@]}" || {
      rm -f /usr/sbin/policy-rc.d
      die "apt-get install falló"
    }
    rm -f /usr/sbin/policy-rc.d
  fi
}

ensure_user() {
  if ! id saibridge >/dev/null 2>&1; then
    log "creando usuario saibridge"
    useradd --system --no-create-home --shell /usr/sbin/nologin -g nut saibridge
  fi
}

ensure_credentials() {
  if [[ -r "$CREDS" ]]; then
    # shellcheck disable=SC1090
    source "$CREDS"
  fi
  local generated=0
  if [[ -z "${BRIDGE_TOKEN-}" ]]; then
    BRIDGE_TOKEN="$(openssl rand -hex 32)"; generated=1
  fi
  if [[ -z "${NUT_PASS-}" ]]; then
    NUT_PASS="$(openssl rand -hex 16)"; generated=1
  fi
  if [[ -z "${BRIDGE_ENROLLMENT_PASSWORD-}" ]]; then
    if [[ -n "$CONF_BRIDGE_ENROLLMENT_PASSWORD" ]]; then
      BRIDGE_ENROLLMENT_PASSWORD="$CONF_BRIDGE_ENROLLMENT_PASSWORD"
    else
      BRIDGE_ENROLLMENT_PASSWORD="$(openssl rand -base64 12 | tr -d '/+=' | cut -c1-16)"
    fi
    generated=1
  fi
  if (( generated )); then
    # Pre-crea con permisos finales antes de escribir para que el contenido
    # nunca sea visible con permisos más laxos.
    install -m 0600 -o root -g root /dev/null "$CREDS"
    cat > "$CREDS" <<EOF
BRIDGE_TOKEN=$BRIDGE_TOKEN
NUT_PASS=$NUT_PASS
BRIDGE_ENROLLMENT_PASSWORD=$BRIDGE_ENROLLMENT_PASSWORD
EOF
    log "credenciales generadas en $CREDS (lee con: sudo cat $CREDS)"
  fi
}

# Espera a que aparezca un SAI USB (la enumeración tras boot puede tardar
# varios segundos). Devuelve 0 con bloque [nutdevN] en stdout, 1 si vacío.
scan_ups() {
  local out elapsed=0
  while (( elapsed < USB_WAIT_MAX )); do
    out="$(nut-scanner -U 2>/dev/null || true)"
    if grep -q '^\[nutdev' <<<"$out"; then
      printf '%s\n' "$out"
      return 0
    fi
    sleep "$USB_WAIT_STEP"
    elapsed=$(( elapsed + USB_WAIT_STEP ))
  done
  return 1
}

# Aísla el primer bloque [nutdevN] y extrae key=value de driver/port/vendorid/
# productid/product/vendor. Loguea cuántos bloques había.
parse_first_ups() {
  local scan="$1"
  local total
  total=$(grep -c '^\[nutdev' <<<"$scan")
  if (( total > 1 )); then
    log "detectados $total SAIs, uso el primero; resto ignorado"
  fi
  # Aísla líneas del primer bloque con awk básico (portable mawk),
  # extrae key="value" con sed.
  awk '/^\[nutdev[0-9]+\]/ { count++; if (count > 1) exit; next } count == 1' <<<"$scan" \
    | sed -n 's/^[[:space:]]*\([a-z][a-z]*\)[[:space:]]*=[[:space:]]*"\(.*\)"[[:space:]]*$/\1=\2/p'
}

write_nut_configs() {
  local driver="$1" vendorid="$2" productid="$3" product="$4"
  local desc="${product:-SAI}"
  log "escribiendo configs NUT (driver=$driver vendorid=$vendorid productid=$productid)"

  # nut.conf no contiene secretos; escritura directa con permisos correctos.
  printf 'MODE=netserver\n' > /etc/nut/nut.conf
  chmod 0644 /etc/nut/nut.conf
  chown root:nut /etc/nut/nut.conf

  # Pre-crea con permisos finales antes de escribir para evitar ventana
  # donde upsd.conf/users/ups.conf sean legibles por otros.
  install -m 0640 -o root -g nut /dev/null /etc/nut/upsd.conf
  cat > /etc/nut/upsd.conf <<'EOF'
LISTEN 127.0.0.1 3493
MAXAGE 15
EOF

  install -m 0640 -o root -g nut /dev/null /etc/nut/upsd.users
  cat > /etc/nut/upsd.users <<EOF
[monitor]
    password = ${NUT_PASS}
    upsmon primary
EOF

  install -m 0640 -o root -g nut /dev/null /etc/nut/ups.conf
  cat > /etc/nut/ups.conf <<EOF
maxretry = 3
pollinterval = 2

[sai1]
    driver = ${driver}
    port = auto
    vendorid = ${vendorid}
    productid = ${productid}
    desc = "${desc}"
EOF

  # Regla udev específica para este SAI (complementa 62-nut-usbups.rules,
  # garantiza permisos correctos tras instalar NUT sin reiniciar udev).
  local vid_lower
  vid_lower="$(printf '%s' "${vendorid}" | tr '[:upper:]' '[:lower:]')"
  cat > /etc/udev/rules.d/62-nut-sai1.rules <<EOF
SUBSYSTEM=="usb", ATTR{idVendor}=="${vid_lower}", ATTR{idProduct}=="${productid,,}", GROUP="nut", MODE="0660"
EOF
}

write_env_file() {
  local name="${CONF_BRIDGE_NAME:-$BRIDGE_NAME_DEFAULT}"
  local origins="${CONF_BRIDGE_ORIGINS:-$BRIDGE_ORIGINS_DEFAULT}"
  log "escribiendo $ENV_FILE"
  # Pre-crea con permisos finales antes de escribir (contiene BRIDGE_TOKEN).
  install -m 0600 -o root -g nut /dev/null "$ENV_FILE"
  cat > "$ENV_FILE" <<EOF
BRIDGE_LISTEN=:49152
BRIDGE_TOKEN=${BRIDGE_TOKEN}
BRIDGE_NAME=${name}
BRIDGE_ENROLLMENT_PASSWORD=${BRIDGE_ENROLLMENT_PASSWORD}
NUT_ADDR=127.0.0.1:3493
BRIDGE_CACHE_TTL=1s
BRIDGE_ORIGINS=${origins}
EOF
}

install_binary() {
  if [[ ! -x "$BIN_DST" ]] || ! cmp -s "$BIN_SRC" "$BIN_DST"; then
    [[ -r "$BIN_SRC" ]] || die "falta binario en $BIN_SRC (ejecuta install.sh primero)"
    log "instalando binario en $BIN_DST"
    install -m 0755 -o root -g root "$BIN_SRC" "$BIN_DST"
  fi
}

restart_services() {
  systemctl daemon-reload
  systemctl unmask nut-driver@sai1 nut-server sai-monitor 2>/dev/null || true
  systemctl enable nut-driver@sai1 nut-server sai-monitor >/dev/null 2>&1 || true
  # Aplica reglas udev antes de arrancar el driver (necesario tras instalar NUT)
  log "aplicando reglas udev USB"
  udevadm control --reload-rules
  udevadm trigger --subsystem-match=usb
  sleep 2
  log "arrancando driver NUT (nut-driver@sai1)"
  systemctl restart --no-block nut-driver@sai1
  sleep 3
  log "reiniciando nut-server"
  # --no-block: no esperar a que el driver USB inicialice (puede tardar >90s en ARM)
  systemctl restart --no-block nut-server
  # Polling: hasta 60s para que upsd cargue el driver
  local i
  for i in $(seq 1 20); do
    sleep 3
    if upsc sai1 >/dev/null 2>&1; then
      log "upsc sai1 OK (intento $i)"
      break
    fi
    (( i == 20 )) && log "AVISO: upsc sai1 no responde tras 60s, continúa de todos modos"
  done
  log "reiniciando sai-monitor"
  systemctl restart --no-block sai-monitor
}

main() {
  touch "$LOG"; chmod 640 "$LOG"
  log "==== arranque sai-bootstrap ===="

  load_boot_conf
  ensure_packages
  ensure_user
  ensure_credentials

  local scan
  if ! scan="$(scan_ups)"; then
    log "no se detectó SAI USB tras ${USB_WAIT_MAX}s, saliendo (sin tocar nada)"
    exit 0
  fi

  # Parsea primer bloque.
  declare -A f=()
  local line k v
  while IFS='=' read -r k v; do
    [[ -n "$k" ]] && f["$k"]="$v"
  done < <(parse_first_ups "$scan")

  local driver="${f[driver]:-}" vendorid="${f[vendorid]:-}" productid="${f[productid]:-}" product="${f[product]:-}"
  [[ -n "$driver" && -n "$vendorid" && -n "$productid" ]] \
    || die "nut-scanner devolvió bloque sin driver/vendorid/productid (formato inesperado)"

  local fp="${vendorid}:${productid}:${driver}"
  local prev_fp=""
  [[ -r "$FP_FILE" ]] && prev_fp="$(cat "$FP_FILE")"

  if [[ "$fp" == "$prev_fp" ]]; then
    log "SAI sin cambios ($fp), nada que hacer"
    exit 0
  fi

  log "SAI nuevo o distinto (previo='$prev_fp', actual='$fp')"
  install_binary
  write_nut_configs "$driver" "$vendorid" "$productid" "$product"
  write_env_file
  restart_services
  printf '%s\n' "$fp" > "$FP_FILE"
  chmod 600 "$FP_FILE"
  log "==== bootstrap completado ===="
}

main "$@"
