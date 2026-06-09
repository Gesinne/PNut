# PNut — Monitor SAI con NUT

> **🤖 INSTRUCCIONES PARA CLAUDE / AGENTES IA**
>
> Este repositorio contiene **dos proyectos hermanos pero independientes**. Antes de leer código, editar archivos o proponer cambios, **debes preguntar al usuario con cuál de los dos quiere continuar**:
>
> 1. **`orange-pi/`** — Variante ligera (Go + HTML vanilla) para Orange Pi Zero Plus y hardware ARM64 con poca RAM.
> 2. **`raspberry-pi/`** — Variante completa (Python Flask + React) para Raspberry Pi 3/4/5 o x86.
>
> No mezcles archivos, dependencias ni decisiones de diseño entre las dos carpetas. Cada una es autocontenida. Espera la respuesta del usuario antes de actuar.
>
> Pregunta sugerida: *"¿Quieres continuar con el proyecto de la Orange Pi (Go ligero) o con el de la Raspberry Pi (Flask + React)?"*

---

Dos variantes para monitorizar SAIs (UPS) con [Network UPS Tools](https://networkupstools.org/) según el hardware disponible.

| Carpeta | Hardware | Stack | Cuándo usarla |
|---|---|---|---|
| [`orange-pi/`](orange-pi/) | Orange Pi Zero Plus · Raspberry Pi Zero 2 W · ARM64 con poca RAM | Go (stdlib) + HTML vanilla | Homelab con recursos limitados. Binario estático 6 MB, dashboard sin build. |
| [`raspberry-pi/`](raspberry-pi/) | Raspberry Pi 3/4/5 · x86 con interfaz completa | Python Flask + React | Hardware más potente, UI rica con polling y vista de variables. |

Cada carpeta es autocontenida: tiene su `README.md`, código y configuración. Elige una y trabaja desde ahí.

## Descargas

[Releases](../../releases) incluye binarios precompilados de la variante Orange Pi (ARM64).
