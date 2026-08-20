# Compliance Entity

Go REST service that owns MySQL and Azure Blob Storage access for the GRC
platform. The GRC Platform Backend (Risk Hub + Audit Hub) no longer connects to
MySQL directly — it calls this service over HTTP for every read and write.

## Quick Start

```bash
# from entity/compliance-entity/
go run ./cmd/api
```

Service starts at `http://localhost:8080`.

## Overview

- Default port: `:8080`
- Runtime: Go `1.23+`
- Entry point: `cmd/api/main.go`
- Authentication: handled upstream by the Choreo API Gateway — this service
  applies **no** auth middleware itself. The GRC Backend is a trusted internal
  caller: it validates the Asgardeo JWT, then forwards the actor's email as
  `createdBy`/`updatedBy` in the JSON body of every request. Handler code reads
  actor identity from the decoded request struct, never from request headers.
- Owns two external dependencies the backend cannot reach directly: the MySQL
  `grc_platform` database, and Azure Blob Storage (evidence/risk file bytes).
- Serves both the Risk Hub and Audit Hub domains from one service.

## Prerequisites

- Go `1.23+` — [install](https://go.dev/doc/install)
- MySQL 8+ reachable via `DB_DSN`

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with the race detector
go test -race ./...

# Run a specific package
go test ./internal/service/...

# Check test coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Vet + build
go vet ./...
go build ./...
```

## Configuration

Copy `.env.example` to `.env` and fill in the values:

### Database

| Variable | Description |
|---|---|
| `DB_DSN` | MySQL DSN — `user:password@tcp(host:port)/grc_platform?parseTime=true` (required) |

### Azure Blob Storage

| Variable | Description |
|---|---|
| `AZURE_STORAGE_ACCOUNT_NAME` | Azure Storage account name |
| `AZURE_STORAGE_ACCOUNT_KEY` | Azure Storage account key |
| `AZURE_STORAGE_CONTAINER` | Blob container name (default `grc-evidence`) |

When Azure credentials are absent, the service still starts — the `/files`,
`/evidence-files`, and `/evidence/{id}/files` byte-storage routes are simply
disabled, which is useful for local dev/tests that only exercise MySQL-backed
metadata endpoints.

### Server

| Variable | Description |
|---|---|
| `SERVER_PORT` | Listen port (default `8080`) |

## Project Structure

```text
compliance-entity/
├── cmd/api/main.go                 # Entry point — config load, DB pool, server start, graceful shutdown
├── internal/
│   ├── config/config.go            # Env var loading + Validate()
│   ├── db/db.go                    # MySQL connection pool
│   ├── apierror/apierror.go        # Typed API error with HTTP status
│   ├── cache/                      # In-memory caches (e.g. risk scores, users, audit frameworks)
│   ├── domain/                     # Domain/response types shared across handlers (entity.go, privilege.go, dashboard.go, ...)
│   ├── middleware/
│   │   ├── correlationid.go        # X-Correlation-ID propagation
│   │   ├── recovery.go             # Panic recovery → 500
│   │   ├── logger.go               # Per-request structured logging
│   │   ├── timeout.go              # Request timeout (30s)
│   │   └── usertoken.go            # Captures x-user-id-token header (forwarded, not trusted for identity)
│   ├── repository/                 # SQL queries, one file per resource (risk_repo.go, audit_repo.go, user_repo.go, ...)
│   ├── service/                    # Business logic — validation, workflow-transition guards, orchestration
│   ├── handler/                    # HTTP handlers, one file per resource
│   ├── server/
│   │   ├── routes.go               # Builds the full repository→service→handler graph and registers every route
│   │   └── server.go               # http.Server with production timeouts, wraps NewRouter()
│   └── storage/                    # Azure Blob Storage wrapper
```

**Request flow through the layers:**
```text
HTTP request (from the GRC Backend)
    → middleware (CorrelationID → Recovery → Logger → Timeout)
    → handler    (decode request, call service, write response)
    → service    (business rules, status-transition guards, validation)
    → repository (SQL queries, no business logic)
    → MySQL / Azure Blob Storage
```

## API Endpoints

All paths below are relative to the service root (no `/api/v1` prefix — that
prefix is added by the GRC Backend on its own external-facing routes, not
here).

### Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness check |

### Users & Privileges

| Method | Path | Description |
|---|---|---|
| `POST` | `/users` | Create user |
| `POST` | `/users/search` | Search users |
| `GET` | `/users/{id}` | Get user by ID |
| `GET` | `/users/by-email/{email}` | Get user by email |
| `PATCH` | `/users/{id}` | Update user |
| `GET` | `/role-privileges` | Role → privilege map (backend's privilege store loads from here) |

### Risk Hub — Teams, Scores, References

| Method | Path | Description |
|---|---|---|
| `POST` | `/risk/teams` | Create risk team |
| `POST` | `/risk/teams/search` | Search risk teams |
| `GET` | `/risk/teams/{id}` | Get risk team |
| `PATCH` | `/risk/teams/{id}` | Update risk team |
| `GET` | `/risk/scores` | List risk scores |
| `POST` | `/risk/compliance-references` | Create compliance reference |
| `POST` | `/risk/compliance-references/search` | Search compliance references |
| `GET` | `/risk/compliance-references/{id}` | Get compliance reference |
| `PATCH` | `/risk/compliance-references/{id}` | Update compliance reference |

### Risk Hub — Risks & Workflow

| Method | Path | Description |
|---|---|---|
| `POST` | `/risks` | Create risk |
| `POST` | `/risks/search` | Search/list risks |
| `GET` | `/risks/{id}` | Get risk |
| `GET` | `/risks/{id}/detail` | Get full risk detail (embeds action plan, scores, etc.) |
| `PATCH` | `/risks/{id}` | Update risk (workflow-status transitions included) |
| `GET` | `/risks/next-sequence-number` | Preview next risk code sequence number |
| `GET` | `/risks/{riskId}/compliance-references` | List a risk's compliance references |
| `POST` | `/risks/{riskId}/compliance-references` | Attach a compliance reference |
| `DELETE` | `/risks/{riskId}/compliance-references/{referenceId}` | Detach a compliance reference |

### Risk Hub — Action Plans, Escalations, Assessments, Evidence, Change Log, Notifications

| Method | Path | Description |
|---|---|---|
| `POST` | `/risks/{riskId}/action-plans` | Create action plan (`plan_type` STANDARD or MANAGEMENT) |
| `GET` | `/risks/{riskId}/action-plans` | List a risk's action plans |
| `GET` | `/action-plans/{planId}` | Get action plan |
| `PATCH` | `/action-plans/{planId}` | Update action plan |
| `POST` | `/action-plans/{planId}/steps` | Add action step |
| `GET` | `/action-plans/{planId}/steps` | List action steps |
| `GET` | `/action-plans/{planId}/steps/{stepId}` | Get action step |
| `PATCH` | `/action-plans/{planId}/steps/{stepId}` | Update action step |
| `DELETE` | `/action-plans/{planId}/steps/{stepId}` | Delete action step |
| `POST` | `/action-plans/{planId}/complete` | Complete a plan (all steps must already be COMPLETED); for a MANAGEMENT plan also resolves its escalation and reverts the risk to IN_REMEDIATION |
| `POST` | `/risks/{riskId}/escalate` | Manually trigger escalation of an overdue IN_REMEDIATION risk (alternative to waiting for the daily job) |
| `POST` | `/risks/{riskId}/escalations` | Create escalation (created automatically by the daily overdue-risk job — see `internal/job` — not by a user) |
| `GET` | `/risks/{riskId}/escalations` | List escalations |
| `GET` | `/risks/{riskId}/escalations/{escalationId}` | Get escalation |
| `PATCH` | `/risks/{riskId}/escalations/{escalationId}` | Update escalation |
| `POST` | `/risks/{riskId}/assessments` | Create reassessment |
| `GET` | `/risks/{riskId}/assessments` | List reassessment history |
| `POST` | `/risks/{riskId}/evidence` | Add risk evidence record |
| `GET` | `/risks/{riskId}/evidence` | List risk evidence |
| `DELETE` | `/risk-evidence/{fileId}` | Delete risk evidence file |
| `POST` | `/risks/{riskId}/changes` | Write a change-log entry |
| `GET` | `/risks/{riskId}/changes` | List a risk's change log |
| `POST` | `/notifications` | Create a notification |
| `GET` | `/notifications?recipientId=` | List a recipient's notifications |
| `PATCH` | `/notifications/{id}/read` | Mark a notification read |

### Risk Hub — Analytics & Dashboard

| Method | Path | Description |
|---|---|---|
| `POST` | `/risk/analytics/search` | Risk analytics summary |
| `POST` | `/risk/dashboard/search` | Risk dashboard summary |

### Audit Hub — Teams, Frameworks, Products

| Method | Path | Description |
|---|---|---|
| `POST` | `/audit/teams` | Create audit team |
| `POST` | `/audit/teams/search` | Search audit teams |
| `GET` | `/audit/teams/{id}` | Get audit team |
| `PATCH` | `/audit/teams/{id}` | Update audit team |
| `POST` | `/audit/frameworks` | Create framework |
| `POST` | `/audit/frameworks/search` | Search frameworks |
| `GET` | `/audit/frameworks/{id}` | Get framework |
| `PATCH` | `/audit/frameworks/{id}` | Update framework |
| `POST` | `/audit/frameworks/{id}/controls` | Create a framework control |
| `GET` | `/audit/frameworks/{id}/controls` | List current controls |
| `GET` | `/audit/frameworks/{id}/controls/{controlNumber}/versions` | List all versions of a control |
| `PUT` | `/audit/frameworks/{id}/controls/{controlId}` | Create a new control version |
| `POST` | `/audit/products` | Create product |
| `POST` | `/audit/products/search` | Search products |
| `GET` | `/audit/products/{id}` | Get product |
| `PATCH` | `/audit/products/{id}` | Update product |

### Audit Hub — Audits, Controls, Population, Evidence

| Method | Path | Description |
|---|---|---|
| `POST` | `/audits` | Create audit |
| `POST` | `/audits/search` | Search audits |
| `GET` | `/audits/{id}` | Get audit |
| `PATCH` | `/audits/{id}` | Update audit |
| `DELETE` | `/audits/{id}` | Delete audit |
| `POST` | `/audits/{auditId}/controls` | Add control |
| `POST` | `/audits/{auditId}/controls/bulk` | Bulk-add controls |
| `POST` | `/audits/{auditId}/controls/search` | Search an audit's controls |
| `GET` | `/audits/{auditId}/controls/{controlId}` | Get control |
| `PATCH` | `/audits/{auditId}/controls/{controlId}` | Update control |
| `DELETE` | `/audits/{auditId}/controls/{controlId}` | Delete control |
| `POST` | `/controls/search` | Search controls globally |
| `GET` | `/controls/assigned-for-evidence` | List controls assigned to the caller for evidence |
| `GET` | `/audit-controls/{controlId}/evidence-assignment` | Get a control's evidence assignment |
| `GET` | `/audit-controls/{controlId}/active-population` | Get a control's active population |
| `POST` | `/audits/{auditId}/controls/{controlId}/populations` | Create population |
| `GET` | `/audits/{auditId}/controls/{controlId}/populations` | List populations |
| `GET` | `/populations/{populationId}` | Get population |
| `PATCH` | `/populations/{populationId}` | Update population |
| `POST` | `/populations/{populationId}/files` | Add population file |
| `GET` | `/populations/{populationId}/files` | List population files |
| `GET` | `/population-files/{fileId}` | Get population file |
| `DELETE` | `/population-files/{fileId}` | Delete population file |
| `POST` | `/audits/{auditId}/controls/{controlId}/evidence` | Create evidence |
| `GET` | `/audits/{auditId}/controls/{controlId}/evidence` | List evidence for a control |
| `GET` | `/evidence/{evidenceId}` | Get evidence |
| `PATCH` | `/evidence/{evidenceId}` | Update evidence |
| `POST` | `/evidence/{evidenceId}/files` | Add evidence file |
| `GET` | `/evidence/{evidenceId}/files` | List evidence files |
| `GET` | `/evidence-files/{fileId}` | Get evidence file |
| `DELETE` | `/evidence-files/{fileId}` | Delete evidence file |
| `POST` | `/evidence/{evidenceId}/ai-validations` | Create AI validation |
| `GET` | `/evidence/{evidenceId}/ai-validations` | List AI validations |

### Audit Hub — Comments, Trail, Dashboard

| Method | Path | Description |
|---|---|---|
| `POST` | `/audits/{auditId}/controls/{controlId}/comments` | Add comment |
| `GET` | `/audits/{auditId}/controls/{controlId}/comments` | List comments |
| `DELETE` | `/comments/{commentId}` | Delete comment |
| `POST` | `/audits/{auditId}/trail` | Write trail entry |
| `GET` | `/audits/{auditId}/trail` | List trail |
| `POST` | `/audit/dashboard/search` | Audit dashboard summary |
| `POST` | `/audit/work-queue/search` | Work queue page |
| `POST` | `/audit/notifications` | Log a sent audit-module email |
| `POST` | `/audit/notifications/claim` | Claim a notification for the reminder job (de-dup) |
| `DELETE` | `/audit/notifications/{id}/claim` | Release a claimed notification |

### Files (Azure Blob byte storage — disabled when Azure is not configured)

| Method | Path | Description |
|---|---|---|
| `POST` | `/files` | Upload file |
| `GET` | `/files` | Download file |
| `GET` | `/files/list` | List files |
| `DELETE` | `/files` | Delete file |
| `GET` | `/evidence-files/{fileId}/content` | Get evidence file content |

## Run Locally

**Start the service:**
```bash
go run ./cmd/api
```

### Examples

```bash
# Health check
curl http://localhost:8080/health

# Get a user by email
curl http://localhost:8080/users/by-email/alice@wso2.com

# Search risks
curl -X POST http://localhost:8080/risks/search \
  -H "Content-Type: application/json" \
  -d '{"statuses":["IN_REMEDIATION"]}'

# Get full risk detail
curl http://localhost:8080/risks/1/detail

# Create an escalation (system-driven — no escalatedTo/reason, see risk_escalation schema)
curl -X POST http://localhost:8080/risks/1/escalations \
  -H "Content-Type: application/json" \
  -d '{"createdBy":"system"}'

# Create an audit
curl -X POST http://localhost:8080/audits \
  -H "Content-Type: application/json" \
  -d '{"title":"Q2 SOC2 Audit","frameworkId":1,"productId":2,"assignedLeadId":5,"createdBy":"alice@wso2.com"}'
```

Note: unlike the GRC Backend, this service does **not** validate a bearer
token itself — requests here are expected to originate from the Choreo
gateway or the trusted GRC Backend, not directly from a browser.
