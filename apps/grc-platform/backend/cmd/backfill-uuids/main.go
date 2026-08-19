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

// Command backfill-uuids fills `user`.uuid for rows that predate it, resolving
// each existing user's email to their Asgardeo user id.
//
// # WHY THIS EXISTS
//
// A security review required that the platform stop storing user emails and
// display names. The replacement identity is the Asgardeo `sub` claim, added as
// `user`.uuid — but the column arrives empty, and every existing row was keyed
// on email. Until each row carries the uuid its owner will actually
// authenticate with, the new identity path can resolve nobody.
//
// # RESOLUTION SOURCE
//
// Email → uuid pairs come from the SCIM Operations Service's Users-search
// endpoint, scoped to a single domain suffix (scim.Client.ListUsersByDomain —
// the same bulk call this platform's identity directory refreshes itself
// from). A user is only resolvable if their email falls under -domain; anyone
// outside it lands in the report with no uuid rather than being silently
// skipped.
//
// # IT WRITES NOTHING
//
// The SQL goes to a file for review, never to the database. Re-run it freely:
// against staging first, diff, then production. The generated UPDATEs are
// guarded with `uuid IS NULL`, so applying the file twice is a no-op and an
// already-populated row is never overwritten.
//
// Usage:
//
//	backfill-uuids [-domain wso2.com] [-out uuids.sql] [-report report.txt]
//
// Requires the same environment as the server: COMPLIANCE_ENTITY_BASE_URL and
// the SCIM_* variables.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/entity"
)

// uuidRow is one resolved (email → uuid) pair destined for an UPDATE.
type uuidRow struct {
	email string
	uuid  string
}

// findings collects every user the backfill could not place, plus the cases
// where the directory itself is inconsistent. These are the point of running a
// program rather than writing UPDATEs by hand.
type findings struct {
	// unresolved: a platform user whose email matched no directory entry under
	// -domain. Their uuid stays NULL — they keep working off email for now, but
	// they cannot sign in under the new identity path, so each one is either an
	// email/domain mismatch to investigate or a row to deactivate.
	unresolved []string
	// collisions: two platform users resolving to the SAME uuid. uq_user_uuid
	// would reject the second UPDATE, so no rows are emitted for either — this
	// needs a human to decide which email is the real account.
	collisions []string
	// blankUUID: the directory listed them, but with an empty id. Nothing to
	// write, and it means the search returned an entry without one.
	blankUUID []string
}

func main() {
	domain := flag.String("domain", "wso2.com",
		"email-domain suffix to search the Asgardeo directory for (e.g. wso2.com, not @wso2.com)")
	outPath := flag.String("out", "backfill_uuids.sql", "file to write the UPDATE statements to")
	reportPath := flag.String("report", "backfill_uuids_report.txt", "file to write the anomaly report to")
	scimTimeout := flag.Duration("scim-timeout", 90*time.Second,
		"per-request timeout for SCIM calls; the service cold starts, so allow well over an interactive budget")
	flag.Parse()

	if strings.TrimSpace(*domain) == "" {
		fatal("-domain must not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	entityURL := mustEnv("COMPLIANCE_ENTITY_BASE_URL")
	cli := entityclient.New(entityURL)
	scimCli := scim.NewClient(
		mustEnv("SCIM_BASE_URL"), mustEnv("SCIM_TOKEN_URL"),
		mustEnv("SCIM_CLIENT_ID"), mustEnv("SCIM_CLIENT_SECRET"), mustEnv("SCIM_SCOPES"),
	)
	// The client's default timeout is tuned for the live request path. Nobody is
	// waiting on a backfill, and the SCIM Operations Service is Choreo-hosted:
	// a cold first call can take well over a minute, and timing out here means
	// re-running the whole migration rather than showing someone a slow page.
	scimCli.SetHTTPTimeout(*scimTimeout)

	// ── Load the two inputs ──────────────────────────────────────────────────
	users, err := userentity.NewRepository(cli).List(ctx)
	if err != nil {
		fatal("load users: %v", err)
	}
	fmt.Printf("loaded %d platform user(s) from %s\n", len(users), entityURL)

	var found findings

	directoryUsers, err := scimCli.ListUsersByDomain(ctx, *domain)
	if err != nil {
		fatal("SCIM search for domain %q: %v", *domain, err)
	}
	fmt.Printf("directory search under %q returned %d user(s)\n", *domain, len(directoryUsers))

	uuidByEmail := make(map[string]string)
	for _, du := range directoryUsers {
		key := strings.ToLower(strings.TrimSpace(du.Email))
		if key == "" {
			continue
		}
		if du.UUID == "" {
			found.blankUUID = append(found.blankUUID,
				fmt.Sprintf("%s — directory entry carried no id", du.Email))
			continue
		}
		if prev, ok := uuidByEmail[key]; ok && prev != du.UUID {
			found.collisions = append(found.collisions,
				fmt.Sprintf("%s — directory returned two different ids: %s and %s", du.Email, du.UUID, prev))
			continue
		}
		uuidByEmail[key] = du.UUID
	}
	fmt.Printf("directory holds %d distinct email(s)\n", len(uuidByEmail))

	// ── Resolve each platform user ───────────────────────────────────────────
	// emailsByUUID catches the reverse collision: two DIFFERENT platform rows
	// (say a person's primary address and an alias) mapping to one Asgardeo
	// account. uq_user_uuid makes that unwritable, so it has to be caught here
	// rather than discovered when the generated file half-applies.
	emailsByUUID := make(map[string][]string)
	var rows []uuidRow
	for _, u := range users {
		key := strings.ToLower(strings.TrimSpace(u.Email))
		uuid, ok := uuidByEmail[key]
		if !ok {
			found.unresolved = append(found.unresolved,
				fmt.Sprintf("%s (status %s) — no matching directory entry under %q", u.Email, u.Status, *domain))
			continue
		}
		emailsByUUID[uuid] = append(emailsByUUID[uuid], u.Email)
		rows = append(rows, uuidRow{email: u.Email, uuid: uuid})
	}

	// Drop every row involved in a uuid collision, both sides. Emitting one and
	// not the other would silently pick a winner.
	for uuid, emails := range emailsByUUID {
		if len(emails) < 2 {
			continue
		}
		sort.Strings(emails)
		found.collisions = append(found.collisions,
			fmt.Sprintf("uuid %s claimed by %d user rows: %s", uuid, len(emails), strings.Join(emails, ", ")))
		rows = filterOutUUID(rows, uuid)
	}

	// ── Write ────────────────────────────────────────────────────────────────
	// 0o600, not 0o644: every line pairs someone's email with their stable
	// directory id. World-readable would leak that to any other local account.
	if err := os.WriteFile(*outPath, []byte(renderSQL(rows, *domain)), 0o600); err != nil {
		fatal("write %s: %v", *outPath, err)
	}
	report := renderReport(found, len(rows), len(users), *domain)
	if err := os.WriteFile(*reportPath, []byte(report), 0o600); err != nil {
		fatal("write %s: %v", *reportPath, err)
	}

	fmt.Printf("\n%d uuid row(s) → %s\n", len(rows), *outPath)
	fmt.Printf("anomaly report  → %s\n\n", *reportPath)
	fmt.Print(report)

	// A non-zero exit on findings makes this awkward to run and ignore, which is
	// the intent: an unresolved user cannot authenticate under the new identity.
	if found.total() > 0 {
		fmt.Fprintf(os.Stderr, "\n%d finding(s) — review before applying %s\n", found.total(), *outPath)
		os.Exit(2)
	}
	fmt.Println("no findings; every platform user resolved to a uuid.")
}

func filterOutUUID(rows []uuidRow, uuid string) []uuidRow {
	kept := rows[:0]
	for _, r := range rows {
		if r.uuid != uuid {
			kept = append(kept, r)
		}
	}
	return kept
}

func (f findings) total() int {
	return len(f.unresolved) + len(f.collisions) + len(f.blankUUID)
}

// renderSQL emits UPDATEs keyed on email — the only key that exists on both
// sides before this runs — each guarded so it can never overwrite a uuid that
// is already there.
func renderSQL(rows []uuidRow, domain string) string {
	sort.Slice(rows, func(i, j int) bool { return rows[i].email < rows[j].email })

	var b strings.Builder
	fmt.Fprintf(&b, `-- =============================================================================
-- user.uuid backfill — GENERATED, review before running.
--
-- Produced by cmd/backfill-uuids. Each row's uuid is that person's Asgardeo
-- user id, read from a directory search scoped to: %s
--
-- Every statement is guarded with "uuid IS NULL", so this file:
--   * never overwrites a uuid that is already set, and
--   * is a no-op on a second run.
--
-- Rows are matched on email because that is the only key both sides share
-- before the backfill; email remains in the table precisely so this is
-- possible, and stops being needed once it completes.
--
-- Read the companion report before running this. Anyone listed there is a user
-- this file does NOT cover — they keep a NULL uuid and cannot be resolved
-- through the new identity path.
-- =============================================================================

USE grc_platform;

`, sqlComment(domain))

	if len(rows) == 0 {
		b.WriteString("-- No uuids resolved. That is almost certainly wrong; check the report.\n")
		return b.String()
	}

	for _, r := range rows {
		// sqlComment, not the raw value: a `--` comment only runs to end of
		// line, so a newline in an email would end the comment early and leave
		// the rest of the value as an unguarded top-level SQL statement in a
		// file meant to be piped straight into mysql.
		fmt.Fprintf(&b, "-- %s\n", sqlComment(r.email))
		fmt.Fprintf(&b, "UPDATE `user` SET uuid = '%s' WHERE email = '%s' AND uuid IS NULL;\n\n",
			sqlEscape(r.uuid), sqlEscape(r.email))
	}

	b.WriteString(`-- Verify: no ACTIVE user should be left without a uuid once the report's
-- findings have been dealt with.
SELECT id, email, uuid
FROM   ` + "`user`" + `
WHERE  status = 'ACTIVE' AND uuid IS NULL
ORDER  BY email;
`)
	return b.String()
}

// sqlEscape doubles single quotes. Inputs are emails and ids from your own
// directory, but a generated .sql file that someone will run as root is not the
// place to assume that.
func sqlEscape(s string) string { return strings.ReplaceAll(s, "'", "''") }

// sqlComment neutralises line terminators so a value can never break out of a
// single-line `--` comment. Separate from sqlEscape: a raw newline inside a
// quoted '...' string literal is legal SQL and stays part of the value, so the
// UPDATE statements don't need this — only the plain-text comment lines do.
var commentLineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

func sqlComment(s string) string { return commentLineBreaks.Replace(s) }

func renderReport(f findings, rowCount, userCount int, domain string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "UUID BACKFILL REPORT — %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%d of %d platform user(s) resolved to a uuid.\n\n", rowCount, userCount)

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
		fmt.Sprintf("  No uuid could be resolved, so their row keeps a NULL one. They still\n"+
			"  work off email today, but cannot be resolved through the new identity\n"+
			"  path — and will fail outright once uuid becomes NOT NULL.\n"+
			"  Fix: confirm they have an Asgardeo account whose email falls under\n"+
			"  %q (or pass a wider -domain), or deactivate the row if the person has\n"+
			"  left.", domain),
		f.unresolved)

	section("TWO USER ROWS, ONE UUID",
		"  NO ROWS WERE EMITTED for either side. user.uuid is UNIQUE, so writing\n"+
			"  both would fail halfway through the file and leave the backfill\n"+
			"  half-applied. Usually an alias address with its own user row.\n"+
			"  Fix: merge or deactivate the duplicate row, then re-run.",
		f.collisions)

	section("DIRECTORY ENTRY WITH NO ID",
		"  The directory search returned an entry carrying an email but no uuid,\n"+
			"  so there was nothing to write. Worth checking that account in Asgardeo.",
		f.blankUUID)

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
	fmt.Fprintf(os.Stderr, "backfill-uuids: "+format+"\n", args...)
	os.Exit(1)
}
