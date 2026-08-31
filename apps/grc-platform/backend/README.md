# GRC Platform Backend

Go backend service for the Governance, Risk & Compliance (GRC) platform. The platform has two modules, **Risk Hub** and **Audit Hub**.

## Quick Start

```bash
# from backend/
set -a && source .env && set +a && go run ./cmd/server
```

Backend starts at `http://localhost:8081`.

## Overview

- Default port: `:8081`
- Runtime: Go `1.23+`
- Entry point: `cmd/server/main.go`
- Authentication: Asgardeo JWT Bearer token — validated via JWKS endpoint; pass as `Authorization: Bearer <token>` header
- Two modules: **Risk Hub** (`/api/v1/risks/`) and **Audit Hub** (`/api/v1/audits/`)
- Data layer: this service holds no database handle and never talks to Azure directly. Every read/write goes through the **Compliance Entity** (`COMPLIANCE_ENTITY_BASE_URL`), a separate service that owns MySQL and Azure Blob Storage

## Prerequisites

- Go `1.23+` — [install](https://go.dev/doc/install)

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with the race detector
go test -race ./...

# Run a specific package
go test ./internal/risk/...
go test ./internal/audit/...

# Check test coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Or use `make`:

```bash
make test    # vet + test
make build   # vet + test + compile
```

### Run tests before every push (recommended)

Set up the shared git hook once from the repo root:

```bash
git config core.hooksPath .githooks
```

Or from the backend directory:

```bash
make setup
```

After this, `git push` automatically runs `go test ./...` whenever backend files are in the push. If any test fails, the push is aborted.

(This requires a `pre-push` script under a `.githooks/` directory at the repo root — that directory is not currently committed to this repository, so `make setup`/`git config core.hooksPath .githooks` alone configures the hook path but has no script to run yet.)

To skip the hook in exceptional cases:

```bash
git push --no-verify
```

## Configuration

Copy `.env.example` to `.env` and fill in the values:

### Compliance Entity

There is no database or Azure configuration here: the backend holds no DB handle and never talks to Azure directly. All data access and file storage go through the Compliance Entity.

| Variable | Description |
|---|---|
| `COMPLIANCE_ENTITY_BASE_URL` | Base URL of the Compliance Entity (data layer + Azure Blob Storage), e.g. `http://localhost:8080` |

### Asgardeo JWT

| Variable | Description |
|---|---|
| `AUTH_JWKS_ENDPOINT` | Asgardeo JWKS URL |
| `AUTH_ISSUER` | Expected `iss` claim |
| `AUTH_AUDIENCE` | Expected `aud` claim |
| `AUTH_TOKEN_VALIDATOR_ENABLED` | Set to `false` to skip signature verification locally (default `true`). Disables every privilege check too (allow-all) — requires `APP_ENV=local` or the server refuses to start |
| `APP_ENV` | Set to `local` to permit `AUTH_TOKEN_VALIDATOR_ENABLED=false`. Leave unset in every deployed environment |

### HR Entity

Used by the Risk module's "Risk Identified By: Employee" search. Required (`mustEnv`).

| Variable | Description |
|---|---|
| `HR_ENTITY_GRAPHQL_URL` | WSO2 HR entity GraphQL endpoint (local: `go run ./cmd/hrmock`) |
| `HR_ENTITY_TOKEN_URL` | OAuth2 token URL for the HR entity |
| `HR_ENTITY_CLIENT_ID` | OAuth2 client ID |
| `HR_ENTITY_CLIENT_SECRET` | OAuth2 client secret |

### Asgardeo SCIM2 (identity directory)

Resolves a person's uuid to their name/email (names and emails are no longer stored in this platform's own data). Optional — an unset `SCIM_BASE_URL` disables directory lookups rather than failing startup. Internal users and external auditors live in separate Asgardeo organizations, each with its own app registration.

| Variable | Description |
|---|---|
| `SCIM_BASE_URL` | Shared Asgardeo API root, e.g. `https://api.asgardeo.io` |
| `SCIM_USER_DOMAIN` | Email-domain suffix the bulk directory cache is scoped to (default `wso2.com`) |
| `SCIM_INTERNAL_ORG` | Internal-org Asgardeo tenant |
| `SCIM_INTERNAL_CLIENT_ID` / `SCIM_INTERNAL_CLIENT_SECRET` | Internal-org OAuth2 credentials |
| `SCIM_INTERNAL_TOKEN_URL` | `https://api.asgardeo.io/t/{SCIM_INTERNAL_ORG}/oauth2/token` |
| `SCIM_INTERNAL_SCOPES` | Space-separated scopes (default `internal_user_mgt_view internal_user_mgt_list`) |
| `SCIM_EXTERNAL_ORG` | External-org (auditor) Asgardeo tenant |
| `SCIM_EXTERNAL_CLIENT_ID` / `SCIM_EXTERNAL_CLIENT_SECRET` | External-org OAuth2 credentials |
| `SCIM_EXTERNAL_TOKEN_URL` | `https://api.asgardeo.io/t/{SCIM_EXTERNAL_ORG}/oauth2/token` |
| `SCIM_EXTERNAL_SCOPES` | Space-separated scopes (default `internal_user_mgt_view internal_user_mgt_list`) |

### Email notifications

Connection details for the shared `email-service` (`POST /send-email`), used for risk/audit notification emails. Required (`mustEnv`).

| Variable | Description |
|---|---|
| `EMAIL_SERVICE_URL` | Base URL of email-service |
| `EMAIL_FROM_ADDRESS` | From address for outgoing notifications |
| `EMAIL_CLIENT_ID` / `EMAIL_CLIENT_SECRET` | OAuth2 client credentials |
| `EMAIL_TOKEN_URL` | OAuth2 token URL |
| `FRONTEND_BASE_URL` | Public URL of the webapp — builds links inside notification emails, and doubles as the CORS-allowed origin (there is no separate CORS variable) |

### AI Validation (optional)

| Variable | Description |
|---|---|
| `AI_VALIDATION_ENABLED` | Set to `true` to trigger the AI Validation Agent after an evidence submission (default disabled) |
| `AI_AGENT_BASE_URL` | Agent base URL (default `http://localhost:8090`) |
| `AI_AGENT_API_KEY` | Agent API key |

### Audit overdue lead escalation (optional)

| Variable | Description |
|---|---|
| `AUDIT_LEAD_ESCALATION_ENABLED` | `true`/`false` to override the built-in default (`false`) — emails an overdue item owner's HR line manager |

### Server

| Variable | Description |
|---|---|
| `PORT` | Listen address (default `:8081`) |

## Project Structure

```text
backend/
├── cmd/
│   ├── server/                      # Entry point
│   │   ├── main.go                  # Middleware chain + route registration
│   │   ├── risk_deps.go             # Wires Risk Hub service/repository dependencies
│   │   └── audit_deps.go            # Wires Audit Hub service/repository dependencies
│   ├── backfill-uuids/              # One-off: backfills `user`.uuid from Asgardeo SCIM (email → uuid)
│   ├── backfill-escalation-leads/   # One-off: backfills risk_escalation's frozen lead uuid columns
│   └── backfill-evidence-actors/    # One-off: rewrites risk_evidence.created_by from email to uuid
├── internal/
│   ├── config/config.go            # Env var loading (mustEnv)
│   ├── apierror/apierror.go        # Typed API error with HTTP status
│   ├── response/response.go        # JSON write helpers
│   ├── middleware/
│   │   ├── auth.go                 # Asgardeo JWT validation, UserInfo → context
│   │   ├── correlation.go          # X-Correlation-ID generation + slog injection
│   │   ├── logger.go               # Per-request structured logging
│   │   ├── cors.go                 # CORS headers (allowed origin = FRONTEND_BASE_URL)
│   │   └── security_headers.go     # Baseline security response headers
│   ├── shared/
│   │   ├── auth/auth.go            # HasPrivilege / RequirePrivilege helpers (no role constants)
│   │   ├── privilege/privilege.go  # Privilege name constants + Store (role→privilege map, loaded from the Compliance Entity)
│   │   ├── grant/                  # (role, scope) grant repository — create/revoke/candidates, backs the Admin Console
│   │   ├── entityclient/client.go  # Typed HTTP client to the Compliance Entity (all data access goes through this)
│   │   ├── file/file.go            # HTTP client to the Compliance Entity's file (Azure Blob) endpoints
│   │   ├── emailer/                # Client for the shared email-service (risk/audit notification sends)
│   │   ├── aiagent/client.go       # Fire-and-forget trigger to the AI Validation Agent
│   │   └── blobpath/blobpath.go    # Blob path construction helpers
│   ├── directory/directory.go      # uuid → name/email resolution via SCIM, with caching
│   ├── hrentity/client.go          # WSO2 HR entity GraphQL client (employee search)
│   ├── scim/client.go              # Asgardeo SCIM2 client (internal org + external/auditor org)
│   ├── user/                       # Shared user entity (both modules reference it)
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── entity/repository.go    # Compliance Entity-backed implementation
│   │   └── handler/                # GET /api/v1/me/profile (routes.go)
│   ├── admin/                      # Admin Console (user provisioning, roles, grants)
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── entity/repository.go
│   │   └── handler/                # HTTP handlers + route registration (routes.go)
│   ├── risk/                       # Risk Hub
│   │   ├── model/                  # Domain types and request/response structs
│   │   ├── repository/             # Interfaces (repository.go) + Compliance Entity client (entity/)
│   │   ├── service/                # Business logic — workflow rules, validations
│   │   ├── handler/                # HTTP handlers + route registration (routes.go)
│   │   └── job/                    # Daily overdue-risk escalation job
│   └── audit/                      # Audit Hub
│       ├── model/
│       ├── repository/
│       ├── service/
│       ├── handler/
│       └── job/                    # Daily due-date reminder digest job
└── tests/integration/              # Integration tests against a real MySQL database — skipped unless DB_DSN is set
```

**Request flow through the layers:**
```
HTTP request
    → middleware (SecurityHeaders → CORS → CorrelationID → Logger → Auth)
    → handler   (parse request, call service, write response)
    → service   (business rules, status transition guards, changelog/trail writes)
    → repository (typed HTTP calls to the Compliance Entity via internal/shared/entityclient — no direct SQL)
    → Compliance Entity (owns MySQL and Azure Blob Storage)
```

## API Endpoints

### Current User

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/me/profile` | Current user's profile |
| `GET` | `/api/v1/me/privileges` | Current user's resolved privilege set (unions RISK_* and AUDIT_* names; both hubs' frontends call this same endpoint) |

### Admin

Every route below requires `MANAGE_USERS` GLOBAL.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/admin/directory/search` | Search the WSO2-org directory |
| `GET` | `/api/v1/admin/directory/search-external` | Search the external-org directory |
| `POST` | `/api/v1/admin/users` | Provision a platform user |
| `GET` | `/api/v1/admin/users` | List platform users |
| `PATCH` | `/api/v1/admin/users/{id}/status` | Update a user's status |
| `POST` | `/api/v1/admin/users/{id}/grants` | Create a (role, scope) grant |
| `DELETE` | `/api/v1/admin/users/{id}/grants/{grantId}` | Revoke a grant |
| `GET` | `/api/v1/admin/roles` | List roles |

### Risk Hub

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/risks/teams` | List risk teams |
| `POST` | `/api/v1/risks/teams` | Create team |
| `PUT` | `/api/v1/risks/teams/{id}` | Update team |
| `GET` | `/api/v1/risks/scores` | List risk scores (read-only; the likelihood x impact matrix is fixed) |
| `GET` | `/api/v1/risks/compliance-references` | List compliance references |
| `POST` | `/api/v1/risks/compliance-references` | Create compliance reference |
| `PUT` | `/api/v1/risks/compliance-references/{id}` | Update compliance reference |
| `DELETE` | `/api/v1/risks/compliance-references/{id}` | Delete compliance reference |
| `GET` | `/api/v1/risks/categories` | List risk categories |
| `POST` | `/api/v1/risks/categories` | Create risk category |
| `PUT` | `/api/v1/risks/categories/{id}` | Update risk category |
| `DELETE` | `/api/v1/risks/categories/{id}` | Delete risk category |
| `GET` | `/api/v1/risks/users` | List users |
| `POST` | `/api/v1/risks/users/resolve` | Resolve an employee email to an internal user, provisioning one if needed |
| `GET` | `/api/v1/risks/management-approvers` | List management-approver candidates |
| `GET` | `/api/v1/risks/owner-candidates` | List risk-owner candidates |
| `GET` | `/api/v1/risks/assigner-candidates` | List risk-assigner candidates |
| `GET` | `/api/v1/risks/employees/search` | Search employees (HR entity) |
| `GET` | `/api/v1/risks/next-sequence-id` | Preview the next risk sequence ID for a register/year/quarter |
| `GET` | `/api/v1/risks` | List risks |
| `POST` | `/api/v1/risks` | Register a risk |
| `GET` | `/api/v1/risks/{id}` | Get risk by ID |
| `PUT` | `/api/v1/risks/{id}` | Update risk |
| `POST` | `/api/v1/risks/{id}/owner-approve` | Risk owner approves closure |
| `POST` | `/api/v1/risks/{id}/management-approve` | Management approves |
| `POST` | `/api/v1/risks/{id}/approve` | Compliance approves |
| `POST` | `/api/v1/risks/{id}/reject` | Compliance rejects |
| `POST` | `/api/v1/risks/{id}/complete` | Complete remediation |
| `POST` | `/api/v1/risks/{id}/resubmit` | Resubmit after rejection |
| `POST` | `/api/v1/risks/{id}/close` | Compliance closes |
| `POST` | `/api/v1/risks/{id}/cancel` | Cancel risk |
| `POST` | `/api/v1/risks/{id}/assess` | Management assessment |
| `GET` | `/api/v1/risks/dashboard` | Risk Hub dashboard |
| `GET` | `/api/v1/risks/analytics/summary` | Risk analytics summary |
| `POST` | `/api/v1/risks/{id}/action-plans` | Create action plan |
| `GET` | `/api/v1/risks/{id}/action-plans` | List action plans |
| `GET` | `/api/v1/risks/{id}/action-plans/{planId}/steps` | List an action plan's steps |
| `PATCH` | `/api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}` | Update a step |
| `POST` | `/api/v1/risks/{id}/action-plans/{planId}/complete` | Complete an action plan |
| `POST` | `/api/v1/risks/{id}/escalate` | Escalate to management |
| `GET` | `/api/v1/risks/{id}/escalations` | Escalation history |
| `GET` | `/api/v1/risks/{id}/history` | Full risk history (workflow events + field edits) |
| `POST` | `/api/v1/risks/{id}/escalations/{escalationId}/comment` | Answer an escalation, returning the risk to its assigner |
| `POST` | `/api/v1/risks/escalations/run` | Manually trigger the daily overdue-risk escalation sweep (whole batch; requires `MANAGE_RISK_HUB` GLOBAL) |
| `POST` | `/api/v1/risks/{id}/evidence` | Upload evidence |
| `GET` | `/api/v1/risks/{id}/evidence` | List evidence |
| `DELETE` | `/api/v1/risks/{id}/evidence/{fileId}` | Delete an evidence file |
| `GET` | `/api/v1/risks/{id}/evidence/{fileId}/download` | Download an evidence file |

### Audit Hub

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/audits/dashboard` | Audit Hub dashboard |
| `GET` | `/api/v1/audits/work-queue` | Auditor's work queue |
| `GET` | `/api/v1/audits/frameworks` | List frameworks |
| `POST` | `/api/v1/audits/frameworks` | Create framework |
| `GET` | `/api/v1/audits/frameworks/controls?frameworkId={id}` | List a framework's controls |
| `POST` | `/api/v1/audits/frameworks/controls` | Add a control to a framework (frameworkId in body) |
| `GET` | `/api/v1/audits/products` | List products |
| `POST` | `/api/v1/audits/products` | Create product |
| `GET` | `/api/v1/audits/users` | List Audit Hub users |
| `GET` | `/api/v1/audits/auditor-candidates` | List auditor candidates |
| `GET` | `/api/v1/audits/teams` | List audit teams |
| `POST` | `/api/v1/audits/teams` | Create audit team |
| `PUT` | `/api/v1/audits/teams/{id}` | Update audit team |
| `POST` | `/api/v1/audits/reminders/run` | Manually trigger the due-date reminder digest |
| `GET` | `/api/v1/audits` | List audits |
| `POST` | `/api/v1/audits` | Create audit |
| `GET` | `/api/v1/audits/{id}` | Get audit by ID |
| `PUT` | `/api/v1/audits/{id}` | Update audit |
| `DELETE` | `/api/v1/audits/{id}` | Delete audit |
| `GET` | `/api/v1/audits/{id}/controls` | List controls |
| `POST` | `/api/v1/audits/{id}/controls` | Add control |
| `POST` | `/api/v1/audits/{id}/controls/bulk` | Bulk-add controls |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}` | Get control |
| `PUT` | `/api/v1/audits/{id}/controls/{controlId}` | Update control |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}` | Delete control |
| `PATCH` | `/api/v1/audits/{id}/controls/{controlId}/status` | Update control status |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/status/override` | Override control status |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/trail` | Per-control history |
| `GET` | `/api/v1/audits/{id}/trail` | Audit-wide activity log |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/evidence/upload-link` | Get an evidence upload link |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/upload` | Upload an evidence file |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/submit` | Submit an evidence round |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/files` | Add files to the current evidence round |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/withdraw` | Withdraw an evidence submission |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/review` | Internal review decision on evidence |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/validate` | Auditor decision on evidence |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/evidence/files/{fileId}` | Delete one evidence file |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}` | Delete an evidence round |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/evidence` | List evidence |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/evidence/files/{fileId}/download` | Download an evidence file |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/population/upload-link` | Get a population upload link |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/population/upload` | Upload a population file |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/population/submit` | Submit a population round |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/population/review` | Internal review decision on population |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/population/validate` | Auditor decision on population |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/population` | View the current population round |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/population/files/{fileId}` | Delete a population/sample file |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/population/attestation` | Clear the population submission note |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/population/files/{fileId}/download` | Download a population file |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/sample/upload-link` | Get a sample upload link |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/sample/upload` | Upload a sample file |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/sample/submit` | Submit a sample selection |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/sample/request-time` | Request more time for sample selection |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/comments` | List comments |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/comments` | Add comment |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/comments/{commentId}` | Delete comment |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/ai-validations` | List AI validation results |

## Run Locally

**Start the server:**
```bash
set -a && source .env && set +a && go run ./cmd/server
```

When `AUTH_TOKEN_VALIDATOR_ENABLED=false`, JWT signature verification is skipped — the token is still decoded so user info is populated. Pass any valid-structure JWT as the Bearer token for local testing. This also requires `APP_ENV=local` to be set, or `config.Load()` refuses to start — see [Asgardeo JWT](#asgardeo-jwt).

### Examples

```bash
JWT="<your-jwt-token>"

# Health check
curl http://localhost:8081/health

# Get current user profile
curl -H "Authorization: Bearer $JWT" http://localhost:8081/api/v1/me/profile

# List risks
curl -H "Authorization: Bearer $JWT" http://localhost:8081/api/v1/risks

# Register a risk (see internal/risk/model/risk.go's CreateRiskRequest for the
# full field list — creation itself submits the risk, there is no separate
# /submit endpoint)
curl -X POST http://localhost:8081/api/v1/risks \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{
        "year": 2026, "quarter": "Q1", "source_register_id": 1,
        "risk_title": "Unauthorised data access",
        "risk_description": "...", "compliance_reference_ids": [1],
        "identified_by_type": "EMPLOYEE", "identified_by_email": "jane@wso2.com",
        "assigner_id": 3, "risk_identified_date": "2026-01-15",
        "risk_category_ids": [2], "likelihood": 3, "impact": 4,
        "impact_description": "...", "implementation_date": "2026-02-01",
        "reassessment_date": "2026-08-01", "assignment_team_id": 1,
        "owner_id": 5, "management_approver_id": 6, "action_owner_id": 5,
        "action_plan_description": "...", "action_steps": [{"description":"..."}],
        "treatment_strategy": "REMEDIATE", "email_subject": "New risk registered"
      }'

# Compliance approves a risk (moves PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION)
curl -X POST http://localhost:8081/api/v1/risks/1/approve \
  -H "Authorization: Bearer $JWT"

# List audits
curl -H "Authorization: Bearer $JWT" http://localhost:8081/api/v1/audits

# Create an audit
curl -X POST http://localhost:8081/api/v1/audits \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"Q2 SOC2 Audit","frameworkId":1,"productId":2,"periodStart":"2026-04-01","periodEnd":"2026-06-30"}'

# Upload evidence for a risk
curl -X POST http://localhost:8081/api/v1/risks/1/evidence \
  -H "Authorization: Bearer $JWT" \
  -F "evidenceType=ACTION_PLAN_ATTACHMENT" \
  -F "file=@/path/to/document.pdf"
```
