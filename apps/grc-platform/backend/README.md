# GRC Platform Backend

Go backend service for the Governance, Risk & Compliance (GRC) platform. The platform has two modules, **Risk Hub** and **Audit Hub**.

## Quick Start

```bash
# from backend/
set -a && source .env && set +a && go run ./cmd/server
```

Backend starts at `http://localhost:8080`.

## Overview

- Default port: `:8080`
- Runtime: Go `1.23+`
- Entry point: `cmd/server/main.go`
- Authentication: Asgardeo JWT Bearer token — validated via JWKS endpoint; pass as `Authorization: Bearer <token>` header
- Two modules: **Risk Hub** (`/api/v1/`) and **Audit Hub** (`/api/v1/audit/`)

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

To skip the hook in exceptional cases:

```bash
git push --no-verify
```

## Configuration

Copy `.env` and fill in the values:

### Database

| Variable | Description |
|---|---|
| `DB_DSN` | MySQL DSN — `user:password@tcp(host:port)/grc_platform?parseTime=true` |

### Asgardeo JWT

| Variable | Description |
|---|---|
| `AUTH_JWKS_ENDPOINT` | Asgardeo JWKS URL |
| `AUTH_ISSUER` | Expected `iss` claim |
| `AUTH_AUDIENCE` | Expected `aud` claim |
| `AUTH_TOKEN_VALIDATOR_ENABLED` | Set to `false` to skip signature verification locally (default `true`) |

### Azure Blob Storage

| Variable | Description |
|---|---|
| `AZURE_STORAGE_ACCOUNT_NAME` | Azure Storage account name |
| `AZURE_STORAGE_ACCOUNT_KEY` | Azure Storage account key |
| `AZURE_STORAGE_CONTAINER` | Blob container name for evidence files |

### Server

| Variable | Description |
|---|---|
| `PORT` | Listen address (default `:8080`) |

## Project Structure

```text
backend/
├── cmd/server/main.go              # Entry point — middleware chain + route registration
├── internal/
│   ├── config/config.go            # Env var loading (mustEnv)
│   ├── db/db.go                    # MySQL connection pool
│   ├── apierror/apierror.go        # Typed API error with HTTP status
│   ├── response/response.go        # JSON write helpers
│   ├── middleware/
│   │   ├── auth.go                 # Asgardeo JWT validation, UserInfo → context
│   │   ├── correlation.go          # X-Correlation-ID generation + slog injection
│   │   └── logger.go               # Per-request structured logging
│   ├── shared/
│   │   ├── auth/auth.go            # HasPrivilege / RequirePrivilege helpers (no role constants)
│   │   ├── privilege/privilege.go  # Privilege name constants + Store (DB-loaded role→privilege map)
│   │   └── file/file.go            # Azure Blob Storage wrapper (TODO)
│   ├── user/                       # Shared user entity (both modules reference it)
│   │   ├── model.go
│   │   ├── repository.go
│   │   └── mysql/repository.go
│   ├── risk/                       # Risk Hub
│   │   ├── model/                  # Domain types and request/response structs
│   │   ├── repository/             # Interfaces (repository.go) + MySQL stubs (mysql/)
│   │   ├── service/                # Business logic — workflow rules, validations
│   │   └── handler/                # HTTP handlers + route registration (routes.go)
│   └── audit/                      # Audit Hub
│       ├── model/
│       ├── repository/
│       ├── service/
│       └── handler/
└── tests/integration/              # Integration test stubs
```

**Request flow through the layers:**
```
HTTP request
    → middleware (CorrelationID → Auth → Logger)
    → handler   (parse request, call service, write response)
    → service   (business rules, status transition guards, changelog/trail writes)
    → repository (SQL queries, no business logic)
    → MySQL
```

## API Endpoints

### Risk Hub

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users` | List users |
| `GET` | `/api/v1/users/me` | Current user profile |
| `GET` | `/api/v1/teams` | List risk teams |
| `POST` | `/api/v1/teams` | Create team |
| `PUT` | `/api/v1/teams/{id}` | Update team |
| `GET` | `/api/v1/risk-scores` | List risk scores |
| `POST` | `/api/v1/risk-scores` | Create risk score |
| `PUT` | `/api/v1/risk-scores/{id}` | Update risk score |
| `GET` | `/api/v1/risks` | List risks |
| `POST` | `/api/v1/risks` | Register a risk |
| `GET` | `/api/v1/risks/{id}` | Get risk by ID |
| `PUT` | `/api/v1/risks/{id}` | Update risk |
| `POST` | `/api/v1/risks/{id}/submit` | Submit for compliance review |
| `POST` | `/api/v1/risks/{id}/approve` | Compliance approves |
| `POST` | `/api/v1/risks/{id}/reject` | Compliance rejects |
| `POST` | `/api/v1/risks/{id}/complete` | Complete remediation |
| `POST` | `/api/v1/risks/{id}/owner-approve` | Risk owner approves closure |
| `POST` | `/api/v1/risks/{id}/close` | Compliance closes |
| `POST` | `/api/v1/risks/{id}/escalate` | Escalate to management |
| `POST` | `/api/v1/risks/{id}/assess` | Management assessment |
| `GET` | `/api/v1/risks/{id}/changelog` | Risk change history |
| `GET` | `/api/v1/risks/{id}/action-plans` | List action plans |
| `POST` | `/api/v1/risks/{id}/action-plans` | Create action plan |
| `GET` | `/api/v1/risks/{id}/action-plans/{planId}` | Get action plan |
| `PUT` | `/api/v1/risks/{id}/action-plans/{planId}` | Update action plan |
| `GET` | `/api/v1/risks/{id}/action-plans/{planId}/steps` | List steps |
| `POST` | `/api/v1/risks/{id}/action-plans/{planId}/steps` | Add step |
| `PUT` | `/api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}` | Update step |
| `GET` | `/api/v1/risks/{id}/evidence` | List evidence |
| `POST` | `/api/v1/risks/{id}/evidence` | Upload evidence |
| `DELETE` | `/api/v1/risks/{id}/evidence/{evidenceId}` | Delete evidence |
| `GET` | `/api/v1/risks/{id}/escalations` | Escalation history |
| `GET` | `/api/v1/notifications` | List notifications |
| `PATCH` | `/api/v1/notifications/{id}/read` | Mark notification read |
| `GET` | `/api/v1/compliance-references` | List compliance references |
| `POST` | `/api/v1/compliance-references` | Create compliance reference |
| `GET` | `/api/v1/risks/analytics/summary` | Risk analytics summary |

### Audit Hub

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/audit/frameworks` | List audit frameworks |
| `POST` | `/api/v1/audit/frameworks` | Create framework |
| `GET` | `/api/v1/audit/products` | List products |
| `POST` | `/api/v1/audit/products` | Create product |
| `GET` | `/api/v1/audits` | List audits |
| `POST` | `/api/v1/audits` | Create audit |
| `GET` | `/api/v1/audits/{id}` | Get audit by ID |
| `PUT` | `/api/v1/audits/{id}` | Update audit |
| `POST` | `/api/v1/audits/{id}/fieldwork` | Move to fieldwork |
| `POST` | `/api/v1/audits/{id}/review` | Submit for review |
| `POST` | `/api/v1/audits/{id}/complete` | Complete audit |
| `GET` | `/api/v1/audits/{id}/controls` | List controls |
| `POST` | `/api/v1/audits/{id}/controls` | Add control |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}` | Get control |
| `PUT` | `/api/v1/audits/{id}/controls/{controlId}` | Update control |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/population` | List population |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/population` | Upload population |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/population/{populationId}` | Delete population entry |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/evidence` | List evidence |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence` | Upload evidence |
| `DELETE` | `/api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}` | Delete evidence |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/review` | Review evidence |
| `GET` | `/api/v1/audits/{id}/controls/{controlId}/comments` | List comments |
| `POST` | `/api/v1/audits/{id}/controls/{controlId}/comments` | Add comment |
| `GET` | `/api/v1/audits/{id}/assignments` | List assignments |
| `POST` | `/api/v1/audits/{id}/assignments` | Create assignment |
| `DELETE` | `/api/v1/audits/{id}/assignments/{assignmentId}` | Remove assignment |
| `GET` | `/api/v1/audits/{id}/trail` | Audit trail |
| `GET` | `/api/v1/audit/notifications` | List notifications |
| `PATCH` | `/api/v1/audit/notifications/{id}/read` | Mark notification read |

## Run Locally

**Start the server:**
```bash
set -a && source .env && set +a && go run ./cmd/server
```

When `AUTH_TOKEN_VALIDATOR_ENABLED=false`, JWT signature verification is skipped — the token is still decoded so user info is populated. Pass any valid-structure JWT as the Bearer token for local testing.

### Examples

```bash
JWT="<your-jwt-token>"

# Health check
curl http://localhost:8080/health

# Get current user profile
curl -H "Authorization: Bearer $JWT" http://localhost:8080/api/v1/users/me

# List risks
curl -H "Authorization: Bearer $JWT" http://localhost:8080/api/v1/risks

# Register a risk
curl -X POST http://localhost:8080/api/v1/risks \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"title":"Unauthorised data access","category":"SECURITY","likelihood":3,"impact":4}'

# Submit a risk for compliance review
curl -X POST http://localhost:8080/api/v1/risks/1/submit \
  -H "Authorization: Bearer $JWT"

# List audits
curl -H "Authorization: Bearer $JWT" http://localhost:8080/api/v1/audits

# Create an audit
curl -X POST http://localhost:8080/api/v1/audits \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"title":"Q2 SOC2 Audit","frameworkId":1,"productId":2,"assignedLeadId":5}'

# Upload evidence for a risk
curl -X POST http://localhost:8080/api/v1/risks/1/evidence \
  -H "Authorization: Bearer $JWT" \
  -F "file=@/path/to/document.pdf"
```
