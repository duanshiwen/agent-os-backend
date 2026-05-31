#!/usr/bin/env python3
"""Tiny mock SAGE Plugin Server used by local smoke tests.

It intentionally has no AgentOS backend dependencies. The backend remains the
control plane; this server only stands in for a third-party plugin server that
AgentOS Client would call after receiving a policy bundle.
"""
from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parent
MANIFEST = json.loads((ROOT / "sage-plugin.json").read_text())


class Handler(BaseHTTPRequestHandler):
    def _json(self, status: int, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read_json(self) -> dict:
        length = int(self.headers.get("Content-Length", "0") or "0")
        if length == 0:
            return {}
        return json.loads(self.rfile.read(length).decode("utf-8"))

    def do_GET(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
        if self.path in ("/.well-known/sage-plugin.json", "/manifest.json"):
            self._json(200, MANIFEST)
            return
        if self.path == "/sage/health":
            self._json(200, {"status": "ok", "plugin_key": MANIFEST["plugin_key"], "version": MANIFEST["version"]})
            return
        self._json(404, {"error": "not_found"})

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
        payload = self._read_json()
        if self.path == "/sage/flow":
            self._json(200, {
                "flow_id": "flow_hotel_search_demo",
                "flow_version": "1.0.0",
                "request_echo": payload,
                "steps": [
                    {
                        "id": "search_hotels",
                        "type": "plugin_api",
                        "description": "Search mock hotel candidates",
                        "requires_permission": "plugin.api.call"
                    },
                    {
                        "id": "present_options",
                        "type": "present_to_user",
                        "description": "Present three hotel candidates to the user"
                    }
                ],
                "expected_report": {
                    "required_fields": ["client_report_id", "status", "steps_completed"]
                }
            })
            return
        if self.path == "/sage/callback":
            self._json(200, {"status": "accepted", "received": payload})
            return
        self._json(404, {"error": "not_found"})

    def log_message(self, fmt: str, *args) -> None:
        # Keep smoke output deterministic and quiet.
        return


def main() -> None:
    server = HTTPServer(("127.0.0.1", 18080), Handler)
    print("mock SAGE plugin server listening on http://127.0.0.1:18080", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
