#!/usr/bin/env python3
"""
NUT UPS Monitor — Flask API
Serves UPS data from Network UPS Tools via upsc.

Install: pip install flask flask-cors
Run:     python3 api.py
"""

from flask import Flask, jsonify, send_from_directory
from flask_cors import CORS
import subprocess
import os
import logging

app = Flask(__name__)
CORS(app)
logging.basicConfig(level=logging.INFO)

# ── NUT config ────────────────────────────────────────────────────────────────
# Run `upsc -l` to find your UPS name, e.g. "ups", "salicru", "sps850"
UPS_NAME = "salicru@localhost"

def run_upsc(ups_name=UPS_NAME):
    """Run upsc and return dict of key→value pairs."""
    try:
        result = subprocess.run(
            ["upsc", ups_name],
            capture_output=True, text=True, timeout=5
        )
        data = {}
        for line in result.stdout.splitlines():
            if ":" in line:
                k, _, v = line.partition(":")
                data[k.strip()] = v.strip()
        return data, None
    except FileNotFoundError:
        return {}, "upsc not found — is NUT installed?"
    except subprocess.TimeoutExpired:
        return {}, "upsc timed out"
    except Exception as e:
        return {}, str(e)


def safe_float(d, key, default=None):
    try:
        return float(d[key])
    except (KeyError, ValueError, TypeError):
        return default


def safe_int(d, key, default=None):
    v = safe_float(d, key)
    return int(v) if v is not None else default


# ── API endpoint ──────────────────────────────────────────────────────────────
@app.route("/api/ups")
def ups():
    d, err = run_upsc()

    if err:
        app.logger.warning("NUT error: %s", err)
        return jsonify({"error": err}), 503

    # battery.runtime comes in seconds → convert to minutes
    runtime_s = safe_int(d, "battery.runtime")
    runtime_min = round(runtime_s / 60) if runtime_s is not None else None

    payload = {
        # Battery
        "battery_charge":    safe_int(d,   "battery.charge"),
        "battery_voltage":   safe_float(d, "battery.voltage"),
        "battery_runtime":   runtime_min,

        # Input
        "input_voltage":     safe_float(d, "input.voltage"),
        "input_frequency":   safe_float(d, "input.frequency"),

        # Output
        "output_voltage":    safe_float(d, "output.voltage"),
        "output_frequency":  safe_float(d, "output.frequency"),

        # UPS
        "ups_load":          safe_int(d,   "ups.load"),
        "ups_status":        d.get("ups.status", "OL"),
        "ups_temperature":   safe_float(d, "ups.temperature"),

        # Device info
        "ups_model":         d.get("ups.model"),
        "ups_mfr":           d.get("ups.mfr"),
        "ups_firmware":      d.get("ups.firmware"),
        "ups_serial":        d.get("ups.serial"),

        # Power
        "ups_realpower_nom": safe_int(d, "ups.realpower.nominal"),

        # Battery thresholds
        "battery_charge_low": safe_int(d, "battery.charge.low"),
        "battery_runtime_low": round(safe_int(d, "battery.runtime.low", 0) / 60),
    }

    return jsonify(payload)


@app.route("/api/ups/raw")
def ups_raw():
    """Full dump of all NUT variables — useful to see what your UPS supports."""
    d, err = run_upsc()
    if err:
        return jsonify({"error": err}), 503
    return jsonify(d)


# ── Serve React frontend ──────────────────────────────────────────────────────
FRONTEND_DIST = os.path.join(os.path.dirname(__file__), "frontend", "dist")

@app.route("/", defaults={"path": ""})
@app.route("/<path:path>")
def serve_frontend(path):
    if os.path.exists(FRONTEND_DIST):
        target = os.path.join(FRONTEND_DIST, path)
        if path and os.path.isfile(target):
            return send_from_directory(FRONTEND_DIST, path)
        return send_from_directory(FRONTEND_DIST, "index.html")
    return jsonify({"status": "API running", "frontend": "not built yet"}), 200


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000, debug=False)
