// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Command risk-register-import loads the historical risk register into the
// platform database exactly once, by driving the compliance-entity HTTP API.
//
// It is deployed as a Choreo Manual Task and run twice per environment: first
// with dryRun=true (validate + resolve + report, write nothing), then with
// dryRun=false to write. See RISK_REGISTER_CSV_MIGRATION_PLAN.md at the repo
// root for the full design; the section numbers in the comments below refer to
// it.
//
// # WHY THE ENTITY API, NOT SQL
//
// POST /risks on compliance-entity owns the per-register sequence lock, the
// risk_code format, gross-score resolution, the mandatory STANDARD action plan,
// and the CREATE change-log row, all in one transaction. The entity sends no
// email (only the grc-platform backend does), so writing through it is
// email-free by construction.
//
// # RESUME
//
// A Manual Task has no durable filesystem, so there is no ledger. Every run
// reconstructs progress from the entity (see state.go): risk exists ->
// workflow_status reached -> suppressing escalation present -> grants written ->
// action plan COMPLETED. Re-running skips finished rows and completes partial
// ones (status PATCHes are no-ops when from == to).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// marker is written to created_by / updatedBy / createdBy on every row and
// child row this tool creates. It is the only handle for resume (state.go) and
// rollback (rollback.sql).
const marker = "risk-sheet-migration"

// Exit codes (plan §9).
const (
	exitOK         = 0 // clean: every row migrated, no findings
	exitStructural = 1 // preflight / fatal abort before or during, nothing partial left intentionally
	exitFindings   = 2 // completed, but with REJECT / WARN findings
)

// Config is assembled from flags first, then environment variables for anything
// still unset — so Choreo config (env) and a local `go run` (flags) both work.
type Config struct {
	InputPath     string // mounted CSV path (Choreo config file mount)
	DryRun        bool   // defaults true; must be explicitly disabled to write
	MigrationDate string // YYYY-MM-DD; the "we don't actually know when" date for closure fields (§5)
	SCIMDomain    string // e.g. "wso2.com"

	// SCIMSnapshotFile, when set, replaces the live Asgardeo SCIM call with an
	// email→uuid CSV read from this path. Only for local testing — Choreo runs
	// leave it empty and hit the real directory. When set, the SCIM_INTERNAL_*
	// config becomes optional.
	SCIMSnapshotFile string

	ComplianceEntityBaseURL string

	SCIMBaseURL      string
	SCIMTokenURL     string // derived {base}/t/{org}/oauth2/token unless SCIM_TOKEN_URL overrides
	SCIMOrg          string // SCIM_INTERNAL_ORG — tenant path segment, e.g. "wso2"
	SCIMClientID     string
	SCIMClientSecret string
	SCIMScopes       string

	HTTPTimeout time.Duration
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "risk-register-import: %v\n", err)
		os.Exit(exitStructural)
	}
	os.Exit(run(context.Background(), cfg))
}

// loadConfig reads flags, then fills gaps from the environment.
func loadConfig() (Config, error) {
	var c Config
	flag.StringVar(&c.InputPath, "input", envOr("INPUT_PATH", ""), "path to the prepared CSV export")
	dryRunDefault, err := parseBoolEnv("DRY_RUN", true)
	if err != nil {
		return Config{}, err
	}
	flag.BoolVar(&c.DryRun, "dry-run", dryRunDefault, "validate and report only; do not write")
	flag.StringVar(&c.MigrationDate, "migration-date", envOr("MIGRATION_DATE", time.Now().UTC().Format("2006-01-02")),
		"YYYY-MM-DD used for completed_date / compliance_approval_date on CLOSED rows")
	flag.StringVar(&c.SCIMDomain, "scim-domain", envOr("SCIM_DOMAIN", "wso2.com"), "email-domain suffix to snapshot from SCIM")
	flag.StringVar(&c.SCIMSnapshotFile, "scim-snapshot", envOr("SCIM_SNAPSHOT_FILE", ""),
		"local testing only: path to an email,uuid CSV used instead of calling Asgardeo SCIM")
	timeout := flag.Duration("http-timeout", envDuration("HTTP_TIMEOUT", 90*time.Second), "per-request HTTP timeout")
	flag.Parse()

	c.HTTPTimeout = *timeout
	c.SCIMSnapshotFile = strings.TrimSpace(c.SCIMSnapshotFile)
	c.ComplianceEntityBaseURL = os.Getenv("COMPLIANCE_ENTITY_BASE_URL")
	c.SCIMBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("SCIM_BASE_URL")), "/")
	c.SCIMOrg = strings.TrimSpace(os.Getenv("SCIM_INTERNAL_ORG"))
	c.SCIMTokenURL = strings.TrimSpace(os.Getenv("SCIM_TOKEN_URL"))
	c.SCIMClientID = os.Getenv("SCIM_INTERNAL_CLIENT_ID")
	c.SCIMClientSecret = os.Getenv("SCIM_INTERNAL_CLIENT_SECRET")
	c.SCIMScopes = os.Getenv("SCIM_INTERNAL_SCOPES")

	// The real SCIM path is Asgardeo-direct: token at {base}/t/{org}/oauth2/token,
	// search at {base}/t/{org}/scim2/Users/.search (internal/scim.Client,
	// cmd/backfill-uuids). Derive the token URL from base + org the same way
	// config.SCIMTokenURL does, unless SCIM_TOKEN_URL is set as an override.
	if c.SCIMTokenURL == "" && c.SCIMBaseURL != "" && c.SCIMOrg != "" {
		c.SCIMTokenURL = c.SCIMBaseURL + "/t/" + c.SCIMOrg + "/oauth2/token"
	}

	var missing []string
	if c.InputPath == "" {
		missing = append(missing, "-input / INPUT_PATH")
	}
	if c.ComplianceEntityBaseURL == "" {
		missing = append(missing, "COMPLIANCE_ENTITY_BASE_URL")
	}
	// The SCIM_INTERNAL_* config is only needed for the live directory call; a
	// local -scim-snapshot run reads identity from a file instead.
	if c.SCIMSnapshotFile == "" {
		if c.SCIMBaseURL == "" {
			missing = append(missing, "SCIM_BASE_URL")
		}
		if c.SCIMOrg == "" {
			missing = append(missing, "SCIM_INTERNAL_ORG")
		}
		if c.SCIMClientID == "" {
			missing = append(missing, "SCIM_INTERNAL_CLIENT_ID")
		}
		if c.SCIMClientSecret == "" {
			missing = append(missing, "SCIM_INTERNAL_CLIENT_SECRET")
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if _, err := time.Parse("2006-01-02", c.MigrationDate); err != nil {
		return Config{}, fmt.Errorf("-migration-date %q is not YYYY-MM-DD", c.MigrationDate)
	}
	return c, nil
}

// run is the whole pipeline. It returns the process exit code.
func run(ctx context.Context, cfg Config) int {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("risk-register-import starting", "dryRun", cfg.DryRun, "migrationDate", cfg.MigrationDate)

	rep := NewReport()

	ec := NewEntityClient(cfg.ComplianceEntityBaseURL, cfg.HTTPTimeout)
	sc := NewSCIMClient(cfg.SCIMBaseURL, cfg.SCIMTokenURL, cfg.SCIMClientID, cfg.SCIMClientSecret, cfg.SCIMScopes, cfg.SCIMOrg, cfg.HTTPTimeout)

	// ── Preflight (§7) — any failure aborts before a single write ─────────────
	refs, dirUsers, err := preflight(ctx, cfg, ec, sc)
	if err != nil {
		log.Error("preflight failed — aborting, nothing written", "err", err)
		return exitStructural
	}

	// ── Parse + validate + map the CSV (§4, §6) ──────────────────────────────
	f, err := os.Open(cfg.InputPath)
	if err != nil {
		log.Error("cannot open input CSV", "path", cfg.InputPath, "err", err)
		return exitStructural
	}
	defer f.Close()

	rows, parseFindings, err := parseSheet(f, refs)
	if err != nil {
		log.Error("CSV is structurally unusable — aborting", "err", err)
		return exitStructural
	}
	rep.Add(parseFindings...)

	// ── Resolve people: email → uuid → user.id (§6 Identity resolution) ───────
	// Reuses the SCIM snapshot preflight already fetched — the directory is
	// 300k+ records, not worth pulling twice. In a dry run the resolver only
	// checks that each email maps to exactly one uuid; it provisions user rows
	// (POST /users) only on a real run.
	res := NewResolver(ec, dirUsers, cfg.DryRun)
	res.Build()
	resolveFindings, err := res.Apply(ctx, rows)
	if err != nil {
		log.Error("identity resolution failed — aborting; re-run to resume", "err", err)
		return exitStructural
	}
	rep.Add(resolveFindings...)

	// Rows that picked up a REJECT finding do not proceed.
	pending := selectMigratable(rows, rep)

	if cfg.DryRun {
		log.Info("dry run — no writes", "migratable", len(pending), "rejected", len(rows)-len(pending))
		rep.Emit(os.Stdout)
		if rep.HasFindings() {
			return exitFindings
		}
		return exitOK
	}

	// ── Reconstruct resume state from the entity (§8) ────────────────────────
	progress, err := reconstructState(ctx, ec, refs, cfg.MigrationDate, pending, rep)
	if err != nil {
		log.Error("could not reconstruct resume state — aborting", "err", err)
		return exitStructural
	}

	// reconstructState may reject rows that collide on the natural key.
	rejected := rep.RejectedMigrationIDs()

	// ── Write, in Migration ID order (§9) ───────────────────────────────────
	for _, row := range pending {
		if _, bad := rejected[row.MigrationID]; bad {
			continue
		}
		if err := migrateRow(ctx, ec, cfg, refs, row, progress[row.MigrationID], rep); err != nil {
			// Auth / network class errors are fatal; row-level problems are
			// recorded inside migrateRow and do not reach here.
			log.Error("fatal error during write — stopping; re-run to resume", "migrationId", row.MigrationID, "err", err)
			rep.Emit(os.Stdout)
			return exitStructural
		}
	}

	rep.Emit(os.Stdout)
	if rep.HasFindings() {
		return exitFindings
	}
	return exitOK
}

// preflight runs the structural checks in plan §7 in order — the first failure
// returns an error and the run aborts before any write — and returns the
// reference-data lookups every row needs (team codes, category names,
// compliance-ref names, score cells, role ids).
//
// The AVOID-enum precondition (§7) is not checked here: it is a per-environment
// checklist tick, with a write-time backstop in migrateRow (T7).
//
// It also returns the SCIM org snapshot it fetched for the "> 0" check, so the
// resolver can reuse it rather than pull the whole directory again.
func preflight(ctx context.Context, cfg Config, ec *EntityClient, sc *SCIMClient) (RefData, []DirectoryUser, error) {
	if err := ec.Health(ctx); err != nil {
		return RefData{}, nil, fmt.Errorf("compliance-entity health check: %w", err)
	}

	var (
		users []DirectoryUser
		err   error
	)
	if cfg.SCIMSnapshotFile != "" {
		users, err = loadSnapshotFile(cfg.SCIMSnapshotFile)
		if err != nil {
			return RefData{}, nil, fmt.Errorf("read -scim-snapshot %q: %w", cfg.SCIMSnapshotFile, err)
		}
		if len(users) == 0 {
			return RefData{}, nil, fmt.Errorf("-scim-snapshot %q has no email,uuid rows", cfg.SCIMSnapshotFile)
		}
	} else {
		users, err = sc.ListUsersByDomain(ctx, cfg.SCIMDomain)
		if err != nil {
			return RefData{}, nil, fmt.Errorf("SCIM snapshot for %q: %w", cfg.SCIMDomain, err)
		}
		if len(users) == 0 {
			return RefData{}, nil, fmt.Errorf("SCIM snapshot for %q is empty — check SCIM_INTERNAL_* credentials and scopes", cfg.SCIMDomain)
		}
	}

	roles, err := ec.ListRoles(ctx)
	if err != nil {
		return RefData{}, nil, fmt.Errorf("list roles: %w", err)
	}
	teams, err := ec.ListRiskTeams(ctx)
	if err != nil {
		return RefData{}, nil, fmt.Errorf("list risk teams: %w", err)
	}
	cats, err := ec.ListRiskCategories(ctx)
	if err != nil {
		return RefData{}, nil, fmt.Errorf("list risk categories: %w", err)
	}
	refs, err := ec.ListComplianceRefs(ctx)
	if err != nil {
		return RefData{}, nil, fmt.Errorf("list compliance references: %w", err)
	}
	scores, err := ec.ListRiskScores(ctx)
	if err != nil {
		return RefData{}, nil, fmt.Errorf("list risk scores: %w", err)
	}

	rd, err := buildRefData(teams, cats, refs, scores, roles)
	if err != nil {
		return RefData{}, nil, err
	}
	return rd, users, nil
}

// selectMigratable returns the rows with no REJECT finding, in Migration ID order.
func selectMigratable(rows []Row, rep *Report) []Row {
	rejected := rep.RejectedMigrationIDs()
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if _, bad := rejected[r.MigrationID]; bad {
			continue
		}
		out = append(out, r)
	}
	sortByMigrationID(out)
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseBoolEnv strictly parses a bool-ish env value. Unlike a permissive
// parser that maps anything unrecognized to false, an unrecognized non-empty
// value (a typo, e.g. "flase") is an error, not a silent result — DRY_RUN is
// the write gate, so a garbled value must fail loud rather than fail toward
// "write".
func parseBoolEnv(key string, def bool) (bool, error) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "":
		return def, nil
	case "1", "true", "yes", "y":
		return true, nil
	case "0", "false", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("%s=%q is not a recognized boolean (true/false/yes/no/1/0)", key, os.Getenv(key))
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
