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
`ollama`.

**Azure (the normal path) needs no key.** Run `az login` once, and set
`AZURE_OPENAI_ENDPOINT`, `AZURE_OPENAI_DEPLOYMENT` and `AZURE_TENANT_ID` in
your `.env` (see `.env.example`). The Runner then authorises every Azure
OpenAI call with your own signed-in Azure identity — no credential is ever
written to disk. `wso2-runner configure` asks for these and reminds you to
run `az login`; it never asks for a key.

Access is granted through **one Entra group**, assigned the Azure OpenAI
role once on the Azure OpenAI resource. A new joiner is added to that
group and needs no per-person Azure change; removing someone from the
group ends their access.

Anthropic and Gemini still use a plain API key — set the matching key in
your `.env`. Ollama needs no key at all.

### When your Azure sign-in expires

Azure sign-ins expire periodically (governed by WSO2 IT's Conditional
Access policy — there's no fixed number to plan around). You don't need to
watch for this yourself: `wso2-runner start` checks your Azure sign-in
*before* it starts polling for tasks, and if a task is already running when
your session expires mid-run, the Runner fails that task with a plain
message and stops rather than continuing to poll uselessly. Either way, the
fix is the same:

```bash
az login
```

Then start the Runner again.

### `wso2-runner doctor` — Azure sign-in states

In `entra` mode (the default), `doctor` attempts a real Azure sign-in and
reports one of five states. The Azure library can't always tell "wrong
tenant" apart from "missing the role" by itself, so `doctor` names both
possibilities and tells you how to tell them apart with `az account show`:

| State | Meaning | Fix |
| --- | --- | --- |
| Azure CLI is not installed | `az` isn't on your machine | Install the Azure CLI, then run `az login` |
| Azure CLI is installed but nobody is signed in | No cached session | Run `az login` |
| Signed in to the wrong tenant | Rejected — run `az account show` and compare its `tenantId` to `AZURE_TENANT_ID` in your config; they differ | `az login --tenant <AZURE_TENANT_ID>` |
| Signed in to the right tenant but missing the role | Rejected — `az account show`'s `tenantId` matches `AZURE_TENANT_ID`, so it's not a tenant problem | Ask an admin to check your membership in the Azure OpenAI access group |
| Azure sign-in works — token acquired | Everything is working | Nothing to do |

In `api_key` mode (rollout-period only), `doctor` just checks that
`AZURE_OPENAI_API_KEY` is set, same as before.
