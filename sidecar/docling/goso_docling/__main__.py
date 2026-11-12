# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

"""Internal HTTP server. Bind is the compose network, not a published host port."""

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from goso_docling.convert import convert_document
from goso_docling.jail import PathDenied, RawMissing, resolve_raw


def _bounded_timeout(value) -> float:
    try:
        number = float(value)
    except (TypeError, ValueError):
        number = _env_float("GOSO_DOCLING_PAGE_TIMEOUT", 60)
    if number <= 0:
        number = 60
    if number > 600:
        number = 600
    return number


def _bounded_attempts(value) -> int:
    try:
        number = int(value)
    except (TypeError, ValueError):
        number = int(_env_float("GOSO_DOCLING_PAGE_ATTEMPTS", 2))
    if number < 1:
        number = 2
    if number > 10:
        number = 10
    return number


def _env_float(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        return float(raw)
    except ValueError:
        return default


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    vault_root = ""

    def log_message(self, fmt, *args):
        return

    def do_GET(self):
        if self.path.split("?", 1)[0] != "/healthz":
            self._send(404, {"error_class": "not_configured"})
            return
        self._send(200, {"ok": True})

    def do_POST(self):
        if self.path.split("?", 1)[0] != "/v1/convert":
            self._send(404, {"error_class": "not_configured"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = -1
        if length < 0 or length > (1 << 20):
            self._send(413, {"error_class": "too_large"})
            return
        raw = self.rfile.read(length)
        try:
            payload = json.loads(raw.decode("utf-8"))
        except (UnicodeError, json.JSONDecodeError):
            self._send(400, {"error_class": "unsupported"})
            return
        if not isinstance(payload, dict):
            self._send(400, {"error_class": "unsupported"})
            return
        try:
            path = resolve_raw(
                self.vault_root,
                str(payload.get("tenant", "")),
                str(payload.get("raw_path", "")),
            )
        except PathDenied:
            self._send(400, {"error_class": "unsupported", "markdown": ""})
            return
        except RawMissing:
            self._send(200, {"error_class": "internal", "markdown": "", "page_count": 0, "ocr_engine": "tesseract", "ocr_lang": "vie+eng"})
            return
        result = convert_document(
            path,
            str(payload.get("mime", "")),
            _bounded_timeout(payload.get("page_timeout_sec")),
            _bounded_attempts(payload.get("max_attempts")),
        )
        self._send(200, result)

    def _send(self, status, payload):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    bind = os.environ.get("GOSO_DOCLING_BIND", "0.0.0.0")
    port = int(os.environ.get("GOSO_DOCLING_PORT", "8071"))
    Handler.vault_root = os.environ.get("GOSO_DOCLING_VAULT_ROOT", "")
    server = ThreadingHTTPServer((bind, port), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
