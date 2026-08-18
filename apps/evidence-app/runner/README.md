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

Access comes from the **Cognitive Services OpenAI User** role on the Azure
OpenAI resource. Today that role is granted **per person**, so a new joiner
has to be granted it individually before their first run, and it has to be
removed individually when they leave. Ask whoever owns the Azure OpenAI
resource to grant it to you.

Moving this to a single Entra group, so joining the group is all that is
needed, is planned but not done. Until then, `wso2-runner doctor` and
`wso2-runner start` both check that you can actually call the endpoint, so
a missing role shows up before a run starts rather than part-way through
one.

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

### `wso2-runner doctor` — Azure access states

In `entra` mode (the default), `doctor` signs in to Azure and then makes one
real, free call to your Azure OpenAI endpoint. Both halves matter: signing in
proves who you are, and only the call proves you are allowed to use it. Azure
hands a token to anyone in the tenant, and the role assignment is checked by
the endpoint when the token is presented — so a sign-in that succeeds is not
on its own proof that anything will work.

| State | Meaning | Fix |
| --- | --- | --- |
| Azure CLI is not installed | `az` isn't on your machine | Install the Azure CLI, then run `az login` |
| Azure CLI is installed but nobody is signed in | No cached session | Run `az login` |
| Signed in to the wrong tenant | Rejected at sign-in. Your CLI holds a session, but not for the tenant in your config | Run `az account show`, compare its `tenantId` to `AZURE_TENANT_ID`, then `az login --tenant <AZURE_TENANT_ID>` |
| Signed in, but not allowed to call Azure OpenAI | You are authenticated and the endpoint refused the call. You are missing the Azure OpenAI role | Ask an administrator to grant you the role. `az login` will not fix this |
| Access could not be confirmed | `AZURE_OPENAI_ENDPOINT` isn't set, or it couldn't be reached | Set the endpoint, or check your network. The Runner still starts |
| Azure OpenAI access works | Everything is working | Nothing to do |

`wso2-runner start` runs the same check before it takes any work, so a missing
role stops you at the terminal rather than halfway through a task. The one
exception is "access could not be confirmed": that only warns, and the Runner
starts anyway — a check that can't reach a verdict must not stop a Runner that
would have worked.

In `api_key` mode (rollout-period only), `doctor` just checks that
`AZURE_OPENAI_API_KEY` is set, same as before.
