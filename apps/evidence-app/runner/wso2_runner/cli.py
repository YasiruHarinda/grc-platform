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
        # No API key prompt here, deliberately — the Runner authorises Azure
        # OpenAI calls with the engineer's own Entra identity (az login),
        # not a shared secret written to disk. See azure_credential.py.
        lines.append("AZURE_OPENAI_ENDPOINT=" + typer.prompt("Azure OpenAI endpoint (https://...)"))
        lines.append("AZURE_OPENAI_DEPLOYMENT=" + typer.prompt("Deployment name", default=model))
        lines.append("AZURE_OPENAI_API_VERSION=2024-10-21")
        lines.append("AZURE_TENANT_ID=" + typer.prompt("Azure tenant ID"))
        print("\n  Next: run `az login` to sign in with your own Azure identity.")
        print("  The Runner uses that session to call Azure OpenAI — no key is stored.")
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

    from wso2_runner.config import AZURE_AUTH_API_KEY, settings, CONFIG_FILE
    if not settings.AGENT_PROVIDER:
        typer.echo(
            "\n[runner] No LLM config found. Run this first:\n\n"
            "    wso2-runner configure\n",
            err=True,
        )
        raise typer.Exit(1)

    # ASGARDEO_ORG has no default that would work (it names a specific
    # Asgardeo tenant), and `configure` above doesn't ask for it — it's set
    # by hand, per the setup docs. Caught here, in plain language, rather
    # than left to blow up deep inside run_forever (loop.py reads it to sign
    # in and to build the cloud client).
    if not settings.ASGARDEO_ORG:
        typer.echo(
            f"\n[runner] ASGARDEO_ORG is not set. Add it to {CONFIG_FILE} — "
            "see the setup docs.\n",
            err=True,
        )
        raise typer.Exit(1)

    # Prove Azure auth works *before* the poll loop starts — not on first
    # use. The credential otherwise authenticates lazily on first LLM call,
    # which happens after a browser window has already opened and a task
    # has already been consumed. api_key mode needs no such check; it
    # behaves exactly as it always has.
    if settings.AGENT_PROVIDER == "azure" and settings.AZURE_OPENAI_AUTH_MODE != AZURE_AUTH_API_KEY:
        from wso2_runner.azure_credential import (
            AzureAccessDeniedError,
            AzureAccessUnverifiedError,
            ClientAuthenticationError,
            CredentialUnavailableError,
            verify_access,
        )

        try:
            asyncio.run(verify_access())
        # CredentialUnavailableError is a subclass of ClientAuthenticationError —
        # it must be checked first or it's silently swallowed by the wider case.
        except CredentialUnavailableError:
            typer.echo(
                "\n[runner] Azure sign-in not found — the Azure CLI isn't "
                "installed, or nobody is signed in. Run this first:\n\n"
                "    az login\n",
                err=True,
            )
            raise typer.Exit(1)
        except ClientAuthenticationError:
            typer.echo(
                "\n[runner] Azure sign-in was rejected — your session may "
                "have expired, or you may be signed in to the wrong "
                "tenant. Run this first:\n\n"
                "    az login\n",
                err=True,
            )
            raise typer.Exit(1)
        except AzureAccessDeniedError:
            # Signed in correctly and still refused. Nothing the engineer can
            # do at a terminal fixes this, so `az login` is the wrong advice
            # here -- it is the one failure that needs another person.
            typer.echo(
                "\n[runner] You are signed in, but your account is not "
                "allowed to call Azure OpenAI. Ask an administrator to grant "
                "you the Azure OpenAI role on the resource, then try "
                "again.\n",
                err=True,
            )
            raise typer.Exit(1)
        except AzureAccessUnverifiedError as exc:
            # Warn and carry on. See verify_access(): an inconclusive probe
            # must never stop a runner that would have worked.
            typer.echo(
                f"\n[runner] Could not confirm Azure OpenAI access ({exc}). "
                "Starting anyway -- if calls fail, run `wso2-runner doctor`.\n",
                err=True,
            )
        except Exception as exc:
            typer.echo(f"\n[runner] Azure authentication check failed: {exc}\n", err=True)
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
    from wso2_runner.config import AZURE_AUTH_API_KEY, settings

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
    if not settings.ASGARDEO_ORG:
        print("    ✗ ASGARDEO_ORG is not set — see the setup docs")
    elif not settings.ASGARDEO_CLIENT_ID:
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
    elif settings.AGENT_PROVIDER == "azure" and settings.AZURE_OPENAI_AUTH_MODE == AZURE_AUTH_API_KEY and not settings.AZURE_OPENAI_API_KEY:
        print("    ✗ AZURE_OPENAI_API_KEY is not set")
    elif settings.AGENT_PROVIDER == "azure" and settings.AZURE_OPENAI_AUTH_MODE != AZURE_AUTH_API_KEY:
        # entra mode: a non-empty AZURE_OPENAI_API_KEY proves nothing — it
        # could be revoked, and it isn't even used in this mode. Attempt a
        # real token instead, the same way the start-up gate (ticket #94)
        # does, but without forcing an interactive login (see [2] above —
        # doctor only ever reads a session that already exists).
        import shutil

        from wso2_runner.azure_credential import (
            AzureAccessDeniedError,
            AzureAccessUnverifiedError,
            ClientAuthenticationError,
            CredentialUnavailableError,
            verify_access,
        )

        # CredentialUnavailableError is a subclass of ClientAuthenticationError —
        # it must be checked first or it's silently swallowed by the wider case.
        try:
            asyncio.run(verify_access())
            print("    ✓ Azure OpenAI access works — signed in and authorised")
        except CredentialUnavailableError:
            # The credential could not even attempt authentication. The Azure
            # CLI's presence on PATH is the one extra signal we genuinely
            # have, so use it to tell "not installed" apart from "installed,
            # nobody signed in" — the library's own exception doesn't
            # distinguish these two, so we don't invent a way to.
            if shutil.which("az") is None:
                print("    ✗ Azure CLI is not installed — install it, then run: az login")
            else:
                print("    ✗ Azure CLI is installed but nobody is signed in — run: az login")
        except ClientAuthenticationError:
            # Authentication was attempted and rejected. Because the
            # credential is pinned to AZURE_TENANT_ID, this is the wrong
            # tenant: the CLI holds a session, but not one that can produce a
            # token for the tenant configured here.
            print(
                "    ✗ Azure sign-in was rejected — you appear to be signed in "
                "to the wrong tenant. Run `az account show` and compare its "
                "tenantId to AZURE_TENANT_ID in your config; if they differ, "
                "run `az login --tenant <AZURE_TENANT_ID>`."
            )
        except AzureAccessDeniedError:
            # The state a token check alone can never see: authentication
            # succeeded and the resource still refused the call. Azure AD
            # issues a Cognitive Services token to any member of the tenant;
            # the role is enforced by the resource, so only a real call
            # reaches this.
            print(
                "    ✗ Azure sign-in works, but your account is not allowed to "
                "call Azure OpenAI — you are missing the Azure OpenAI role on "
                "the resource. `az login` will not fix this: ask an "
                "administrator to grant you the role."
            )
        except AzureAccessUnverifiedError as exc:
            print(f"    – Azure sign-in works, but access could not be confirmed: {exc}")
            print("       Set AZURE_OPENAI_ENDPOINT, or check network access to it.")
        except Exception as exc:
            print(f"    ✗ Azure authentication check failed: {exc}")
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
