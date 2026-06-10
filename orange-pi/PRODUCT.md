# Product

## Register

product

## Users

Homelab sysadmin en entorno doméstico. Accede desde Mac o móvil vía LAN. Contexto de uso: comprobaciones rápidas del estado del SAI, no monitorización de misión crítica. Trabajo técnico, conocimiento del sistema.

## Product Purpose

Dashboard de solo lectura para monitorizar SAIs (UPS) conectados a una Orange Pi vía NUT. Muestra estado en tiempo real, variables del hardware, y gráfica histórica de batería y consumo. Éxito: abrir la URL y saber en 2 segundos si el SAI está bien.

## Brand Personality

Moderna · Clara · Profesional. Herramienta que se mantiene fuera del camino. El dato es el protagonista, no la interfaz.

## Anti-references

- Home Assistant: demasiado domótico, paleta recargada, sensación de panel de juguete.
- Grafana: densidad oscura de sysadmin industrial, demasiado complejo para un solo SAI.
- Admin Bootstrap genérico: tablas azules, sidebar pesada, diseño de 2015.

## Design Principles

1. **El dato manda.** La jerarquía visual prioriza el estado y la batería sobre todo lo demás.
2. **Desaparece.** La interfaz no debe llamar la atención; debe ejecutar la tarea y retirarse.
3. **Consistencia sobre sorpresa.** Mismo vocabulario visual en todas las partes del dashboard.
4. **Feedback inmediato.** Cada acción tiene respuesta visual; ningún estado queda sin representar.
5. **Zero-trust en la red.** El descubrimiento SSDP encuentra la URL del puente, pero el token Bearer es obligatorio en cada petición HTTP. La conveniencia de UX nunca relaja la autenticación.

## Accessibility & Inclusion

WCAG AA. Soporte teclado completo. Respeto a `prefers-reduced-motion`. Contraste mínimo 4.5:1 en texto normal. Indicadores de foco visibles.
