#!/usr/bin/env python3
"""Generate user-facing terminal proof for bv push auth refresh.

This is intentionally different from run_actual_app_proof.py: it hides the
operator/server trace and captures only what a user would type and see in a
terminal while using the real built bv binary.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import threading
from dataclasses import dataclass
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

from run_actual_app_proof import PROOF_DIR, REPO_ROOT, Scenario, run_server, write_json, rfc3339
from datetime import timedelta

OUTPUT_DIR = PROOF_DIR / "user-terminal-run"


@dataclass
class TerminalScenario:
    slug: str
    title: str
    scenario: Scenario
    command_args: list[str]
    expected_exit: int
    setup_note: str


def run(args: list[str], env: dict[str, str], cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        args,
        cwd=str(cwd),
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=60,
    )


def wrap_lines(lines: list[str], max_chars: int) -> list[str]:
    out: list[str] = []
    for line in lines:
        if len(line) <= max_chars:
            out.append(line)
            continue
        rest = line
        prefix = ""
        while len(rest) > max_chars:
            cut = rest.rfind(" ", 0, max_chars)
            if cut < max_chars // 2:
                cut = max_chars
            out.append(prefix + rest[:cut].rstrip())
            rest = rest[cut:].lstrip()
            prefix = "    "
        out.append(prefix + rest)
    return out


def render_terminal(text: str, title: str, path: Path) -> None:
    lines = wrap_lines(text.expandtabs(4).splitlines() or [""], 100)
    try:
        font = ImageFont.truetype("/System/Library/Fonts/Menlo.ttc", 18)
        font_bold = ImageFont.truetype("/System/Library/Fonts/Menlo.ttc", 21)
    except Exception:
        font = ImageFont.load_default()
        font_bold = font

    line_height = 27
    padding_x = 32
    padding_y = 28
    title_height = 46
    width = 1240
    height = padding_y * 2 + title_height + line_height * len(lines)
    image = Image.new("RGB", (width, height), "#0b1020")
    draw = ImageDraw.Draw(image)
    draw.rounded_rectangle((12, 12, width - 12, height - 12), radius=18, fill="#111827", outline="#334155", width=2)
    draw.text((padding_x, padding_y), title, fill="#e5e7eb", font=font_bold)

    y = padding_y + title_height
    for line in lines:
        fill = "#e5e7eb"
        if line.startswith("$"):
            fill = "#a7f3d0"
        elif line.startswith("Refreshed"):
            fill = "#fde68a"
        elif line.startswith("URL:") or line.startswith("Site:") or line.startswith("Status:"):
            fill = "#86efac"
        elif "UNAUTHENTICATED" in line or line.startswith("bv: error"):
            fill = "#fca5a5"
        draw.text((padding_x, y), line, fill=fill, font=font)
        y += line_height
    image.save(path)


def seed_config(path: Path, base_url: str, token: str, expired: bool) -> None:
    write_json(path, {
        "api_url": base_url,
        "installation_token": token,
        "tenant_id": "t_alice",
        "account_login": "alice",
        "installation_id": 42,
        "token_expires_at": rfc3339(timedelta(hours=-1 if expired else 1)),
    })


def run_terminal_scenario(binary_dir: Path, workspace: Path, item: TerminalScenario) -> str:
    scenario = item.scenario
    server, thread, base_url = run_server(scenario)
    scenario_dir = workspace / item.slug
    site_dir = scenario_dir / "demo-site"
    config_path = scenario_dir / "config.json"
    site_dir.mkdir(parents=True, exist_ok=True)
    (site_dir / "index.html").write_text(f"<h1>{item.title}</h1>\n")

    env = os.environ.copy()
    env["PATH"] = str(binary_dir) + os.pathsep + env.get("PATH", "")
    env["GH_TOKEN"] = "ghu_refresh" if not scenario.token_override else "ghu_should_not_be_used"
    if scenario.token_override:
        seed_config(config_path, base_url, "ghs_saved_but_overridden", False)
        env["BV_CONFIG_PATH"] = str(config_path)
    else:
        seed_config(config_path, base_url, scenario.old_token, scenario.expired_config)
        env["BV_CONFIG_PATH"] = str(config_path)

    command_args = [base_url if arg == "http://127.0.0.1:LOCAL" else arg for arg in item.command_args]

    try:
        result = run(command_args, env, scenario_dir)
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)

    if result.returncode != item.expected_exit:
        raise AssertionError(f"{item.slug}: exit {result.returncode}, want {item.expected_exit}\n{result.stdout}")
    if scenario.expect_success and "URL:" not in result.stdout:
        raise AssertionError(f"{item.slug}: success output did not include URL\n{result.stdout}")
    if not scenario.expect_success and "UNAUTHENTICATED" not in result.stdout:
        raise AssertionError(f"{item.slug}: error output did not include UNAUTHENTICATED\n{result.stdout}")

    # Keep a machine-checkable trace next to the user transcript, but do not
    # include it in the screenshot; the screenshot is strictly the user view.
    trace = "\n".join(f"{idx}. {event}" for idx, event in enumerate(scenario.events, start=1))
    (OUTPUT_DIR / f"{item.slug}-server-trace.txt").write_text(trace + "\n")

    command_line = "$ " + " ".join(command_args)
    transcript = f"""{command_line}
{result.stdout.rstrip()}
"""
    (OUTPUT_DIR / f"{item.slug}.txt").write_text(transcript)
    render_terminal(transcript, item.title, OUTPUT_DIR / f"{item.slug}.png")
    return transcript


def main() -> int:
    if OUTPUT_DIR.exists():
        shutil.rmtree(OUTPUT_DIR)
    OUTPUT_DIR.mkdir(parents=True)

    with tempfile.TemporaryDirectory(prefix="bv-user-terminal-proof-") as raw_tmp:
        workspace = Path(raw_tmp)
        binary_dir = workspace / "bin"
        binary_dir.mkdir()
        binary = binary_dir / "bv"
        build = run(["go", "build", "-o", str(binary), "./cmd/bv"], os.environ.copy(), REPO_ROOT)
        if build.returncode != 0:
            raise SystemExit(build.stdout)

        scenarios = [
            TerminalScenario(
                slug="01-expired-token-user-push",
                title="User runs bv push after saved token expired",
                scenario=Scenario(
                    slug="kind-otter-7q",
                    title="User runs bv push after saved token expired",
                    old_token="ghs_old_user_expired",
                    new_token="ghs_new_user_expired",
                    expired_config=True,
                ),
                command_args=["bv", "push", "demo-site"],
                expected_exit=0,
                setup_note="user previously ran bv login; saved install token is now expired; gh auth is still available",
            ),
            TerminalScenario(
                slug="02-create-401-user-push",
                title="User runs bv push when create gets an auth 401",
                scenario=Scenario(
                    slug="silver-fox-4m",
                    title="User runs bv push when create gets an auth 401",
                    old_token="ghs_old_user_create",
                    new_token="ghs_new_user_create",
                    expired_config=False,
                    create_401_first=True,
                ),
                command_args=["bv", "push", "demo-site"],
                expected_exit=0,
                setup_note="saved token appears valid locally, but API rejects create once with 401",
            ),
            TerminalScenario(
                slug="03-finalize-401-user-push",
                title="User runs bv push when finalize gets an auth 401",
                scenario=Scenario(
                    slug="bright-lynx-2k",
                    title="User runs bv push when finalize gets an auth 401",
                    old_token="ghs_old_user_finalize",
                    new_token="ghs_new_user_finalize",
                    expired_config=False,
                    finalize_401_first=True,
                ),
                command_args=["bv", "push", "demo-site"],
                expected_exit=0,
                setup_note="saved token appears valid locally, but API rejects finalize once with 401",
            ),
            TerminalScenario(
                slug="04-token-override-user-push",
                title="User runs bv push with explicit --token override",
                scenario=Scenario(
                    slug="quiet-panda-9x",
                    title="User runs bv push with explicit --token override",
                    old_token="ghs_example_expired",
                    new_token="ghs_new_should_not_be_used",
                    expired_config=False,
                    token_override=True,
                    expect_success=False,
                ),
                command_args=["bv", "--token", "ghs_example_expired", "push", "demo-site"],
                expected_exit=4,
                setup_note="explicit --token is user-supplied and intentionally not auto-refreshed",
            ),
        ]

        full = []
        for item in scenarios:
            full.append(run_terminal_scenario(binary_dir, workspace, item))

        combined = "\n" + ("=" * 90) + "\n\n"
        combined = combined.join(full).strip() + "\n"
        (OUTPUT_DIR / "full-user-terminal-transcript.txt").write_text(combined)
        render_terminal(combined, "What a user sees in the terminal", OUTPUT_DIR / "full-user-terminal-session.png")

    print(f"Wrote user-facing terminal proof to {OUTPUT_DIR}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
