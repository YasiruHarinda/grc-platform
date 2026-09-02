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

// Command backfill-evidence-actors rewrites risk_evidence.created_by from
// email to uuid for rows that predate the identity migration.
//
// # WHY THIS EXISTS
//
// handleDeleteRiskEvidence's ownership check is a direct string comparison —
// `ev.CreatedBy != actor` — not an id lookup. Once the request path stamps new
// evidence with a uuid (see requireCallerUUID), that comparison stays correct
// for every row created afterward, but every row created before it keeps an
// email in created_by — which a uuid actor can never equal. Left alone, nobody
// could self-delete evidence they uploaded before this shipped; only the
// compliance-admin override would still work.
//
// # THE MATCH IS NARROWER THAN cmd/backfill-uuids
//
// It only rewrites rows whose created_by is a known-resolvable email — one
// that matches a directory entry under -domain (see scim.Client.
// ListUsersByDomain). A row whose uploader has since left, or falls outside
// the searched domain, is left untouched and reported: its original uploader
// permanently loses self-delete on it (falls back to the admin override),
// which is the same fate an un-backfilled `user` row has under the wider
// migration.
//
// # IT WRITES NOTHING
//
// Emits reviewable SQL, never touches the database. Each UPDATE is scoped to
// one exact created_by value, so once a row is rewritten the same statement
// matches nothing on a second run — naturally idempotent without a sentinel
// column.
//
// Usage:
//
//	backfill-evidence-actors [-domain wso2.com] [-out evidence_actors.sql] [-report evidence_actors_report.txt]
//
// Requires the same environment as the server: COMPLIANCE_ENTITY_BASE_URL is
// NOT used — this talks to MySQL directly via DB_DSN, since risk_evidence has
// no Compliance Entity endpoint for a bulk actor rewrite — and the SCIM_*
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

func main() {
	domain := flag.String("domain", "wso2.com",
		"email-domain suffix to search the Asgardeo directory for (e.g. wso2.com, not @wso2.com)")
	outPath := flag.String("out", "backfill_evidence_actors.sql", "file to write the UPDATE statements to")
	reportPath := flag.String("report", "backfill_evidence_actors_report.txt", "file to write the anomaly report to")
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

	// Both helpers are the ones Load uses, so this tool normalises the base URL
	// and derives the token endpoint exactly as the running server does.
	scimBaseURL := config.NormalizeBaseURL(mustEnv("SCIM_BASE_URL"))
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
	// created_by value to any of the candidate ids would silently pick a
	// winner.
	ambiguousEmails := make(map[string]bool)
	var collisions []string
	for _, du := range directoryUsers {
		key := strings.ToLower(strings.TrimSpace(du.Email))
		if key == "" || du.UUID == "" || ambiguousEmails[key] {
			continue
		}
		if prev, ok := uuidByEmail[key]; ok && prev != du.UUID {
			collisions = append(collisions,
				fmt.Sprintf("%s — directory returned two different ids: %s and %s", du.Email, du.UUID, prev))
			delete(uuidByEmail, key)
			ambiguousEmails[key] = true
			continue
		}
		uuidByEmail[key] = du.UUID
	}
	fmt.Printf("directory holds %d distinct email(s)\n", len(uuidByEmail))

	// ── Load every distinct created_by value already on risk_evidence ───────
	rows, err := db.QueryContext(ctx,
		"SELECT DISTINCT created_by FROM risk_evidence WHERE created_by IS NOT NULL AND created_by LIKE '%@%'")
	if err != nil {
		fatal("query risk_evidence.created_by: %v", err)
	}
	var actors []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			fatal("scan created_by: %v", err)
		}
		actors = append(actors, a)
	}
	if err := rows.Close(); err != nil {
		fatal("close risk_evidence.created_by rows: %v", err)
	}
	if err := rows.Err(); err != nil {
		fatal("iterate risk_evidence.created_by: %v", err)
	}
	fmt.Printf("%d distinct email-shaped created_by value(s) on risk_evidence\n", len(actors))

	// ── Resolve ──────────────────────────────────────────────────────────────
	var toRewrite []rewrite
	var unresolved []string
	for _, a := range actors {
		if uuid, ok := uuidByEmail[strings.ToLower(strings.TrimSpace(a))]; ok {
			toRewrite = append(toRewrite, rewrite{email: a, uuid: uuid})
		} else {
			unresolved = append(unresolved, a)
		}
	}

	// ── Write ────────────────────────────────────────────────────────────────
	if err := os.WriteFile(*outPath, []byte(renderSQL(toRewrite, *domain)), 0o600); err != nil {
		fatal("write %s: %v", *outPath, err)
	}
	report := renderReport(unresolved, collisions, len(toRewrite), len(actors), *domain)
	if err := os.WriteFile(*reportPath, []byte(report), 0o600); err != nil {
		fatal("write %s: %v", *reportPath, err)
	}

	fmt.Printf("\n%d rewrite(s) → %s\n", len(toRewrite), *outPath)
	fmt.Printf("anomaly report → %s\n\n", *reportPath)
	fmt.Print(report)

	if findings := len(unresolved) + len(collisions); findings > 0 {
		fmt.Fprintf(os.Stderr, "\n%d finding(s) — review before applying %s\n", findings, *outPath)
		os.Exit(2)
	}
	fmt.Println("no findings; every created_by value resolved.")
}

// rewrite is one created_by value resolved from email to uuid.
type rewrite struct{ email, uuid string }

func renderSQL(rows []rewrite, domain string) string {
	sort.Slice(rows, func(i, j int) bool { return rows[i].email < rows[j].email })

	var b strings.Builder
	fmt.Fprintf(&b, `-- =============================================================================
-- risk_evidence.created_by backfill — GENERATED, review before running.
--
-- Rewrites historical (pre-migration) created_by values from email to uuid, so
-- handleDeleteRiskEvidence's ownership check (a direct string comparison, not
-- an id lookup) keeps recognising the original uploader.
--
-- Each UPDATE is scoped to one exact, no-longer-current created_by value, so
-- once applied it matches nothing on a second run — no guard column needed.
--
-- Directory: %s
--
-- Read the companion report before running this. Anyone listed there keeps an
-- email in created_by and loses self-delete on their historical evidence
-- (falls back to the compliance-admin override).
-- =============================================================================

USE grc_platform;

`, sqlComment(domain))

	if len(rows) == 0 {
		b.WriteString("-- No rewrites. Either nothing to backfill, or nothing resolved; check the report.\n")
		return b.String()
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "-- %s\n", sqlComment(r.email))
		fmt.Fprintf(&b, "UPDATE risk_evidence SET created_by = '%s' WHERE created_by = '%s';\n\n",
			sqlEscape(r.uuid), sqlEscape(r.email))
	}
	b.WriteString(`-- Verify: no row should still hold an email after the findings above are dealt with.
SELECT id, created_by FROM risk_evidence WHERE created_by LIKE '%@%' ORDER BY id;
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

func renderReport(unresolved, collisions []string, rewritten, total int, domain string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "EVIDENCE ACTOR BACKFILL REPORT — %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%d of %d distinct created_by value(s) resolved to a uuid.\n\n", rewritten, total)

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
		fmt.Sprintf("  No uuid could be resolved, so created_by stays an email for these rows.\n"+
			"  The original uploader loses self-delete on them (admin override still\n"+
			"  works). Fix: confirm they have an Asgardeo account under %q, widen\n"+
			"  -domain, or accept it.", domain),
		unresolved)

	section("AMBIGUOUS IN DIRECTORY",
		"  NO REWRITE WAS EMITTED for this created_by value. The directory\n"+
			"  returned two different ids for the same email, so there was no single\n"+
			"  uuid to rewrite it to. Usually a stale or duplicate Asgardeo account.\n"+
			"  Fix: resolve the duplicate in Asgardeo, then re-run.",
		collisions)

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
	fmt.Fprintf(os.Stderr, "backfill-evidence-actors: "+format+"\n", args...)
	os.Exit(1)
}
