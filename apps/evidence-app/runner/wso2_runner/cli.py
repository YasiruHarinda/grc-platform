"""CLI entry point for the WSO2 Compliance Runner."""

import asyncio
import sys

import typer

app = typer.Typer(help="WSO2 Compliance Evidence Runner — polls cloud backend and runs browser automation locally.")


def _fetch_server_config(server: str) -> dict[str, str]:
    """Fetch CLOUD_URL / ASGARDEO_ORG / ASGARDEO_CLIENT_ID for `--server`.

    Talks to GET {server}/api/runner-config (see
    backend/app/api/routes/runner_config.py), which hands back
    {"asgardeo_org": ..., "asgardeo_client_id": ...} with no auth required —
    the Runner needs these before it can log in at all, so there is no
    token yet to present.

    On any failure this prints a plain message naming the URL that was
    tried and raises typer.Exit(1) — it never returns a partial dict. The
    whole reason `--server` exists is to stop a fresh Runner from silently
    inheriting the http://localhost:8000 default in config.py, which looks
    configured but polls nothing. A --server that half-works (e.g. writes
    CLOUD_URL but not the org, because the response was odd) recreates
    exactly that trap in a new shape: `configure` would exit 0, the file
    would exist, and the Runner would still fail to reach anything real.
    Better to write nothing at all and make the operator try again.
    """
    import httpx

    url = f"{server.rstrip('/')}/api/runner-config"

    try:
        response = httpx.get(url, timeout=5)
    except Exception as exc:
        typer.echo(f"\nCould not reach {url}: {exc}\n", err=True)
        raise typer.Exit(1)

    if response.status_code != 200:
        typer.echo(
            f"\n{url} returned HTTP {response.status_code}. Check the "
            "server address — or the backend there may be too old to have "
            "this endpoint.\n",
            err=True,
        )
        raise typer.Exit(1)

    try:
        data = response.json()
    except Exception:
        typer.echo(
            f"\n{url} did not return JSON. Check the server address — this "
            "usually means it's wrong (e.g. pointing at a login page or a "
            "load balancer's error page instead of the backend).\n",
            err=True,
        )
        raise typer.Exit(1)

    asgardeo_org = data.get("asgardeo_org") if isinstance(data, dict) else None
    asgardeo_client_id = data.get("asgardeo_client_id") if isinstance(data, dict) else None
    if not asgardeo_org or not asgardeo_client_id:
        typer.echo(
            f"\n{url} responded, but the body is missing asgardeo_org or "
            "asgardeo_client_id. Check the server address — or the backend "
            "there may be too old to have this endpoint.\n",
            err=True,
        )
        raise typer.Exit(1)

    return {
        "CLOUD_URL": server,
        "ASGARDEO_ORG": asgardeo_org,
        "ASGARDEO_CLIENT_ID": asgardeo_client_id,
    }


@app.command()
def configure(
    server: str = typer.Option(
        None,
        "--server",
        "-s",
        help="Cloud backend URL, e.g. https://portal.example.com — fetches "
        "the Asgardeo org and client ID from it and saves CLOUD_URL "
        "alongside them, so you never hand edit ~/.wso2-runner/.env",
    ),
):
    """First-time setup wizard — saves your LLM credentials to ~/.wso2-runner/.env."""
    from wso2_runner.browser_install import (
        MANUAL_INSTALL_COMMAND,
        ChromiumInstallError,
        ensure_chromium_installed,
    )
    from wso2_runner.config import CONFIG_DIR, CONFIG_FILE, settings
    from wso2_runner.env_file import read_env_values, write_config_file

    # Fetched before anything is printed or asked, deliberately. A wrong
    # --server is a typo or a stale URL, and the sooner that's reported the
    # better — nobody should have to answer eight prompts about their LLM
    # provider only to be told at the very end that the server address was
    # wrong and none of it was saved anyway.
    server_updates: dict[str, str] = {}
    if server is not None:
        server_updates = _fetch_server_config(server)

    CONFIG_DIR.mkdir(parents=True, exist_ok=True)

    print("\nWSO2 Compliance Runner — setup wizard")
    print("=" * 42)
    print("This saves your config to ~/.wso2-runner/.env\n")

    if server_updates:
        print(f"Fetched Asgardeo org and client ID from {server}\n")

    # Read what's already on disk before asking anything, so a value this
    # wizard already knows (e.g. re-running configure on a machine that's
    # been set up before) can be offered back as the default — pressing
    # Enter keeps it instead of the engineer having to retype it.
    current = read_env_values(CONFIG_FILE)

    email = typer.prompt(
        "Your WSO2 email",
        default=current.get("USER_EMAIL", ""),
        prompt_suffix=" (used to sign in via Asgardeo, and so `wso2-runner start` needs no argument): ",
    )

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

    updates = {"USER_EMAIL": email, "AGENT_PROVIDER": provider, "AGENT_MODEL": model}

    if provider == "azure":
        # No API key prompt here, deliberately — the Runner authorises Azure
        # OpenAI calls with the engineer's own Entra identity (az login),
        # not a shared secret written to disk. See azure_credential.py.
        updates["AZURE_OPENAI_ENDPOINT"] = typer.prompt("Azure OpenAI endpoint (https://...)")
        updates["AZURE_OPENAI_DEPLOYMENT"] = typer.prompt("Deployment name", default=model)
        updates["AZURE_OPENAI_API_VERSION"] = "2024-10-21"
        updates["AZURE_TENANT_ID"] = typer.prompt("Azure tenant ID")
        print("\n  Next: run `az login` to sign in with your own Azure identity.")
        print("  The Runner uses that session to call Azure OpenAI — no key is stored.")
    elif provider == "anthropic":
        updates["ANTHROPIC_API_KEY"] = typer.prompt("Anthropic API key", hide_input=True)
    elif provider == "gemini":
        updates["GEMINI_API_KEY"] = typer.prompt("Gemini API key", hide_input=True)
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
    updates["SCREENSHOT_MONITOR"] = str(monitor)

    if monitor > 1:
        print(f"\n  Tip: drag the agent's Chrome window to monitor {monitor} after it opens.")
        print("  Chrome remembers the position — you only need to do this once.")

    # CLOUD_URL, ASGARDEO_ORG and ASGARDEO_CLIENT_ID come from --server
    # above when it's given, and are otherwise set by hand per the setup
    # docs — this wizard's own prompts never touch them. Merged in here,
    # right before the single write, rather than fetched later, so a run
    # without --server writes exactly the keys it always did.
    updates.update(server_updates)

    # Update the file in place rather than replacing it — a plain
    # write_text() of only the keys collected above used to delete
    # everything else in the file, including these three. See
    # write_config_file() for how the merge, the atomic write, and the file
    # permissions are handled.
    write_config_file(CONFIG_FILE, updates)
    print(f"\nConfig saved to {CONFIG_FILE}")

    # Chromium used to be installed by runner/install.sh's own step 4. That
    # script is going away in favour of a plain wheel install, and neither
    # `uv tool install` nor `pip install` fetches a browser binary — so this
    # is now the only place a fresh machine gets one. See
    # browser_install.py's module docstring for why only BROWSER_CHANNEL ==
    # "chromium" triggers a real check here.
    #
    # A failed install is reported, not raised — the config above is
    # already saved and good, so treating this the same way `start`'s
    # guards do (exit non-zero) would make an otherwise successful
    # `configure` look like it failed outright. `start`'s own guard below
    # will catch this again, for real, if it's still broken by then.
    try:
        ensure_chromium_installed(settings.BROWSER_CHANNEL)
    except ChromiumInstallError as exc:
        typer.echo(
            f"\n[runner] {exc}. Run this yourself, then try again:\n\n"
            f"    {MANUAL_INSTALL_COMMAND}\n",
            err=True,
        )

    print("Next: wso2-runner start  (opens a browser to sign in via Asgardeo)\n")


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

    # Guard, not just a one-time setup step: `configure` already tries this,
    # but the browser could still be missing here -- an engineer skipping
    # `configure`'s prompt on a flaky connection, someone else's machine
    # image, `~/.cache` cleared by hand, etc. Checked here, before the poll
    # loop (and therefore any browser) starts, for the same reason the
    # Azure check above runs before the loop rather than lazily on first
    # use: a broken environment should fail once, plainly, right here --
    # not resurface as a cryptic Playwright exception after a task has
    # already been pulled off the queue.
    from wso2_runner.browser_install import (
        MANUAL_INSTALL_COMMAND,
        ChromiumInstallError,
        ensure_chromium_installed,
    )

    try:
        ensure_chromium_installed(settings.BROWSER_CHANNEL)
    except ChromiumInstallError as exc:
        typer.echo(
            f"\n[runner] {exc}. Run this yourself, then try again:\n\n"
            f"    {MANUAL_INSTALL_COMMAND}\n",
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

    # Its own numbered section rather than folded into [3]: [3] is about
    # whether a browser can launch at all, which is a Chromium/Playwright
    # concern. This is about whether the OS is willing to hand the Runner
    # real pixels once something IS on screen -- a completely different
    # failure mode (a missing OS permission, not a missing browser), with
    # its own separate fix. Keeping them apart means a ✓ on [3] can never
    # be misread as "screenshots will work too".
    print("\n[5] Screen capture sanity check")
    from wso2_runner import capture_check

    try:
        test_capture = capture_check.capture_test_screenshot()
    except Exception as exc:
        # Not this check's problem to diagnose further -- [3] above already
        # covers a broken browser/display loudly. This is just "couldn't
        # even try", reported plainly like every other check here.
        print(f"    ✗ Could not take a test capture: {exc}")
    else:
        if capture_check.looks_blank(test_capture):
            # This is a heuristic, not a permission API call — see
            # capture_check.py's module docstring. It must only ever warn:
            # a legitimately plain screen would trip it too, so this can
            # never be treated as a hard failure of `doctor`, and nothing
            # about it ever gates `start`.
            print("    ✗ Test capture looks blank (almost no colour variation)")
            print(capture_check.BLANK_CAPTURE_ADVICE)
        else:
            print("    ✓ Test capture looks fine")

    print()
