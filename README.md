# PNut — Monitor SAI con NUT (variante Orange Pi)

> **🤖 INSTRUCCIONES PARA CLAUDE / AGENTES IA**
>
> Esta rama contiene **solo la variante Orange Pi** (Go + HTML vanilla, hardware ARM64 ligero).
>
> La variante Raspberry Pi (Python Flask + React) vive en una rama separada llamada **`raspberry-pi`** con su propia historia. Si el usuario menciona Raspberry Pi, Flask, React o el dashboard rico, **redirígelo a esa rama** (`git checkout raspberry-pi`) en lugar de intentar reconstruirla aquí.

---

Puente de **solo lectura** entre NUT (`upsd`) y HTTP/JSON, con dashboard web para monitorizar SAIs (UPS) conectados por USB. Sin dependencias externas: binario Go estático + HTML vanilla.

Optimizada para **hardware ligero** (Orange Pi Zero Plus, Orange Pi Zero 2, Raspberry Pi Zero 2 W; ARM64, menos de 512 MB RAM).

## Estructura

```
orange-pi/                  Código + deploy de esta variante
├── pi/                     Lo que va a la Pi (Go bridge + systemd + NUT configs)
├── client/                 Dashboard HTML que corre en el Mac/navegador
├── INSTALACION.md          Guía manual paso a paso con errores reales
├── PRODUCT.md              Decisiones de diseño y propósito
└── README.md
```

## Instalación

- **Automática (cero comandos tras el primer arranque)**: ver [`orange-pi/pi/deploy/firstboot/README.md`](orange-pi/pi/deploy/firstboot/README.md).
- **Manual (paso a paso, referencia y troubleshooting)**: ver [`orange-pi/INSTALACION.md`](orange-pi/INSTALACION.md).

## Otras ramas

| Rama | Contenido |
|---|---|
| `main` | Esta variante (Orange Pi) estable |
| `dev` | Desarrollo activo de la variante Orange Pi |
| `raspberry-pi` | Variante Raspberry Pi (Python Flask + React). Historia independiente. |

## Descargas

[Releases](../../releases) incluye binarios precompilados ARM64.
