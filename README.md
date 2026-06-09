# PNut — Monitor SAI con NUT

Dos variantes para monitorizar SAIs (UPS) con [Network UPS Tools](https://networkupstools.org/) según el hardware disponible.

| Carpeta | Hardware | Stack | Cuándo usarla |
|---|---|---|---|
| [`orange-pi/`](orange-pi/) | Orange Pi Zero Plus · Raspberry Pi Zero 2 W · ARM64 con poca RAM | Go (stdlib) + HTML vanilla | Homelab con recursos limitados. Binario estático 6 MB, dashboard sin build. |
| [`raspberry-pi/`](raspberry-pi/) | Raspberry Pi 3/4/5 · x86 con interfaz completa | Python Flask + React | Hardware más potente, UI rica con polling y vista de variables. |

Cada carpeta es autocontenida: tiene su `README.md`, código y configuración. Elige una y trabaja desde ahí.

## Descargas

[Releases](../../releases) incluye binarios precompilados de la variante Orange Pi (ARM64).
