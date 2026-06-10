# Instrucciones para Claude — PNut (rama Orange Pi)

Este árbol de trabajo contiene **solo la variante Orange Pi** del proyecto PNut: puente Go + HTML vanilla pensado para hardware ARM64 con poca RAM (Orange Pi Zero Plus, Pi Zero 2 W).

La variante **Raspberry Pi (Flask + React)** vive en una rama separada llamada `raspberry-pi`, con su propia historia. No está en este árbol.

## Regla al iniciar sesión

- Si el usuario habla de la variante Orange Pi: trabaja directamente, no preguntes.
- Si el usuario menciona Raspberry Pi, Flask, React o el dashboard rico: **dile que esa variante vive en la rama `raspberry-pi`** y propón `git checkout raspberry-pi` antes de seguir. No intentes recrearla aquí.
- Si está ambiguo: pregunta explícitamente cuál.

## Layout del proyecto Orange Pi

- `orange-pi/pi/bridge/` — código Go del puente NUT→HTTP
- `orange-pi/pi/deploy/` — units systemd, configs NUT, env file
- `orange-pi/pi/deploy/firstboot/` — bootstrap automático del SAI (detecta SAI USB en cada arranque, reconfig solo si cambió)
- `orange-pi/client/` — dashboard HTML vanilla servido localmente
- `orange-pi/INSTALACION.md` — guía manual paso a paso
- `orange-pi/PRODUCT.md` — decisiones de diseño

Lee primero el `README.md` y `INSTALACION.md` de `orange-pi/` antes de tocar código.

## Convenciones

- Commits: `feat(orange-pi):`, `fix(orange-pi):`, `docs(orange-pi):`, `refactor(orange-pi):`
- No tocar el binario `sai-monitor-arm64` (recompilarlo solo si se modifica `main.go`)
- No añadir dependencias Go nuevas sin justificación clara
- Idioma por defecto: español
