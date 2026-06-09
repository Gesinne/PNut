# Instrucciones para Claude — Repo PNut

Este repositorio contiene **dos proyectos hermanos pero independientes**:

- **`orange-pi/`** — Go (stdlib) + HTML vanilla. Hardware ARM64 ligero (Orange Pi Zero Plus, Pi Zero 2 W).
- **`raspberry-pi/`** — Python Flask + React. Hardware potente (Raspberry Pi 3/4/5, x86).

## Regla obligatoria al iniciar sesión

Antes de leer código, editar archivos, proponer cambios o investigar más allá del README raíz, **pregunta explícitamente al usuario con cuál de los dos proyectos quiere trabajar en esta sesión**.

Pregunta sugerida:

> ¿Quieres continuar con el proyecto de la **Orange Pi** (Go ligero) o el de la **Raspberry Pi** (Flask + React)?

Espera la respuesta antes de actuar. No asumas por el nombre del directorio raíz (`Sai Orange Pi`) que el usuario quiere la variante Orange Pi — el repo cubre ambas.

## Una vez elegido el proyecto

- Trabaja **solo dentro de la carpeta elegida**. No mezcles archivos, dependencias, decisiones de diseño ni patrones entre las dos.
- Cada carpeta es autocontenida: tiene su propio `README.md`, `PRODUCT.md` y, en `orange-pi/`, además `INSTALACION.md`.
- Lee primero esos archivos de la variante elegida antes de tocar código.

## Excepciones (no requieren preguntar)

- El usuario ya indicó la variante en el mensaje actual (ej. menciona "orange", "Go", "raspberry", "Flask", "React").
- La tarea afecta solo al README raíz o a este `CLAUDE.md`.
- El usuario pide una comparación entre ambas variantes.
