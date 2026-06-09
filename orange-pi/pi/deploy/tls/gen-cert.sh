#!/usr/bin/env bash
# Certificado autofirmado de larga duración para el puente.
# Debes IMPORTAR cert.pem como CA de confianza en cada dispositivo que abra el dashboard.
set -euo pipefail
IP="${1:?uso: gen-cert.sh <IP-de-la-pi>}"
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout key.pem -out cert.pem \
  -subj "/CN=sai-monitor" \
  -addext "subjectAltName=IP:${IP}"
echo "Generados cert.pem y key.pem para IP ${IP}"
