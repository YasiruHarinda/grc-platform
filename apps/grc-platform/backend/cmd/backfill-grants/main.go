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

// Command backfill-grants converts the platform's existing authorisation state
// into user_role_grant rows, for the migration off Asgardeo groups.
//
// # WHY THIS EXISTS
//
// user_role_grant starts empty. The moment the backend stops reading the JWT's
// groups claim, that table is the only thing granting access — so if it is
// empty at cutover, every user loses everything at once. Creating the rows that
// reproduce today's access is the backfill.
//
// Today the answer is split across two systems:
//
//	Asgardeo         which groups a person is in   → their ROLE
//	user_risk_team   which teams they belong to    → their SCOPE
//
// Both are already written down, so a program can convert them. A human typing
// rows into an admin UI cannot notice the people they forgot exist, which is
// exactly what the anomaly report below is for.
//
// THE RULE
//
//	a role carrying an org-wide privilege  → one GLOBAL grant
//	any other role                         → one RISK_TEAM grant per membership
//
// That is the retired isTeamScopedOnly classifier, run once as a migration
// instead of on every request. After this, nothing infers standing again.
//
// # IT WRITES NOTHING
//
// The SQL goes to a file for review, never to the database. Re-run it freely:
// against staging first, diff, then production.
//
// Usage:
//
//	backfill-grants [-out grants.sql] [-report report.txt]
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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/entity"
)

// orgWidePrivileges decides which roles become GLOBAL grants.
//
// This is a verbatim snapshot of the retired seesEveryRisk allowlist from
// risk_registers.go — the privileges that were only ever granted to
// Compliance/Management/Admin, and whose holders therefore saw every register
// unscoped. It is deliberately frozen here rather than imported: the live code
// no longer has such a list (that is the point of the migration), and this one
// describes the OLD model, which stops changing the moment the backfill runs.
//
// Do not "keep it up to date". It is a historical fact, not a policy.
var orgWidePrivileges = map[string]bool{
	"RISK_VIEW_ALL_RISKS":         true,
	"RISK_COMPLIANCE_APPROVE":     true,
	"RISK_COMPLIANCE_REJECT":      true,
	"RISK_CLOSE":                  true,
	"RISK_ESCALATE":               true,
	"RISK_MANAGE_COMPLIANCE_REFS": true,
	"RISK_MANAGEMENT_APPROVE":     true,
	"RISK_MANAGEMENT_REJECT":      true,
	"RISK_MANAGE_TEAMS":           true,
	"RISK_MANAGE_SCORES":          true,
}

// manageUsersPrivilege gates User Management — provisioning users and granting
// or revoking roles. The Compliance Entity refuses to grant any role carrying it
// at anything other than GLOBAL scope (see grant_service.validateScope), because
// a scoped grant-manager would need an escalation rule that does not exist yet.
//
// This command writes SQL straight into user_role_grant, bypassing that service
// entirely, so it has to honour the same rule or it can produce rows the API
// would have rejected.
const manageUsersPrivilege = "MANAGE_USERS"

type role struct {
	ID       int    `json:"id"`
	RoleName string `json:"roleName"`
	Module   string `json:"module"`
	Status   string `json:"status"`
}

// grantRow is one row destined for user_role_grant.
type grantRow struct {
	email     string
	roleName  string
	scopeType string
	scopeID   int
}

// findings collects everyone the rule could not place. These are the point of
// running a program rather than typing rows by hand.
type findings struct {
	// inGroupNoUser: exists to Asgardeo, not to this platform. Would silently
	// receive nothing.
	inGroupNoUser []string
	// hasTeamsNoRole: works today via team scoping, but belongs to no Asgardeo
	// group — so the rule produces no grant and they go dark at cutover.
	hasTeamsNoRole []string
	// scopedRoleNoTeams: holds a team-scoped role but no memberships, so there
	// is nothing to scope a grant to. Matches today's behaviour (an empty
	// page), but worth eyeballing.
	scopedRoleNoTeams []string
	// manageUsersNotGlobal: the role carries MANAGE_USERS but is not SHARED, so
	// it would be scoped to a team — which the entity refuses. Needs a human
	// decision, so no grant is emitted at all.
	manageUsersNotGlobal []string
}

func main() {
	outPath := flag.String("out", "backfill_grants.sql", "file to write the INSERT statements to")
	reportPath := flag.String("report", "backfill_report.txt", "file to write the anomaly report to")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	entityURL := mustEnv("COMPLIANCE_ENTITY_BASE_URL")
	cli := entityclient.New(entityURL)
	scimCli := scim.NewClient(
		mustEnv("SCIM_BASE_URL"), mustEnv("SCIM_TOKEN_URL"),
		mustEnv("SCIM_CLIENT_ID"), mustEnv("SCIM_CLIENT_SECRET"), mustEnv("SCIM_SCOPES"),
	)

	// ── Load the three inputs ────────────────────────────────────────────────
	var rolesResp struct {
		Roles []role `json:"roles"`
	}
	if err := cli.Get(ctx, "/roles", &rolesResp); err != nil {
		fatal("load roles: %v", err)
	}
	var rpResp struct {
		RolePrivileges map[string][]string `json:"rolePrivileges"`
	}
	if err := cli.Get(ctx, "/role-privileges", &rpResp); err != nil {
		fatal("load role-privileges: %v", err)
	}
	users, err := userentity.NewRepository(cli).List(ctx)
	if err != nil {
		fatal("load users: %v", err)
	}

	byEmail := make(map[string]*user.User, len(users))
	for _, u := range users {
		byEmail[strings.ToLower(u.Email)] = u
	}
	fmt.Printf("loaded %d roles, %d users from %s\n", len(rolesResp.Roles), len(users), entityURL)

	// ── Convert ──────────────────────────────────────────────────────────────
	var (
		rows  []grantRow
		found findings
		seen  = map[string]bool{} // dedupe: (email, role, scope)
	)
	// Track which users the rule reached, to spot those it reached via no role.
	reached := map[string]bool{}

	for _, r := range rolesResp.Roles {
		if r.Status != "ACTIVE" {
			continue
		}
		// AUDIT roles are out of scope: their scope comes from user_audit_team,
		// not user_risk_team, so converting them here would attach risk-team
		// ids to audit roles. The Audit Hub runs its own pass with the same
		// rule and a different scope source.
		if r.Module == "AUDIT" {
			fmt.Printf("  %-42s skipped (AUDIT module — see the Audit Hub's own backfill)\n", r.RoleName)
			continue
		}
		// role_name is the Asgardeo group name — that equivalence is what makes
		// the role table itself the list of groups to query.
		emails, err := scimCli.ListGroupMemberEmails(ctx, r.RoleName)
		if err != nil {
			fatal("SCIM lookup for group %q: %v\n"+
				"  (a role with no matching Asgardeo group is itself a finding — "+
				"resolve it before trusting this output)", r.RoleName, err)
		}
		// A SHARED role can ONLY be granted GLOBAL — it spans both hubs, so no
		// single team table its scope could point at. Deciding this by
		// privilege alone would emit RISK_TEAM rows the entity rejects, since
		// grc-platform-admin carries MANAGE_USERS rather than any of the
		// org-wide risk privileges below.
		privs := rpResp.RolePrivileges[r.RoleName]
		global := r.Module == "SHARED" || isOrgWide(privs)

		// A non-SHARED role carrying MANAGE_USERS cannot be placed. Emitting
		// team-scoped grants would write rows the entity's API refuses; quietly
		// promoting it to GLOBAL would widen every OTHER privilege it carries
		// from one team to every team, which is a privilege escalation this
		// tool has no business performing on someone's behalf. Report it and
		// let a human decide whether the role or the scope is wrong.
		if !global && hasPrivilege(privs, manageUsersPrivilege) {
			found.manageUsersNotGlobal = append(found.manageUsersNotGlobal,
				fmt.Sprintf("%s (module %s, %d member(s)) — carries %s but is not SHARED",
					r.RoleName, r.Module, len(emails), manageUsersPrivilege))
			continue
		}

		fmt.Printf("  %-42s %-6s %d member(s)\n", r.RoleName, scopeLabel(global), len(emails))

		for _, email := range emails {
			key := strings.ToLower(strings.TrimSpace(email))
			u, ok := byEmail[key]
			if !ok {
				found.inGroupNoUser = append(found.inGroupNoUser,
					fmt.Sprintf("%s (in group %s)", email, r.RoleName))
				continue
			}
			reached[key] = true

			if global {
				add(&rows, seen, grantRow{email: u.Email, roleName: r.RoleName, scopeType: "GLOBAL", scopeID: 0})
				continue
			}
			if len(u.RiskTeamIDs) == 0 {
				found.scopedRoleNoTeams = append(found.scopedRoleNoTeams,
					fmt.Sprintf("%s (role %s, no risk-team membership)", u.Email, r.RoleName))
				continue
			}
			for _, teamID := range u.RiskTeamIDs {
				add(&rows, seen, grantRow{
					email: u.Email, roleName: r.RoleName,
					scopeType: "RISK_TEAM", scopeID: teamID,
				})
			}
		}
	}

	// Anyone with team memberships that no role reached would lose all access.
	for _, u := range users {
		if len(u.RiskTeamIDs) > 0 && !reached[strings.ToLower(u.Email)] {
			found.hasTeamsNoRole = append(found.hasTeamsNoRole,
				fmt.Sprintf("%s (member of %d risk team(s), in no Asgardeo group)", u.Email, len(u.RiskTeamIDs)))
		}
	}

	// ── Write ────────────────────────────────────────────────────────────────
	// 0o600, not 0o644: every line in these files is someone's email paired
	// with their exact role and scope — a full authorization map for the org.
	// World-readable would leak that to any other local account.
	if err := os.WriteFile(*outPath, []byte(renderSQL(rows)), 0o600); err != nil {
		fatal("write %s: %v", *outPath, err)
	}
	report := renderReport(found, len(rows))
	if err := os.WriteFile(*reportPath, []byte(report), 0o600); err != nil {
		fatal("write %s: %v", *reportPath, err)
	}

	fmt.Printf("\n%d grant row(s) → %s\n", len(rows), *outPath)
	fmt.Printf("anomaly report  → %s\n\n", *reportPath)
	fmt.Print(report)

	// A non-zero exit on findings makes this awkward to run and ignore, which
	// is the intent: every entry is a person who ends up with the wrong access.
	if found.total() > 0 {
		fmt.Fprintf(os.Stderr, "\n%d finding(s) — review before applying %s\n", found.total(), *outPath)
		os.Exit(2)
	}
	fmt.Println("no findings; every group member was placed.")
}

func hasPrivilege(privs []string, want string) bool {
	for _, p := range privs {
		if p == want {
			return true
		}
	}
	return false
}

func isOrgWide(privs []string) bool {
	for _, p := range privs {
		if orgWidePrivileges[p] {
			return true
		}
	}
	return false
}

func scopeLabel(global bool) string {
	if global {
		return "GLOBAL"
	}
	return "team"
}

func add(rows *[]grantRow, seen map[string]bool, g grantRow) {
	key := fmt.Sprintf("%s|%s|%s|%d", strings.ToLower(g.email), g.roleName, g.scopeType, g.scopeID)
	if seen[key] {
		return
	}
	seen[key] = true
	*rows = append(*rows, g)
}

func (f findings) total() int {
	return len(f.inGroupNoUser) + len(f.hasTeamsNoRole) + len(f.scopedRoleNoTeams) +
		len(f.manageUsersNotGlobal)
}

// renderSQL emits INSERTs that resolve ids by natural key rather than hardcoding
// them, so the same file is valid in any environment regardless of
// AUTO_INCREMENT ordering — the pattern risk_module_data_schema.sql already uses.
func renderSQL(rows []grantRow) string {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].email != rows[j].email {
			return rows[i].email < rows[j].email
		}
		if rows[i].roleName != rows[j].roleName {
			return rows[i].roleName < rows[j].roleName
		}
		return rows[i].scopeID < rows[j].scopeID
	})

	var b strings.Builder
	b.WriteString(`-- =============================================================================
-- user_role_grant backfill — GENERATED, review before running.
--
-- Produced by cmd/backfill-grants from the authorisation state that existed
-- before roles moved into this database: Asgardeo group membership (the role)
-- crossed with user_risk_team (the scope).
--
-- Rule applied:
--   role carrying an org-wide privilege -> one GLOBAL grant
--   any other role                      -> one RISK_TEAM grant per membership
--
-- Ids are resolved by natural key (email, role name), so this file is valid in
-- any environment. Re-runnable: ON DUPLICATE KEY reactivates rather than
-- failing, so applying it twice is a no-op.
--
-- Read the companion report before running this. Anyone listed there is a
-- person this file does NOT cover.
-- =============================================================================

USE grc_platform;

`)
	if len(rows) == 0 {
		b.WriteString("-- No grants produced. That is almost certainly wrong; check the report.\n")
		return b.String()
	}

	for _, g := range rows {
		scope := "GLOBAL"
		note := "every register, including ones created later"
		if g.scopeType != "GLOBAL" {
			scope = fmt.Sprintf("RISK_TEAM %d", g.scopeID)
			note = fmt.Sprintf("risk_team.id = %d", g.scopeID)
		}
		fmt.Fprintf(&b, "-- %s: %s @ %s (%s)\n", g.email, g.roleName, scope, note)
		fmt.Fprintf(&b, `INSERT INTO user_role_grant (user_id, role_id, scope_type, scope_id, created_by)
SELECT u.id, r.id, '%s', %d, 'backfill'
FROM   `+"`user`"+` u
JOIN   `+"`role`"+` r ON r.role_name = '%s'
WHERE  u.email = '%s'
ON DUPLICATE KEY UPDATE status = 'ACTIVE';

`, g.scopeType, g.scopeID, sqlEscape(g.roleName), sqlEscape(g.email))
	}

	b.WriteString(`-- Verify: every row below should look deliberate.
SELECT u.email, r.role_name, g.scope_type, g.scope_id
FROM   user_role_grant g
JOIN   ` + "`user`" + ` u ON u.id = g.user_id
JOIN   ` + "`role`" + ` r ON r.id = g.role_id
WHERE  g.created_by = 'backfill' AND g.status = 'ACTIVE'
ORDER  BY u.email, r.role_name, g.scope_id;
`)
	return b.String()
}

// sqlEscape doubles single quotes. Inputs are role names and emails from your
// own directory, but a generated .sql file that someone will run as root is not
// the place to assume that.
func sqlEscape(s string) string { return strings.ReplaceAll(s, "'", "''") }

func renderReport(f findings, rowCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "BACKFILL REPORT — %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%d grant row(s) generated.\n\n", rowCount)

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

	section("IN AN ASGARDEO GROUP, BUT NOT A PLATFORM USER",
		"  They exist to the identity provider but have no `user` row here, so no\n"+
			"  grant can reference them. They will silently have no access.\n"+
			"  Fix: provision the user first, then re-run.",
		f.inGroupNoUser)

	section("HAS RISK-TEAM MEMBERSHIPS, BUT IN NO ASGARDEO GROUP",
		"  THE DANGEROUS ONE. They can see risks today purely through team\n"+
			"  membership, and this backfill produces nothing for them — so they are\n"+
			"  locked out at cutover. Decide what role each should hold and grant it.",
		f.hasTeamsNoRole)

	section("CARRIES MANAGE_USERS BUT IS NOT A SHARED ROLE",
		"  NO GRANTS WERE EMITTED for these roles, so their members get nothing.\n"+
			"  A role that hands out authority may only be granted GLOBAL, and a\n"+
			"  non-SHARED role would be scoped to a team. Either move MANAGE_USERS\n"+
			"  off the role, or make the role SHARED — then re-run.",
		f.manageUsersNotGlobal)

	section("TEAM-SCOPED ROLE, BUT NO TEAM MEMBERSHIPS",
		"  Nothing to scope a grant to. This matches today's behaviour (they get\n"+
			"  an empty page), so it is usually fine — but confirm it is intended.",
		f.scopedRoleNoTeams)

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
	fmt.Fprintf(os.Stderr, "backfill-grants: "+format+"\n", args...)
	os.Exit(1)
}
