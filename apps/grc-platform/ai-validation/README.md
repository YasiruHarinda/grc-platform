# AI Evidence Validation

Advisory AI pre-review of audit evidence for the GRC Platform. After evidence is
submitted, this module reviews it against the control's evidence requirement and
previous submissions, then writes an **advisory** result (`PASS` / `FAIL` /
`UNCERTAIN`) plus actionable feedback. Results are **hints only** — they never
block or change evidence/control status.

Built to threat model **[04] — Asynchronous AI evidence validation**. Full design:
[`AI-Validation-Agent-MCP-Design.md`](../backend/Resources/Docs/AI-Validation-Agent-MCP-Design.md).
Deployment runbook: [`AI-Validation-Rollout-Checklist.md`](../backend/Resources/Docs/AI-Validation-Rollout-Checklist.md).

## What's in here

One Go module, **two binaries → two Choreo Service components** (both
project-internal, never public):

| Binary | Port | Role |
|---|---|---|
| **`cmd/agent`** — Validation Agent | 8090 | Triggered fire-and-forget by the GRC backend. Runs the Anthropic tool loop and records the result. **Holds `ANTHROPIC_API_KEY`.** |
| **`cmd/mcpserver`** — MCP Server | 8091 | The agent's only data surface. Serves scoped MCP tools over Streamable HTTP; reads/writes everything via the Compliance Entity. **Never holds the Azure key or DB access.** |

```
GRC Backend ──POST /api/v1/validations (Bearer AGENT_API_KEY)──▶ Agent
Agent ──session bootstrap + MCP tool calls (Bearer session token)──▶ MCP Server
Agent ──tool loop──▶ Anthropic API (claude-sonnet-4-6)
MCP Server ──HTTP──▶ Compliance Entity ──▶ MySQL / Azure Blob
```

Credential separation is deliberate: compromising one component does not yield the
other's secrets.

## Requirements

- **Go 1.25+** (see `go.mod`).
- A running **Compliance Entity** the MCP server can reach (`COMPLIANCE_ENTITY_BASE_URL`),
  with the Phase-0 schema migration applied and the `GET /evidence-files/{fileId}/content`
  route live.
- An **Anthropic API key** for the agent (`ANTHROPIC_API_KEY`). A Claude Pro/Max
  subscription is **not** API access — get a pay-as-you-go key from
  [console.anthropic.com](https://console.anthropic.com).

## Configuration

### Agent (`cmd/agent`, port 8090)

| Variable | Required | Default | Notes |
|---|---|---|---|
| `ANTHROPIC_API_KEY` | ✅ | — | Secret. Anthropic API key. |
| `AGENT_API_KEY` | ✅ | — | Secret. Inbound bearer the GRC backend must present. |
| `MCP_SHARED_SECRET` | ✅ | — | Secret. Must match the MCP server's. |
| `ANTHROPIC_MODEL` | | `claude-sonnet-4-6` | Model id. |
| `MCP_BASE_URL` | | `http://localhost:8091` | Internal URL of the MCP server. |
| `VALIDATION_TIMEOUT_SECONDS` | | `300` | Per-job timeout. |
| `MAX_LOOP_ITERATIONS` | | `12` | Tool-loop hard cap. |
| `MAX_FILES_PER_JOB` | | `12` | Files fetched per job. |
| `PORT` | | `:8090` | Listen address. |
| `LOG_LEVEL` | | `info` | `info` or `debug`. |
| `ANTHROPIC_BASE_URL` | | — | Optional. Honoured by the SDK if you front the API with a gateway. Leave unset to hit `api.anthropic.com`. |

### MCP Server (`cmd/mcpserver`, port 8091)

| Variable | Required | Default | Notes |
|---|---|---|---|
| `MCP_SHARED_SECRET` | ✅ | — | Secret. Must match the agent's. |
| `COMPLIANCE_ENTITY_BASE_URL` | | `http://localhost:8081` | Internal URL of the Compliance Entity. |
| `SESSION_TTL_SECONDS` | | `600` | Session-token lifetime. |
| `MAX_FILE_BYTES_TO_LLM` | | `10485760` | 10 MiB per-file cap. |
| `PORT` | | `:8091` | Listen address. |
| `LOG_LEVEL` | | `info` | `info` or `debug`. |

> `MCP_SHARED_SECRET` must be **identical** in both components. The agent's
> `AGENT_API_KEY` must match the backend's `AI_AGENT_API_KEY`. Generate secrets with
> `openssl rand -hex 32`.

## Build

```bash
# from apps/grc-platform/ai-validation
go build ./...                       # compile everything
go vet ./...                         # static checks

go build -o bin/agent     ./cmd/agent
go build -o bin/mcpserver ./cmd/mcpserver
```

## Run locally

Start the MCP server first, then the agent (each needs its env). Requires a reachable
Compliance Entity and a real Anthropic key.

```bash
# MCP server
COMPLIANCE_ENTITY_BASE_URL=http://localhost:8081 \
MCP_SHARED_SECRET=$SECRET \
go run ./cmd/mcpserver

# Agent
ANTHROPIC_API_KEY=sk-ant-... \
AGENT_API_KEY=$AGENT_KEY \
MCP_SHARED_SECRET=$SECRET \
MCP_BASE_URL=http://localhost:8091 \
go run ./cmd/agent
```

Trigger a validation (as the GRC backend does):

```bash
curl -XPOST -H "Authorization: Bearer $AGENT_KEY" \
  http://localhost:8090/api/v1/validations \
  -d '{"task":"validate_evidence","scope":{"auditId":1,"controlId":2,"evidenceId":3},"requestedBy":"you@wso2.com"}'
# → 202 {"jobId":"vj_..."}
```

Health checks: `GET http://localhost:8090/healthz` and `GET http://localhost:8091/healthz`.

## HTTP endpoints

**Agent**
- `POST /api/v1/validations` — trigger a job. Bearer `AGENT_API_KEY`. Body:
  `{ "task": "validate_evidence", "scope": {"auditId","controlId","evidenceId"}, "requestedBy" }`.
  Returns `202 {"jobId"}`. `401` bad bearer, `400` unknown task / bad scope.
- `GET /healthz` — `200 ok`.

**MCP Server**
- `POST /internal/sessions` — agent bootstraps a scoped session. Bearer `MCP_SHARED_SECRET`.
- `POST /internal/lifecycle` — agent writes `PENDING` / `ERROR` rows. Bearer `MCP_SHARED_SECRET`.
- `/mcp` — MCP Streamable HTTP. Bearer = per-job session token.
- `GET /healthz` — `200 ok`.

## MCP tools (v1)

| Tool | Purpose |
|---|---|
| `get_validation_context` | Control requirement + current/previous submissions (external comments only) + recent trail, in one call. |
| `get_evidence_file` | One file's content by `fileId` (scope-checked). PDF/image native; xlsx → CSV; oversized/unsupported → tool error. |
| `submit_validation_result` | Terminal. Validated server-side, writes the advisory row + `AI_VALIDATED` trail, revokes the session. |

Adding a new AI feature = a new `TaskSpec` in `internal/agent/task/` (+ optional MCP
tool + a trigger call site); the loop, bridge, session, and auth code stay unchanged.

## Docker

Each binary has its own multi-stage distroless image. The **build context is the module
root** so both share `internal/`.

```bash
# from apps/grc-platform/ai-validation
docker build -f docker/agent.Dockerfile     -t ai-validation-agent .
docker build -f docker/mcpserver.Dockerfile -t ai-validation-mcp   .
```

Choreo console values (Service component, Dockerfile buildpack, **Project** visibility)
are in the design doc §3.1 and the rollout checklist.

## Two Choreo components from one folder

A Choreo **component is a build-and-deploy unit created *from* a repo**, not a repo
itself — you can create several from the same repo, even the same folder. Each is defined
by three wizard fields: the **Component Directory** (where its `.choreo/component.yaml`
lives, declaring port + visibility), the **Dockerfile path**, and the **Docker build
context** (the `.` for `COPY`).

This module ships **two** services that share `internal/`, so you run the "Create
Component" wizard **twice**: each time the Component Directory points at a different
`cmd/` subfolder (different port), and a different Dockerfile — but **both use the same
build context** (the module root) so each Dockerfile can `COPY` the shared code and build
just its own `cmd/` target.

```
apps/grc-platform/ai-validation/          ◄── build context for BOTH
├── go.mod, internal/                     (shared — copied by both)
├── cmd/agent/.choreo/component.yaml       ◄── component #1 dir  (port 8090)
├── cmd/mcpserver/.choreo/component.yaml   ◄── component #2 dir  (port 8091)
└── docker/
    ├── agent.Dockerfile      → go build ./cmd/agent
    └── mcpserver.Dockerfile  → go build ./cmd/mcpserver
```

Wizard values (paths are relative to the repo root `grc-platform/`):

| Field | `ai-validation-agent` | `ai-validation-mcp` |
|---|---|---|
| Type | Service | Service |
| Component Directory | `apps/grc-platform/ai-validation/cmd/agent` | `apps/grc-platform/ai-validation/cmd/mcpserver` |
| Buildpack | Dockerfile | Dockerfile |
| Dockerfile path | `apps/grc-platform/ai-validation/docker/agent.Dockerfile` | `apps/grc-platform/ai-validation/docker/mcpserver.Dockerfile` |
| Docker context | `apps/grc-platform/ai-validation` | `apps/grc-platform/ai-validation` |
| Endpoint visibility | **Project** | **Project** |

After both deploy, wire them by their internal (Project) URLs: put the MCP server's URL in
the agent's `MCP_BASE_URL`, and the agent's URL in the backend's `AI_AGENT_BASE_URL`.

> This is also **why Dockerfiles instead of the Go buildpack**: the buildpack assumes one
> directory = one binary, but here one directory produces two binaries that share
> `internal/`. A Dockerfile lets both components use the module root as context and each
> select its own `cmd/` target.

## Layout

```
cmd/agent/          Validation Agent entrypoint + .choreo/component.yaml
cmd/mcpserver/      MCP Server entrypoint + .choreo/component.yaml
internal/agent/     server, job runner, Anthropic loop, bridge, task registry, MCP client
internal/mcpserver/ MCP server, sessions, the 3 tools, xlsx extraction, entity client
internal/config/    env loading for both binaries
docker/             agent.Dockerfile, mcpserver.Dockerfile
```
