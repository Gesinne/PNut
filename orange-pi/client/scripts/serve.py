#!/usr/bin/env python3
"""Dashboard SAI — http://localhost:5500  (Ctrl+C para parar)"""
import http.server
import json
import os
import socket
import struct
import threading
import time
import webbrowser

PORT      = 5500
DIR       = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "dashboard"))
SSDP_ADDR = "239.255.255.250"
SSDP_PORT = 1900
SSDP_ST   = "urn:schemas-pnut:device:SaiMonitor:1"
SSDP_MX   = 3       # segundos esperando respuestas a M-SEARCH
DEVICE_TTL = 3600   # segundos antes de eliminar un dispositivo sin actualizar

_lock         = threading.Lock()
_msearch_lock = threading.Lock()   # evita 2 M-SEARCH simultáneos
_discovered   = {}   # usn -> {"name": str, "url": str, "seen_at": float}


def _parse_ssdp_headers(data: bytes) -> dict:
    headers = {}
    try:
        lines = data.decode("utf-8", errors="replace").splitlines()
    except Exception:
        return headers
    for line in lines[1:]:
        if ":" in line:
            k, _, v = line.partition(":")
            headers[k.strip().upper()] = v.strip()
    return headers


def _ssdp_listener():
    """Daemon: escucha NOTIFYs SSDP multicast y actualiza _discovered."""
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        if hasattr(socket, "SO_REUSEPORT"):
            try:
                sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEPORT, 1)
            except OSError:
                pass
        sock.bind(("", SSDP_PORT))
        mreq = struct.pack("4sL", socket.inet_aton(SSDP_ADDR), socket.INADDR_ANY)
        sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
        sock.settimeout(2.0)
    except OSError as e:
        print(f"[ssdp] No se pudo escuchar en UDP {SSDP_PORT}: {e}")
        return

    while True:
        try:
            data, _ = sock.recvfrom(4096)
        except socket.timeout:
            cutoff = time.time() - DEVICE_TTL
            with _lock:
                stale = [k for k, v in _discovered.items() if v["seen_at"] < cutoff]
                for k in stale:
                    del _discovered[k]
            continue
        except Exception:
            continue

        h = _parse_ssdp_headers(data)
        nt   = h.get("NT", "")
        nts  = h.get("NTS", "")
        usn  = h.get("USN", "")
        loc  = h.get("LOCATION", "")
        name = h.get("X-PNUT-NAME", "")

        if nt == SSDP_ST and nts == "ssdp:alive" and usn and loc:
            with _lock:
                _discovered[usn] = {"name": name or loc, "url": loc, "seen_at": time.time()}
        elif nts == "ssdp:byebye" and usn:
            with _lock:
                _discovered.pop(usn, None)


def _send_msearch():
    """Envía M-SEARCH activo y espera SSDP_MX segundos respuestas.
    Solo un M-SEARCH simultáneo (lock no bloqueante: si ya hay otro, retorna)."""
    if not _msearch_lock.acquire(blocking=False):
        return
    msg = (
        "M-SEARCH * HTTP/1.1\r\n"
        f"HOST: {SSDP_ADDR}:{SSDP_PORT}\r\n"
        'MAN: "ssdp:discover"\r\n'
        f"MX: {SSDP_MX}\r\n"
        f"ST: {SSDP_ST}\r\n"
        "\r\n"
    ).encode()
    sock = None
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
        sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 4)
        sock.settimeout(0.5)
        sock.sendto(msg, (SSDP_ADDR, SSDP_PORT))
        deadline = time.time() + SSDP_MX
        while time.time() < deadline:
            try:
                data, _ = sock.recvfrom(4096)
            except socket.timeout:
                continue
            except Exception:
                break
            h    = _parse_ssdp_headers(data)
            st   = h.get("ST", "")
            usn  = h.get("USN", "")
            loc  = h.get("LOCATION", "")
            name = h.get("X-PNUT-NAME", "")
            if st == SSDP_ST and usn and loc:
                with _lock:
                    _discovered[usn] = {"name": name or loc, "url": loc, "seen_at": time.time()}
    except Exception as e:
        print(f"[ssdp] M-SEARCH error: {e}")
    finally:
        if sock is not None:
            try:
                sock.close()
            except Exception:
                pass
        _msearch_lock.release()


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=DIR, **kwargs)

    def log_message(self, fmt, *args):
        pass

    def do_GET(self):
        if self.path.split("?")[0] == "/api/discover":
            self._handle_discover()
        else:
            super().do_GET()

    def do_OPTIONS(self):
        self.send_response(204)
        self._cors()
        self.end_headers()

    def _cors(self):
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type")

    def _handle_discover(self):
        _send_msearch()
        with _lock:
            snapshot = dict(_discovered)
        devices = [
            {"name": v["name"], "url": v["url"], "usn": k}
            for k, v in snapshot.items()
        ]
        body = json.dumps(devices, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self._cors()
        self.end_headers()
        self.wfile.write(body)


def main():
    t = threading.Thread(target=_ssdp_listener, daemon=True, name="ssdp-listener")
    t.start()

    try:
        with http.server.HTTPServer(("", PORT), Handler) as httpd:
            url = f"http://localhost:{PORT}"
            print(f"Dashboard → {url}")
            print("Ctrl+C para parar\n")
            webbrowser.open(url)
            httpd.serve_forever()
    except OSError as e:
        if e.errno == 48:
            print(f"Puerto {PORT} ocupado. Libéralo con:\n  kill $(lsof -ti:{PORT})")
        else:
            raise
    except KeyboardInterrupt:
        print("\nServidor parado.")


if __name__ == "__main__":
    main()
