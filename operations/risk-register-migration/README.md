# risk-register-migration

One-time migration that loads the historical risk register into the platform
database by driving the **compliance-entity HTTP API**. Deployed as a **Choreo
Manual Task** (Go).

Full design: `RISK_REGISTER_CSV_MIGRATION_PLAN.md` (a planning doc kept outside
this repo). Section references (§n) in the code comments point there.

## What it does

1. **Preflight** (§7) — entity health, SCIM reachable, the three risk roles
   `ACTIVE`, reference data populated, CSV headers present. Any failure aborts
   before a single write.
2. **Parse + map** the CSV (§4, §6) — header-name matched, legend/blank rows
   skipped, values transformed, bad rows → `REJECT` findings.
3. **Resolve people** (§6) — work email → uuid via the SCIM org snapshot →
   `user.id` via `GET /users/by-uuid` (else `POST /users`, never mutating an
   existing row).
4. **Dry run** (default) stops here and prints the report.
5. **Reconstruct resume state** from the entity (§8) — no ledger file.
6. **Write** each row in `Migration ID` order (§5):
   - `POST /risks` (born `PENDING_RISK_OWNER_APPROVAL`, `created_by` = marker)
   - walk `workflow_status` to the row's bucket via `PATCH` (no-op when
     `from == to`)
   - `IN_REMEDIATION` + overdue → `POST /risks/{id}/escalations` to suppress the
     nightly escalation job
   - `IN_REMEDIATION` → ensure the §6 role grants
   - `CLOSED` → also `PATCH` the action plan to `COMPLETED`

## Buckets

Every row's added `Workflow Status` column is exactly `IN_REMEDIATION` or
`CLOSED`. Anything else rejects the row. There is no brand-new / `DRAFT` /
`CANCELLED` bucket.

## Run it

Local (against reachable staging URLs) — copy `.env.example` to `.env`, fill in
the values, then:

```bash
set -a && source .env && set +a

go run . -input ./risks.csv -migration-date 2026-09-15          # dry run

# ... fix everything the report REJECTs, re-export, repeat ...

go run . -input ./risks.csv -migration-date 2026-09-15 -dry-run=false
```

`-input`, `-migration-date` and `-dry-run` stay explicit flags on every
invocation rather than living in `.env` — which CSV, which date, and whether
this run writes should never be a leftover value from an old file. `go run .
-h` prints the full flag surface; every `.choreo/component.yaml` config key
maps to a `Config` field in `main.go`, and `.env`/`.env.example` use the same
names.

In Choreo: set the config keys in `.choreo/component.yaml`, upload the CSV as
the `INPUT_PATH` file mount, leave `DRY_RUN=true`, trigger, read the logs; then
flip `DRY_RUN=false` and trigger again.

Exit codes: `0` clean · `2` completed with findings · `1` structural abort.

## Fully local end-to-end

Two ways to resolve identity locally, against the same local entity + local
MySQL either way — pick one:

- **`-scim-snapshot <file>`** — an `email,uuid` CSV read instead of calling
  Asgardeo. No Asgardeo credentials needed at all. **Local testing only —
  never set `SCIM_SNAPSHOT_FILE` in Choreo**, and pass it as a flag, not a
  `.env` var, so it can't leak into a staging/Choreo run by accident. When
  it's set, the `SCIM_INTERNAL_*` config becomes optional.
- **A real Asgardeo org** (your own personal/dev org is fine) — set
  `SCIM_BASE_URL` / `SCIM_INTERNAL_ORG` / `SCIM_INTERNAL_CLIENT_ID` /
  `SCIM_INTERNAL_CLIENT_SECRET` / `SCIM_INTERNAL_SCOPES`
  (`internal_user_mgt_view internal_user_mgt_list` — only those two) for real,
  set `SCIM_DOMAIN` to the **email suffix your org's users actually carry**
  (not the org name — they're different fields: `SCIM_INTERNAL_ORG` is the
  Asgardeo tenant path segment that goes in the URL), and drop
  `-scim-snapshot`. This exercises the real OAuth2 client-credentials +
  SCIM2 Users API path, not the fake.

Either way, the CSV's four person columns need emails that actually resolve —
either present in the snapshot file, or real users in the org/domain above.

```bash
# 1. local MySQL. shared.sql/risk_schema.sql both open with USE grc_platform
# and no CREATE DATABASE, so the database has to exist first.
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS grc_platform"
mysql -uroot -p grc_platform < ../../apps/grc-platform/backend/Resources/shared.sql
mysql -uroot -p grc_platform < ../../apps/grc-platform/backend/Resources/risk_schema.sql
# risk roles. Must be this file, not the older root staging_shared_seed_data.sql:
# that one predates the management-role split and still carries the pre-split
# name grc-platform-management, so preflight aborts on the missing
# grc-platform-risk-management. Re-running is safe (ON DUPLICATE KEY UPDATE on
# uq_role_name, and the renames are no-ops once applied), and it fixes a DB
# seeded from the old file by renaming the role in place, keeping its role_id.
mysql -uroot -p grc_platform < ../../apps/grc-platform/backend/Resources/shared_seed_data.sql
# teams / categories / compliance refs / scores. NOT risk_module_data_schema.sql
# — that file seeds neither risk_team nor risk_score, and buildRefData aborts
# preflight when any of the four is empty.
mysql -uroot -p grc_platform < <path-to>/staging_risk_seed_data.sql

# Run the seed files from the mysql CLI, not MySQL Workbench: Workbench's safe
# update mode rejects shared_seed_data.sql's `WHERE role_name COLLATE
# utf8mb4_bin IN (...)` with error 1175 (the COLLATE hides the uq_role_name
# index from it). `SET SQL_SAFE_UPDATES = 0;` in the same session also works.

# 2. compliance-entity (separate shell, leave running).
# Back up any .env you already have — this overwrites it.
cd ../../entity/compliance-entity
[ -f .env ] && cp .env .env.bak
# &tls=false is required: internal/db/mysql.go defaults a DSN with no tls= to
# verified TLS and will not fall back to plaintext, so a local MySQL without
# TLS just fails to connect.
printf 'DB_DSN=root:<password>@tcp(127.0.0.1:3306)/grc_platform?parseTime=true&tls=false\nSERVER_PORT=8080\n' > .env
go run ./cmd/api

# 3. confirm the entity is up before every run in this tool — cheap, and it's
# the fastest way to tell "the entity isn't running" apart from a real config
# problem when preflight fails.
curl -s localhost:8080/health
curl -s localhost:8080/risk/scores | head -c 200

# 4. this tool. Copy .env.example to .env, fill in COMPLIANCE_ENTITY_BASE_URL
# plus whichever identity path you picked above.
cd ../../operations/risk-register-migration
set -a && source .env && set +a
IN=<path-to-your-prepared-register>.csv

go run . -input "$IN" -migration-date 2026-09-15          # dry run
echo "exit=$?"

# ... fix everything the report REJECTs, re-run the dry run, repeat ...

go run . -input "$IN" -migration-date 2026-09-15 -dry-run=false   # real run
echo "exit=$?"
```

Verify:

```sql
SELECT risk_code, workflow_status, treatment_strategy
FROM risk WHERE created_by='risk-sheet-migration' ORDER BY risk_code;

SELECT COUNT(*) FROM `user`         WHERE created_by='risk-sheet-migration';
SELECT COUNT(*) FROM risk_escalation  WHERE created_by='risk-sheet-migration';
SELECT COUNT(*) FROM user_role_grant  WHERE created_by='risk-sheet-migration';
SELECT COUNT(*) FROM risk_action_plan WHERE created_by='risk-sheet-migration' AND status='COMPLETED';
```

Then re-run the exact same real-run command once more. Every row must come
back **Skipped** (`migrated=0 skipped(resume)=N`), and none of the counts
above may change — this is the resume path, and the property a retriggered
Choreo Manual Task actually depends on. Reset with `rollback.sql` between
attempts (it deliberately leaves `user` rows alone — see its header comment;
delete those separately if you're switching identity source and want a fully
clean slate).

A `testdata/risks.csv`-shaped register (5 rows: 3 `IN_REMEDIATION`, 2
`CLOSED`, one `ACCEPT`/high row that also gets the management grant) is a good
size for a first local pass before trying the real register export.

## Report

The run prints two blocks to stdout (captured by Choreo):

- **`errors.csv`** — one row per finding: `migration_id, csv_row, risk_title,
  severity (REJECT|WARN), failure, detail`.
- **`report.txt`** — findings-by-code counts, the distinct unresolved people,
  the Migration IDs that got a suppressing escalation, the grant count, and
  (real run) per-bucket migrated counts.

## Rollback

`rollback.sql` — by-marker `DELETE` in FK order, run by hand against the target
DB. The tool never deletes.

## Status

Implemented (T1–T9, T11): HTTP + SCIM clients, preflight + reference data, CSV
value mapping, identity resolution, resume-state reconstruction, the write
pipeline, the report, and a table-driven test suite including an end-to-end
pass over `testdata/risks.csv` (built from the real "Risk Form Structure" tab)
against a stateful in-memory fake of the entity — including a second run that
must be a clean no-op. `go test ./...` is green (~80% statement coverage; the
gap is `main`/`run`/`loadConfig` CLI bootstrap).

Not yet done: `.choreo/component.yaml` confirmation as a real Manual Task in the
Choreo console (T10) and the staging/production dry-run → real-run passes (T12).
