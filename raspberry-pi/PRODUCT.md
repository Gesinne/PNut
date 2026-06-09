---
name: NUT UPS Monitor
description: Real-time web dashboard for monitoring a Salicru SPS 850 Home UPS via Network UPS Tools on a Raspberry Pi
register: product
---

# NUT UPS Monitor

**What it is:** Single-page React dashboard served by Flask on a Raspberry Pi. Reads live UPS telemetry from NUT (`upsc`) and presents it in a web UI.

**Users:** Home user (Guillermo) monitoring a Salicru SPS 850 Home UPS on a local network.

**Purpose:** Instant situational awareness — battery %, charge state, load, input voltage, runtime remaining, NUT flags. Falls back to animated mock data when the Pi is unreachable.

**Stack:** React 19 + Vite + Flask + Python 3 + NUT (Network UPS Tools)

**Language:** Spanish UI labels (Spanish-speaking user, Spanish UPS territory: 230 V / 50 Hz)

**Tone:** Precise, technical, data-dense. A monitoring tool, not a marketing page. Dense information hierarchy with clear status signaling.

**Personality:** Dark glassmorphism (ambient blobs, indigo/blue accent, monospace data) for dark mode; clean white cards with blue accent for light mode. Fira Code for data values, Fira Sans for labels.

**Anti-references:**
- Generic SaaS dashboard (card grids with big gradient numbers, hero metrics)
- Cream/sand background tones
- Flashy neon-on-black gaming aesthetic
- Overly minimal "startup clean" with no data density
