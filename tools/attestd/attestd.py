#!/usr/bin/env python3
"""Serve ChatGPT DeviceCheck attestations for a Sub2API instance running elsewhere.

Sub2API can only mint a Live attestation on Apple Silicon macOS, because the token
comes from the native module bundled inside the official ChatGPT app. Run this on
such a Mac and point Sub2API at it with SUB2API_ATTESTATION_REMOTE_URL.

Endpoints (both require `Authorization: Bearer $SUB2API_ATTESTD_TOKEN`):
    GET /healthz      -> 200 when this Mac can mint attestations
    GET /attestation  -> the attestation JSON, verbatim

The generation logic mirrors backend/internal/platform/liveattestation/attestation_darwin.go.
"""

import http.server
import json
import os
import re
import subprocess
import sys
import threading
import uuid

APP_PATHS = ["/Applications/ChatGPT.app",
             os.path.expanduser("~/Applications/ChatGPT.app")]
TIMEOUT = 5
MAX_BYTES = 16 * 1024

SIGNALS_JS = '''ObjC.import("Foundation"); ObjC.import("AppKit");
const screen = $.NSScreen.mainScreen;
const frame = screen.frame;
JSON.stringify({
  locale: ObjC.unwrap($.NSLocale.currentLocale.localeIdentifier),
  languages: ObjC.deepUnwrap($.NSLocale.preferredLanguages),
  timezone: ObjC.unwrap($.NSTimeZone.localTimeZone.name),
  width: Number(frame.size.width),
  height: Number(frame.size.height),
  scale: Number(screen.backingScaleFactor)
})'''

# Matches the darwin provider: one session id for the lifetime of the process.
APP_SESSION_ID = str(uuid.uuid4())
LOCK = threading.Lock()


class AttestationError(RuntimeError):
    pass


def _truncate(value, limit, fallback):
    value = (value or "").strip() or fallback
    return value[:limit]


def _find_app():
    for path in APP_PATHS:
        if os.path.isdir(path):
            return path
    raise AttestationError("the official ChatGPT app is not installed on this Mac")


def resolve_runtime():
    if os.uname().machine != "arm64":
        raise AttestationError("live attestation requires Apple Silicon")
    app = _find_app()
    resources = os.path.join(app, "Contents", "Resources")
    node = os.path.join(resources, "cua_node", "bin", "node")
    module = os.path.join(resources, "native", "devicecheck.node")
    for path, label in ((node, "bundled Node.js runtime"),
                        (module, "DeviceCheck native module")):
        if not os.path.isfile(path):
            raise AttestationError(f"ChatGPT app is missing its {label}")
    bundle = subprocess.run(
        ["/usr/bin/plutil", "-extract", "CFBundleIdentifier", "raw",
         os.path.join(app, "Contents", "Info.plist")],
        capture_output=True, text=True, timeout=TIMEOUT).stdout.strip()
    if not bundle.startswith("com.openai."):
        raise AttestationError("the installed ChatGPT app has an unexpected bundle identifier")
    return node, module, bundle


def read_signals():
    result = subprocess.run(["/usr/bin/osascript", "-l", "JavaScript", "-e", SIGNALS_JS],
                            capture_output=True, text=True, timeout=TIMEOUT)
    if result.returncode != 0:
        raise AttestationError(f"read macOS signals: {result.stderr.strip()[:200]}")
    values = json.loads(result.stdout)
    locale = _truncate(values.get("locale"), 64, "unknown")
    languages = values.get("languages") or [locale]
    languages = [_truncate(item, 64, locale) for item in languages[:16]]
    scale = values.get("scale") or 0
    if scale <= 0:
        scale = 1
    return {
        "schemaVersion": 1,
        "preferredLanguages": languages,
        "locale": locale,
        "timezone": _truncate(values.get("timezone"), 64, "unknown"),
        "screenSizeSum": max(0, int(values.get("width", 0) + values.get("height", 0) + 0.5)),
        "screenScale": scale,
        "appSessionId": _truncate(APP_SESSION_ID, 128, str(uuid.uuid4())),
    }


def generate(script_path):
    node, module, bundle = resolve_runtime()
    signals = read_signals()
    with open(script_path, encoding="utf-8") as handle:
        script = handle.read()
    result = subprocess.run(
        [node, "-e", script],
        env={"PATH": "/usr/bin:/bin",
             "SUB2API_DEVICECHECK_MODULE": module,
             "SUB2API_ATTESTATION_BUNDLE_ID": bundle,
             "SUB2API_ATTESTATION_SIGNALS": json.dumps(signals)},
        capture_output=True, text=True, timeout=TIMEOUT * 4)
    if result.returncode != 0:
        raise AttestationError(f"DeviceCheck token generation failed: {result.stderr.strip()[:240]}")
    header = result.stdout.strip()
    if len(header) < 20 or len(header) > MAX_BYTES:
        raise AttestationError("DeviceCheck returned a malformed attestation")
    json.loads(header)
    return header


class Handler(http.server.BaseHTTPRequestHandler):
    server_version = "sub2api-attestd/1.0"
    script_path = ""
    token = ""

    def _reply(self, status, body, content_type="text/plain; charset=utf-8"):
        payload = body.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def _authorized(self):
        header = self.headers.get("Authorization", "")
        return header == f"Bearer {self.token}"

    def do_GET(self):
        path = re.sub(r"\?.*$", "", self.path)
        if not self._authorized():
            self._reply(401, "unauthorized")
            return
        try:
            if path == "/healthz":
                resolve_runtime()
                self._reply(200, "ok")
            elif path == "/attestation":
                with LOCK:
                    header = generate(self.script_path)
                self._reply(200, header, "application/json; charset=utf-8")
            else:
                self._reply(404, "not found")
        except AttestationError as error:
            self._reply(503, str(error))
        except Exception as error:  # noqa: BLE001 - surface the reason to Sub2API
            self._reply(500, f"{type(error).__name__}: {error}")

    def log_message(self, fmt, *args):
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))


def main():
    token = os.environ.get("SUB2API_ATTESTD_TOKEN", "").strip()
    if not token:
        sys.exit("SUB2API_ATTESTD_TOKEN is required")
    script = os.environ.get("SUB2API_ATTESTD_SCRIPT", "").strip()
    if not script or not os.path.isfile(script):
        sys.exit("SUB2API_ATTESTD_SCRIPT must point to devicecheck.js")
    addr = os.environ.get("SUB2API_ATTESTD_ADDR", "0.0.0.0:8799")
    host, _, port = addr.rpartition(":")
    Handler.script_path = script
    Handler.token = token
    server = http.server.ThreadingHTTPServer((host or "0.0.0.0", int(port)), Handler)
    sys.stderr.write(f"attestd listening on {host or '0.0.0.0'}:{port}\n")
    server.serve_forever()


if __name__ == "__main__":
    main()
