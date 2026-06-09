#!/usr/bin/env bash
# Copia los ficheros del bootstrap a sus rutas finales y habilita el unit.
# Ejecuta esto UNA vez en la Pi (vía SSH como root) tras flashear Armbian.
# Después, en cada arranque, sai-bootstrap.service detecta el SAI y configura.
# Idempotente: relanzarlo no rompe nada.

set -euo pipefail

[[ $EUID -eq 0 ]] || { echo "requiere root" >&2; exit 1; }

SRC="$(cd "$(dirname "$0")" && pwd)"
DEPLOY="$(cd "$SRC/.." && pwd)"
BIN_SRC="${DEPLOY}/../bridge/sai-monitor-arm64"

[[ -r "$BIN_SRC" ]] || { echo "no encuentro $BIN_SRC" >&2; exit 1; }

echo "==> copiando script bootstrap a /usr/local/sbin/"
install -m 0755 -o root -g root "$SRC/sai-bootstrap.sh" /usr/local/sbin/sai-bootstrap.sh

echo "==> copiando binario a /usr/local/lib/sai-monitor/"
mkdir -p /usr/local/lib/sai-monitor
install -m 0755 -o root -g root "$BIN_SRC" /usr/local/lib/sai-monitor/sai-monitor.bin

echo "==> instalando units systemd"
install -m 0644 -o root -g root "$SRC/sai-bootstrap.service" /etc/systemd/system/sai-bootstrap.service
install -m 0644 -o root -g root "$DEPLOY/systemd/sai-monitor.service" /etc/systemd/system/sai-monitor.service

systemctl daemon-reload
systemctl enable sai-bootstrap.service
systemctl unmask sai-monitor.service 2>/dev/null || true
systemctl enable sai-monitor.service

cat <<MSG

Instalación lista.

  - Si quieres preconfigurar nombre/origen/password: copia
    $SRC/sai-bootstrap.conf.example  a  /boot/sai-bootstrap.conf  y edítalo.

  - Reinicia la Pi:  systemctl reboot

  - O lánzalo ya sin reiniciar:  systemctl start sai-bootstrap.service

  - Ver log:    journalctl -u sai-bootstrap.service -f
  - Token y password generados:  sudo cat /etc/sai-monitor/.creds

MSG
