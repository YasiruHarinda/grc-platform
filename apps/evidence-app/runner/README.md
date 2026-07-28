# Compliance Evidence Portal — Runner

`wso2-runner` is the local browser-automation agent. It runs on the engineer's
own machine (not in Docker or Choreo) because it needs a headful Chromium for
SSO/MFA login and OS-level screen capture. It polls the backend for tasks,
drives cloud consoles, captures evidence screenshots, and posts results back.

## Why it runs locally

- **Headful Chromium** with a persistent profile (`~/.wso2-runner/browser_profile/`)
  keeps you logged in across tasks after a single manual MFA login.
- **OS screen capture** (`mss`) needs a real display, so this cannot run in a
  container.

## Install

> **Note:** the packaging/distribution approach is not finalised yet (a pre-built
> binary bundle is under consideration). `install.sh` is the current dev/local
> install path and may be replaced.

```bash
bash install.sh          # creates ~/.wso2-runner/venv, installs the CLI + Chromium
wso2-runner configure    # interactive setup — writes ~/.wso2-runner/.env
wso2-runner doctor       # sanity check (Python, browser, monitors, backend)
wso2-runner start        # start polling the backend
```

For local development against a source checkout:

```bash
python3.11 -m venv venv && source venv/bin/activate
pip install -e .
python -m playwright install chromium
```

## Configuration

Settings load from `runner/.env` then `~/.wso2-runner/.env` (the latter wins).
See [`.env.example`](.env.example). The runner authenticates to the backend via
Asgardeo (PKCE) using its own native-app client ID.

## Layout

```
wso2_runner/
  cli.py       Typer CLI: start / configure / doctor
  loop.py      Polling loop — claims and runs one task at a time
  agent.py     Chromium session, LLM factory, screenshot capture, Azure helpers
  client.py    httpx wrapper for backend REST calls
  oauth.py     Asgardeo PKCE login
  config.py    Settings loaded from environment
```

## LLM providers

`AGENT_PROVIDER` selects the model backend: `azure`, `anthropic`, `gemini`, or
`ollama`. Set the matching keys in your `.env` (see `.env.example`).
