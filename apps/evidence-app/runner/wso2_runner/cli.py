"""CLI entry point for the WSO2 Compliance Runner."""

import asyncio
import sys

import typer

app = typer.Typer(help="WSO2 Compliance Evidence Runner — polls cloud backend and runs browser automation locally.")


@app.command()
def configure():
    """First-time setup wizard — saves your LLM credentials to ~/.wso2-runner/.env."""
    from wso2_runner.config import CONFIG_DIR, CONFIG_FILE

    CONFIG_DIR.mkdir(parents=True, exist_ok=True)

    print("\nWSO2 Compliance Runner — setup wizard")
    print("=" * 42)
    print("This saves your config to ~/.wso2-runner/.env\n")

    provider = typer.prompt(
        "LLM provider",
        default="azure",
        prompt_suffix=" [azure/anthropic/gemini/ollama]: ",
    )

    model_defaults = {
        "azure": "gpt-4.1-mini",
        "anthropic": "claude-sonnet-4-6",
        "gemini": "gemini-2.0-flash",
        "ollama": "qwen2.5:7b",
    }
    model = typer.prompt("Model name", default=model_defaults.get(provider, ""))

    lines = [f"AGENT_PROVIDER={provider}", f"AGENT_MODEL={model}"]

    if provider == "azure":
        lines.append("AZURE_OPENAI_API_KEY=" + typer.prompt("Azure OpenAI API key", hide_input=True))
        lines.append("AZURE_OPENAI_ENDPOINT=" + typer.prompt("Azure OpenAI endpoint (https://...)"))
        lines.append("AZURE_OPENAI_DEPLOYMENT=" + typer.prompt("Deployment name", default=model))
        lines.append("AZURE_OPENAI_API_VERSION=2024-10-21")
    elif provider == "anthropic":
        lines.append("ANTHROPIC_API_KEY=" + typer.prompt("Anthropic API key", hide_input=True))
    elif provider == "gemini":
        lines.append("GEMINI_API_KEY=" + typer.prompt("Gemini API key", hide_input=True))
    elif provider == "ollama":
        print("  (Ollama uses no API key — make sure it's running on localhost:11434)")

    # Monitor selection for OS-level screenshots
    print("\n── Screenshot monitor ──────────────────────────────────────")
    try:
        import mss as _mss
        with _mss.MSS() as sct:
            mons = sct.monitors[1:]  # skip [0] which is all screens combined
            print("Detected monitors:")
            for i, m in enumerate(mons, 1):
                tag = " ← laptop/primary" if i == 1 else " ← external monitor" if i == 2 else ""
                print(f"  {i}: {m['width']}×{m['height']} at offset ({m['left']},{m['top']}){tag}")
    except Exception:
        print("  (could not list monitors — mss not installed yet, run pip install -e . first)")

    print()
    print("  1 = laptop/primary screen (default)")
    print("  2 = external monitor (plug it in before running the agent)")
    monitor = typer.prompt("Which monitor should the agent use for screenshots?", default=1, type=int)
    lines.append(f"SCREENSHOT_MONITOR={monitor}")

    if monitor > 1:
        print(f"\n  Tip: drag the agent's Chrome window to monitor {monitor} after it opens.")
        print("  Chrome remembers the position — you only need to do this once.")

    CONFIG_FILE.write_text("\n".join(lines) + "\n")
    print(f"\nConfig saved to {CONFIG_FILE}")
    print("Next: wso2-runner start your@wso2.com  (opens a browser to sign in via Asgardeo)\n")


@app.command()
def start(
    email: str = typer.Argument(None, help="Your WSO2 email, e.g. wso2-runner start your@wso2.com — used as a login_hint for the Asgardeo sign-in page"),
    server: str = typer.Option(None, "--server", "-s", help="Cloud backend URL (default: http://localhost:8000)"),
    user: str = typer.Option(None, "--user", "-u", help="Same as the positional email argument"),
    interval: float = typer.Option(None, "--interval", "-i", help="Poll interval in seconds (default: 2.0)"),
):
    """Start the runner. Signs in via Asgardeo, then polls the cloud backend for tasks."""
    # Resolve user from positional arg, --user flag, or config, in that order
    import os
    user = email or user
    if user is None:
        user = os.environ.get("USER_EMAIL") or None

    from wso2_runner.config import settings, CONFIG_FILE
    if not settings.AGENT_PROVIDER:
        typer.echo(
            "\n[runner] No LLM config found. Run this first:\n\n"
            "    wso2-runner configure\n",
            err=True,
        )
        raise typer.Exit(1)

    from wso2_runner.loop import run_forever

    try:
        asyncio.run(run_forever(cloud_url=server, user_email=user, poll_interval=interval))
    except KeyboardInterrupt:
        print("\n[runner] Stopped.")
        sys.exit(0)


@app.command()
def doctor(
    server: str = typer.Option(None, "--server", "-s", help="Cloud backend URL to check"),
    user: str = typer.Option(None, "--user", "-u", help="Your email (login_hint only)"),
):
    """Check connectivity, Chromium install, and LLM config."""
    import httpx

    from wso2_runner import oauth
    from wso2_runner.config import settings

    url = server or settings.CLOUD_URL

    print("WSO2 Compliance Runner — doctor")
    print("=" * 40)

    # Check backend
    print(f"\n[1] Backend connectivity: {url}")
    try:
        r = httpx.get(f"{url}/health", timeout=5)
        print(f"    ✓ {r.json()}")
    except Exception as exc:
        print(f"    ✗ {exc}")

    # Check auth — uses a cached Asgardeo session if one exists; does not
    # force an interactive login just to run a diagnostic check.
    print("\n[2] Asgardeo auth check")
    if not settings.ASGARDEO_CLIENT_ID:
        print("    ✗ ASGARDEO_CLIENT_ID is not set — see the setup docs")
    else:
        if not oauth.has_cached_session():
            print("    – Not signed in yet. Run `wso2-runner start` first to sign in via Asgardeo.")
        else:
            try:
                token = oauth.get_access_token(settings.ASGARDEO_ORG, settings.ASGARDEO_CLIENT_ID)
                r = httpx.get(f"{url}/api/me", headers={"Authorization": f"Bearer {token}"}, timeout=5)
                print(f"    ✓ {r.json()}")
            except Exception as exc:
                print(f"    ✗ {exc}")

    # Check Chromium
    print("\n[3] Chromium / browser-use")
    try:
        from browser_use import BrowserSession
        print("    ✓ browser-use importable")
    except ImportError:
        print("    ✗ browser-use not installed — run: pip install browser-use")

    try:
        from playwright.sync_api import sync_playwright
        with sync_playwright() as p:
            channel = settings.BROWSER_CHANNEL
            b = p.chromium.launch(channel=channel if channel != "chromium" else None, headless=True)
            b.close()
        print(f"    ✓ {channel} launches OK")
    except Exception as exc:
        print(f"    ✗ Browser launch failed: {exc}")
        print("       Try: playwright install chromium")

    # Check LLM
    print(f"\n[4] LLM: provider={settings.AGENT_PROVIDER} model={settings.AGENT_MODEL}")
    if settings.AGENT_PROVIDER == "anthropic" and not settings.ANTHROPIC_API_KEY:
        print("    ✗ ANTHROPIC_API_KEY is not set")
    elif settings.AGENT_PROVIDER == "gemini" and not settings.GEMINI_API_KEY:
        print("    ✗ GEMINI_API_KEY is not set")
    elif settings.AGENT_PROVIDER == "azure" and not settings.AZURE_OPENAI_API_KEY:
        print("    ✗ AZURE_OPENAI_API_KEY is not set")
    elif settings.AGENT_PROVIDER == "ollama":
        try:
            r = httpx.get("http://localhost:11434/api/tags", timeout=5)
            models = [m["name"] for m in r.json().get("models", [])]
            if settings.AGENT_MODEL in models:
                print(f"    ✓ Ollama running, model {settings.AGENT_MODEL} found")
            else:
                print(f"    ✗ Ollama running but model {settings.AGENT_MODEL} not found. Available: {models}")
        except Exception:
            print("    ✗ Ollama not running on localhost:11434")
    else:
        print("    ✓ Key present")

    print()
