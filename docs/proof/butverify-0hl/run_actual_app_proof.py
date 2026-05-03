#!/usr/bin/env python3
"""Run actual bv CLI auth-refresh proof scenarios.

This script builds the real bv binary, starts a live local HTTP server that
implements the control-plane and staging endpoints used by `bv push`, then
runs the binary against that server. It writes raw transcripts and screenshot
PNGs under this proof directory.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
import textwrap
import threading
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

from PIL import Image, ImageDraw, ImageFont

PROOF_DIR = Path(__file__).resolve().parent
REPO_ROOT = PROOF_DIR.parents[2]
MANIFEST_SHA = "4f8c2d9e1a7b6c5d3e2f1098ab76cd54ef3210987a6b5c4d3e2f1098ab76cd54"


@dataclass
class Scenario:
    slug: str
    title: str
    old_token: str
    new_token: str
    expired_config: bool
    create_401_first: bool = False
    finalize_401_first: bool = False
    token_override: bool = False
    expect_success: bool = True
    create_attempts: int = 0
    finalize_attempts: int = 0
    login_calls: int = 0
    upload_bytes: int = 0
    events: list[str] = field(default_factory=list)

    @property
    def site_id(self) -> str:
        return "site_" + self.slug.replace("-", "_")


def rfc3339(delta: timedelta) -> str:
    return (datetime.now(timezone.utc) + delta).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2) + "\n")


def make_handler(scenario: Scenario):
    class Handler(BaseHTTPRequestHandler):
        server_version = "bv-proof/1.0"

        def log_message(self, fmt: str, *args: Any) -> None:
            return

        def _json(self, status: int, payload: dict[str, Any]) -> None:
            body = json.dumps(payload).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _read_body(self) -> bytes:
            length = int(self.headers.get("Content-Length", "0") or "0")
            if length == 0:
                return b""
            return self.rfile.read(length)

        def do_POST(self) -> None:
            parsed = urlparse(self.path)
            auth = self.headers.get("Authorization", "")
            body = self._read_body()

            if parsed.path == "/v1/auth/login":
                scenario.login_calls += 1
                scenario.events.append(f"POST /v1/auth/login auth={auth}")
                self._json(200, {
                    "token": scenario.new_token,
                    "expires_at": rfc3339(timedelta(hours=1)),
                    "tenant_id": "t_alice",
                    "account_login": "alice",
                    "installation_id": 42,
                    "account_type": "User",
                })
                return

            if parsed.path == "/v1/sites":
                scenario.create_attempts += 1
                try:
                    upload_id = json.loads(body.decode()).get("upload_id", "")
                except Exception:
                    upload_id = "<unparseable>"
                scenario.events.append(f"POST /v1/sites attempt={scenario.create_attempts} auth={auth} upload_id={upload_id}")
                if scenario.create_401_first and scenario.create_attempts == 1:
                    self._json(401, {"error": {"code": "UNAUTHENTICATED", "message": "expired create token"}})
                    return
                if scenario.token_override:
                    self._json(401, {"error": {"code": "UNAUTHENTICATED", "message": "token expired"}})
                    return
                self._json(200, {
                    "site_id": scenario.site_id,
                    "url": f"https://{scenario.site_id}.butverify.dev",
                    "expires_at": "2026-05-04T10:00:00Z",
                    "upload_token": "use_installation_token",
                    "manifest_url": f"https://api.example.test/v1/sites/{scenario.site_id}/manifest",
                    "status": "creating",
                    "idempotent": False,
                    "upload_url": f"http://{self.server.server_address[0]}:{self.server.server_address[1]}/staging/{scenario.slug}",
                    "upload_max_bytes": 100 * 1024 * 1024,
                    "upload_url_expires_at": "2026-04-27T10:15:00Z",
                })
                return

            if parsed.path == f"/v1/sites/{scenario.site_id}/finalize":
                scenario.finalize_attempts += 1
                scenario.events.append(f"POST /v1/sites/{scenario.site_id}/finalize attempt={scenario.finalize_attempts} auth={auth}")
                if scenario.finalize_401_first and scenario.finalize_attempts == 1:
                    self._json(401, {"error": {"code": "UNAUTHENTICATED", "message": "expired finalize token"}})
                    return
                self._json(200, {
                    "site_id": scenario.site_id,
                    "status": "active",
                    "url": f"https://{scenario.site_id}.butverify.dev",
                    "manifest_url": "x",
                    "expires_at": "2026-05-04T10:00:00Z",
                    "manifest_sha": MANIFEST_SHA,
                    "last_pushed_at": "2026-04-27T10:00:00Z",
                    "idempotent": False,
                })
                return

            scenario.events.append(f"POST {parsed.path} auth={auth} unexpected")
            self._json(404, {"error": {"code": "NOT_FOUND", "message": "no route"}})

        def do_PUT(self) -> None:
            parsed = urlparse(self.path)
            body = self._read_body()
            scenario.upload_bytes += len(body)
            scenario.events.append(f"PUT {parsed.path} bytes={len(body)} content_type={self.headers.get('Content-Type', '')}")
            self.send_response(200)
            self.end_headers()

    return Handler


def run_server(scenario: Scenario) -> tuple[ThreadingHTTPServer, threading.Thread, str]:
    server = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(scenario))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base_url = f"http://{server.server_address[0]}:{server.server_address[1]}"
    return server, thread, base_url


def run_command(args: list[str], env: dict[str, str], cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=str(cwd), env=env, text=True, capture_output=True, timeout=60)


def scenario_command_for_display(env_vars: dict[str, str], args: list[str]) -> str:
    parts = [f"{key}={value}" for key, value in env_vars.items()]
    parts.extend(args)
    return "$ " + " ".join(parts)


def assert_scenario(scenario: Scenario, result: subprocess.CompletedProcess[str], config_path: Path) -> str:
    if scenario.expect_success:
        if result.returncode != 0:
            raise AssertionError(f"{scenario.slug}: expected exit 0, got {result.returncode}")
        payload = json.loads(result.stdout)
        if payload["site_id"] != scenario.site_id or payload["status"] != "active":
            raise AssertionError(f"{scenario.slug}: unexpected CLI payload {payload}")
        if scenario.upload_bytes <= 0:
            raise AssertionError(f"{scenario.slug}: expected tar upload bytes")
    else:
        if result.returncode != 4:
            raise AssertionError(f"{scenario.slug}: expected auth exit 4, got {result.returncode}")

    if scenario.expired_config:
        if scenario.login_calls != 1:
            raise AssertionError(f"{scenario.slug}: expected one proactive login, got {scenario.login_calls}")
        if scenario.create_attempts != 1:
            raise AssertionError(f"{scenario.slug}: expected one create attempt, got {scenario.create_attempts}")
    if scenario.create_401_first:
        if scenario.login_calls != 1 or scenario.create_attempts != 2:
            raise AssertionError(f"{scenario.slug}: expected login once and create twice")
    if scenario.finalize_401_first:
        if scenario.login_calls != 1 or scenario.finalize_attempts != 2:
            raise AssertionError(f"{scenario.slug}: expected login once and finalize twice")
    if scenario.token_override:
        if scenario.login_calls != 0:
            raise AssertionError(f"{scenario.slug}: token override must not call login")

    if not scenario.token_override:
        saved = json.loads(config_path.read_text())
        if saved.get("installation_token") != scenario.new_token:
            raise AssertionError(f"{scenario.slug}: config did not persist refreshed token")
        return f"PASS - config now stores refreshed token {scenario.new_token}"
    return "PASS - explicit --token override returned 401 without calling /v1/auth/login"


def run_scenario(binary: Path, workspace: Path, scenario: Scenario) -> str:
    server, thread, base_url = run_server(scenario)
    scenario_dir = workspace / scenario.slug
    site_dir = scenario_dir / "site"
    config_path = scenario_dir / "config.json"
    site_dir.mkdir(parents=True, exist_ok=True)
    (site_dir / "index.html").write_text(f"<h1>{scenario.title}</h1>\n")

    if not scenario.token_override:
        write_json(config_path, {
            "api_url": base_url,
            "installation_token": scenario.old_token,
            "tenant_id": "t_alice",
            "account_login": "alice",
            "installation_id": 42,
            "token_expires_at": rfc3339(timedelta(hours=-1 if scenario.expired_config else 1)),
        })

    env = os.environ.copy()
    display_env: dict[str, str]
    args: list[str]
    if scenario.token_override:
        display_env = {"GH_TOKEN": "ghu_should_not_be_used"}
        env.update(display_env)
        args = [str(binary), "--json", "--api-url", base_url, "--token", scenario.old_token, "push", str(site_dir)]
    else:
        display_env = {"BV_CONFIG_PATH": str(config_path), "GH_TOKEN": "ghu_refresh"}
        env.update(display_env)
        args = [str(binary), "--json", "push", str(site_dir)]

    try:
        result = run_command(args, env, REPO_ROOT)
        verdict = assert_scenario(scenario, result, config_path)
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)

    stdout = result.stdout.strip() or "<empty>"
    stderr = result.stderr.strip() or "<empty>"
    events = "\n".join(f"  {idx}. {event}" for idx, event in enumerate(scenario.events, start=1))
    config_after = "<not used: explicit --token override>"
    if config_path.exists():
        config_after = config_path.read_text().strip()

    return f"""SCENARIO: {scenario.title}

Live local server: {base_url}
Command:
{scenario_command_for_display(display_env, args)}

Exit code: {result.returncode}

CLI stdout:
{stdout}

CLI stderr:
{stderr}

Server observed real HTTP requests:
{events}

Config after run:
{config_after}

Result: {verdict}
"""


def wrap_lines(lines: list[str], max_chars: int) -> list[str]:
    wrapped: list[str] = []
    for line in lines:
        if len(line) <= max_chars:
            wrapped.append(line)
            continue
        prefix = ""
        rest = line
        while len(rest) > max_chars:
            cut = rest.rfind(" ", 0, max_chars)
            if cut < max_chars // 2:
                cut = max_chars
            wrapped.append(prefix + rest[:cut].rstrip())
            rest = rest[cut:].lstrip()
            prefix = "    "
        wrapped.append(prefix + rest)
    return wrapped


def render_png(text: str, title: str, path: Path) -> None:
    text = text.expandtabs(4)
    lines = wrap_lines(text.splitlines() or [""], 115)
    try:
        font = ImageFont.truetype("/System/Library/Fonts/Menlo.ttc", 17)
        font_bold = ImageFont.truetype("/System/Library/Fonts/Menlo.ttc", 21)
    except Exception:
        font = ImageFont.load_default()
        font_bold = font
    line_height = 25
    padding_x = 30
    padding_y = 26
    title_height = 44
    width = 1320
    height = padding_y * 2 + title_height + line_height * len(lines)
    image = Image.new("RGB", (width, height), "#0b1020")
    draw = ImageDraw.Draw(image)
    draw.rounded_rectangle((10, 10, width - 10, height - 10), radius=18, fill="#111827", outline="#334155", width=2)
    draw.text((padding_x, padding_y), title, fill="#e5e7eb", font=font_bold)
    y = padding_y + title_height
    for line in lines:
        fill = "#e5e7eb"
        if line.startswith("$") or line.startswith("SCENARIO:"):
            fill = "#a7f3d0"
        if line.startswith("Result: PASS") or "status\": \"active" in line:
            fill = "#86efac"
        if "UNAUTHENTICATED" in line or "401" in line:
            fill = "#fca5a5"
        draw.text((padding_x, y), line, fill=fill, font=font)
        y += line_height
    image.save(path)


def main() -> int:
    output_dir = PROOF_DIR / "actual-app-run"
    if output_dir.exists():
        shutil.rmtree(output_dir)
    output_dir.mkdir(parents=True)

    with tempfile.TemporaryDirectory(prefix="bv-actual-app-proof-") as tmp_raw:
        workspace = Path(tmp_raw)
        binary = workspace / "bv"
        build_result = run_command(["go", "build", "-o", str(binary), "./cmd/bv"], os.environ.copy(), REPO_ROOT)
        if build_result.returncode != 0:
            raise SystemExit(build_result.stderr or build_result.stdout)

        scenarios = [
            Scenario(
                slug="proactive-expired-token-refresh",
                title="Expired saved token refreshes before bv push creates a site",
                old_token="ghs_old_proactive",
                new_token="ghs_new_proactive",
                expired_config=True,
            ),
            Scenario(
                slug="create-401-refresh-retry",
                title="bv push retries create after 401 by refreshing the token",
                old_token="ghs_old_create_retry",
                new_token="ghs_new_create_retry",
                expired_config=False,
                create_401_first=True,
            ),
            Scenario(
                slug="finalize-401-refresh-retry",
                title="bv push retries finalize after 401 by refreshing the token",
                old_token="ghs_old_finalize_retry",
                new_token="ghs_new_finalize_retry",
                expired_config=False,
                finalize_401_first=True,
            ),
            Scenario(
                slug="token-override-no-refresh",
                title="Explicit --token override does not auto-refresh",
                old_token="ghs_example_expired",
                new_token="ghs_new_should_not_exist",
                expired_config=False,
                token_override=True,
                expect_success=False,
            ),
        ]

        sections = [f"$ go build -o {binary} ./cmd/bv\nBUILD: PASS"]
        for scenario in scenarios:
            section = run_scenario(binary, workspace, scenario)
            sections.append(section)
            (output_dir / f"{scenario.slug}.txt").write_text(section)
            render_png(section, f"Actual bv push run: {scenario.slug}", output_dir / f"{scenario.slug}.png")

        transcript = "\n\n" + ("=" * 100) + "\n\n"
        transcript = transcript.join(sections).strip() + "\n"
        (output_dir / "transcript.txt").write_text(transcript)
        render_png(transcript, "Actual running bv CLI proof: all auth-refresh scenarios", output_dir / "full-transcript.png")

    print(f"Wrote actual app proof artifacts to {output_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
