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

// Command backfill-escalation-leads fills risk_escalation's
// assigner_lead_uuid / action_owner_lead_uuid for rows that predate them,
// resolving each frozen lead email to their Asgardeo user id.
//
// # WHY THIS EXISTS
//
// risk_escalation freezes the assigner's and action-plan-owner's line
// manager at escalation time, historically as an email — the lead need not
// be a platform user, so there is no `user` row to backfill from the way
// cmd/backfill-uuids does. risk_schema.sql refuses to drop
// assigner_lead_email / action_owner_lead_email until every row that has one
// also has the matching uuid column populated (see the SIGNAL guard right
// before that DROP): this tool is what populates it. Skipping it means the
// schema file aborts rather than silently losing that identity, but the
// escalation stays stuck on the old columns until this runs.
//
// # RESOLUTION SOURCE
//
// Same as cmd/backfill-uuids: email → uuid pairs come from the SCIM
// Operations Service's Users-search endpoint, scoped to a single domain
// suffix (scim.Client.ListUsersByDomain). A lead is only resolvable if their
// email falls under -domain; anyone outside it lands in the report with no
// uuid rather than being silently skipped.
//
// # IT WRITES NOTHING
//
// Emits reviewable SQL, never touches the database. Each UPDATE is guarded
// with the target uuid column IS NULL, so applying the file twice — or
// running it after some rows already got a uuid another way — is a no-op for
// those rows.
//
// Usage:
//
//	backfill-escalation-leads [-domain wso2.com] [-out escalation_leads.sql] [-report escalation_leads_report.txt]
//
// Requires the same environment as the server: COMPLIANCE_ENTITY_BASE_URL is
// NOT used — this talks to MySQL directly via DB_DSN, since risk_escalation
// has no Compliance Entity endpoint for a bulk lead rewrite — and the SCIM_*
// variables.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
)

// leadRole is one of the two frozen-lead columns on risk_escalation. Both
// resolve against the same directory the same way, so everything below is
// generic over this instead of being written out twice.
type leadRole struct {
	label       string // for report/SQL-comment text
	emailColumn string
	uuidColumn  string
}

var leadRoles = []leadRole{
	{label: "assigner lead", emailColumn: "assigner_lead_email", uuidColumn: "assigner_lead_uuid"},
	{label: "action owner lead", emailColumn: "action_owner_lead_email", uuidColumn: "action_owner_lead_uuid"},
}

// rewrite is one (role, email) pair resolved to a uuid.
type rewrite struct {
	role  leadRole
	email string
	uuid  string
}

func main() {
	domain := flag.String("domain", "wso2.com",
		"email-domain suffix to search the Asgardeo directory for (e.g. wso2.com, not @wso2.com)")
	outPath := flag.String("out", "backfill_escalation_leads.sql", "file to write the UPDATE statements to")
	reportPath := flag.String("report", "backfill_escalation_leads_report.txt", "file to write the anomaly report to")
	scimTimeout := flag.Duration("scim-timeout", 90*time.Second,
		"per-request timeout for SCIM calls; the service cold starts, so allow well over an interactive budget")
	timeout := flag.Duration("timeout", 5*time.Minute,
		"overall run deadline; every SCIM page and the DB query share this budget, so it must stay generous relative to -scim-timeout or a slow run can be cut off before -scim-timeout's own allowance is used")
	flag.Parse()

	if strings.TrimSpace(*domain) == "" {
		fatal("-domain must not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	db, err := sql.Open("mysql", mustEnv("DB_DSN"))
	if err != nil {
		fatal("open DB_DSN: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		fatal("connect to database: %v", err)
	}

	// The token endpoint is derived from the base URL and the org rather than
	// read from SCIM_INTERNAL_TOKEN_URL, which no longer exists — via the same
	// config.SCIMTokenURL the server uses, so this tool authenticates against
	// exactly the tenant the running backend does.
	scimBaseURL := mustEnv("SCIM_BASE_URL")
	scimOrg := mustEnv("SCIM_INTERNAL_ORG")
	scimCli := scim.NewClient(
		scimBaseURL, config.SCIMTokenURL(scimBaseURL, scimOrg),
		mustEnv("SCIM_INTERNAL_CLIENT_ID"), mustEnv("SCIM_INTERNAL_CLIENT_SECRET"),
		mustEnv("SCIM_INTERNAL_SCOPES"), scimOrg,
	)
	scimCli.SetHTTPTimeout(*scimTimeout)

	// ── Load the directory ──────────────────────────────────────────────────
	directoryUsers, err := scimCli.ListUsersByDomain(ctx, *domain)
	if err != nil {
		fatal("SCIM search for domain %q: %v", *domain, err)
	}
	fmt.Printf("directory search under %q returned %d user(s)\n", *domain, len(directoryUsers))

	uuidByEmail := make(map[string]string)
	// ambiguousEmails marks an email the directory itself disagrees about, so
	// a later entry can never re-populate uuidByEmail for it — rewriting a
	// lead column to any of the candidate ids would silently pick a winner.
	ambiguousEmails := make(map[string]bool)
	var directoryCollisions []string
	for _, du := range directoryUsers {
		key := strings.ToLower(strings.TrimSpace(du.Email))
		if key == "" || du.UUID == "" || ambiguousEmails[key] {
			continue
		}
		if prev, ok := uuidByEmail[key]; ok && prev != du.UUID {
			directoryCollisions = append(directoryCollisions,
				fmt.Sprintf("%s — directory returned two different ids: %s and %s", du.Email, du.UUID, prev))
			delete(uuidByEmail, key)
			ambiguousEmails[key] = true
			continue
		}
		uuidByEmail[key] = du.UUID
	}
	fmt.Printf("directory holds %d distinct email(s)\n", len(uuidByEmail))

	// ── Resolve each role's not-yet-backfilled leads ─────────────────────────
	var toRewrite []rewrite
	var unresolved []string
	var total int
	for _, role := range leadRoles {
		emails, err := distinctUnbackfilled(ctx, db, role)
		if err != nil {
			fatal("query risk_escalation.%s: %v", role.emailColumn, err)
		}
		fmt.Printf("%d distinct unbackfilled %s email(s)\n", len(emails), role.label)
		total += len(emails)
		for _, e := range emails {
			if uuid, ok := uuidByEmail[strings.ToLower(strings.TrimSpace(e))]; ok {
				toRewrite = append(toRewrite, rewrite{role: role, email: e, uuid: uuid})
			} else {
				unresolved = append(unresolved, fmt.Sprintf("%s: %s", role.label, e))
			}
		}
	}

	// ── Write ────────────────────────────────────────────────────────────────
	if err := os.WriteFile(*outPath, []byte(renderSQL(toRewrite, *domain)), 0o600); err != nil {
		fatal("write %s: %v", *outPath, err)
	}
	report := renderReport(unresolved, directoryCollisions, len(toRewrite), total, *domain)
	if err := os.WriteFile(*reportPath, []byte(report), 0o600); err != nil {
		fatal("write %s: %v", *reportPath, err)
	}

	fmt.Printf("\n%d rewrite(s) → %s\n", len(toRewrite), *outPath)
	fmt.Printf("anomaly report → %s\n\n", *reportPath)
	fmt.Print(report)

	if findings := len(unresolved) + len(directoryCollisions); findings > 0 {
		fmt.Fprintf(os.Stderr, "\n%d finding(s) — review before applying %s\n", findings, *outPath)
		os.Exit(2)
	}
	fmt.Println("no findings; every frozen lead email resolved.")
}

// distinctUnbackfilled returns the distinct emails in role's email column for
// rows where that column is set but the matching uuid column is not — i.e.
// the ones this tool still needs to resolve.
func distinctUnbackfilled(ctx context.Context, db *sql.DB, role leadRole) ([]string, error) {
	query := fmt.Sprintf( // #nosec G201 -- column names come from the fixed leadRoles literal above, never from user/network input
		"SELECT DISTINCT %s FROM risk_escalation WHERE %s IS NOT NULL AND %s IS NULL",
		role.emailColumn, role.emailColumn, role.uuidColumn)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		emails = append(emails, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return emails, nil
}

func renderSQL(rows []rewrite, domain string) string {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].role.uuidColumn != rows[j].role.uuidColumn {
			return rows[i].role.uuidColumn < rows[j].role.uuidColumn
		}
		return rows[i].email < rows[j].email
	})

	var b strings.Builder
	fmt.Fprintf(&b, `-- =============================================================================
-- risk_escalation lead backfill — GENERATED, review before running.
--
-- Resolves the frozen assigner_lead_email / action_owner_lead_email values to
-- their Asgardeo uuid, so risk_schema.sql's guard before DROP COLUMN
-- assigner_lead_email, action_owner_lead_email can pass without losing any
-- open (or historical) escalation's frozen lead identity.
--
-- Each UPDATE is guarded with the target uuid column IS NULL, so this file:
--   * never overwrites a uuid that is already set, and
--   * is a no-op on a second run.
--
-- Directory: %s
--
-- Read the companion report before running this. Anyone listed there keeps a
-- NULL uuid on that lead column, which keeps risk_schema.sql's guard tripped
-- until it is dealt with.
-- =============================================================================

USE grc_platform;

`, sqlComment(domain))

	if len(rows) == 0 {
		b.WriteString("-- No rewrites. Either nothing to backfill, or nothing resolved; check the report.\n")
		return b.String()
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "-- %s: %s\n", r.role.label, sqlComment(r.email))
		fmt.Fprintf(&b, "UPDATE risk_escalation SET %s = '%s' WHERE %s = '%s' AND %s IS NULL;\n\n",
			r.role.uuidColumn, sqlEscape(r.uuid), r.role.emailColumn, sqlEscape(r.email), r.role.uuidColumn)
	}
	b.WriteString(`-- Verify: no row should still be missing a uuid for a lead column it has an
-- email in.
SELECT id, assigner_lead_email, assigner_lead_uuid, action_owner_lead_email, action_owner_lead_uuid
FROM   risk_escalation
WHERE  (assigner_lead_email IS NOT NULL AND assigner_lead_uuid IS NULL)
   OR  (action_owner_lead_email IS NOT NULL AND action_owner_lead_uuid IS NULL)
ORDER  BY id;
`)
	return b.String()
}

// sqlEscape doubles single quotes and backslashes — MySQL treats `\` as an
// escape character inside a string literal unless NO_BACKSLASH_ESCAPES is
// set, so a value ending in one would otherwise escape the closing quote.
func sqlEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "'", "''")
}

var commentLineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

func sqlComment(s string) string { return commentLineBreaks.Replace(s) }

func renderReport(unresolved, directoryCollisions []string, rewritten, total int, domain string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ESCALATION LEAD BACKFILL REPORT — %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%d of %d distinct unbackfilled lead email(s) resolved to a uuid.\n\n", rewritten, total)

	section := func(title, why string, items []string) {
		fmt.Fprintf(&b, "── %s (%d) ──\n%s\n", title, len(items), why)
		if len(items) == 0 {
			b.WriteString("  (none)\n\n")
			return
		}
		sort.Strings(items)
		for _, s := range items {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
		b.WriteString("\n")
	}

	section("NOT FOUND IN DIRECTORY",
		fmt.Sprintf("  No uuid could be resolved, so the lead column stays NULL — this keeps\n"+
			"  risk_schema.sql's pre-DROP guard tripped for these rows, so the guard\n"+
			"  will keep failing the schema run.\n"+
			"  Fix: confirm the lead has an Asgardeo account under %q (or pass a wider\n"+
			"  -domain). A lead who has genuinely left is a case the guard cannot\n"+
			"  resolve automatically — decide by hand whether to hand-write a NULL-\n"+
			"  override for that row before dropping the email columns.", domain),
		unresolved)

	section("AMBIGUOUS IN DIRECTORY",
		"  NO REWRITE WAS EMITTED for these emails. The directory returned two\n"+
			"  different ids for the same email, so there was no single uuid to use.\n"+
			"  Usually a stale or duplicate Asgardeo account.\n"+
			"  Fix: resolve the duplicate in Asgardeo, then re-run.",
		directoryCollisions)

	return b.String()
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fatal("required environment variable is not set: %s", k)
	}
	return v
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "backfill-escalation-leads: "+format+"\n", args...)
	os.Exit(1)
}
