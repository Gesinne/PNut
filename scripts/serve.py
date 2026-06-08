#!/usr/bin/env python3
"""Dashboard SAI — http://localhost:5500  (Ctrl+C para parar)"""
import http.server
import os
import webbrowser

PORT = 5500
DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "web")


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=DIR, **kwargs)

    def log_message(self, fmt, *args):
        pass  # silencia logs de peticiones


def main():
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
