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

// Package grant resolves a caller's role grants into the privileges they hold,
// and — crucially — into the privileges they hold *in a particular scope*.
//
// Roles are assigned in this platform's own database, not in Asgardeo, which
// authenticates users and nothing more. A grant is a triple of (user, role,
// scope): either GLOBAL, meaning every scope the role's module defines, or one
// specific team.
//
// The problem this package exists to solve is that a user no longer has one
// privilege set. Someone may be Risk Owner in one register and Risk Assigner in
// another; flattening those into a single set would let them approve, as owner,
// a risk belonging to the register where they are merely an assigner. So a Set
// answers two different questions, and callers must be deliberate about which
// one they are asking:
//
//	Has(priv)             — "may this user do X anywhere?"      (union)
//	HasIn(priv, teamID)   — "may this user do X on THIS thing?" (scoped)
//
// Has is the weaker of the two and is not an authorisation decision on its own.
// It exists for endpoints with no object in hand, and to keep the Audit Hub
// working unchanged until its own migration. Anywhere a team id is available,
// HasIn is the check that counts.
package grant

import (
	"context"
	"sort"
)

type contextKey struct{}

// WithContext stores the caller's resolved Set in the request context.
// Called by the auth middleware once per request.
func WithContext(ctx context.Context, s *Set) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// FromContext retrieves the caller's Set.
//
// Returns nil when no grant repository was configured (local dev), which every
// Set method treats as "holds nothing" — so a scoped check fails closed. The
// allow-all behaviour of that mode lives in shared/auth, which is the only
// place that decides to skip a check entirely.
func FromContext(ctx context.Context) *Set {
	s, _ := ctx.Value(contextKey{}).(*Set)
	return s
}

// Scope types, mirroring user_role_grant.scope_type.
const (
	ScopeGlobal    = "GLOBAL"
	ScopeRiskTeam  = "RISK_TEAM"
	ScopeAuditTeam = "AUDIT_TEAM"
)

// Scope bases, mirroring role.scope_basis: which column of a risk a grant on
// this role scopes by. A risk_team row can be BOTH a register and an assignment
// team, so the scope id alone cannot say which sense was meant.
const (
	BasisSourceRegister = "SOURCE_REGISTER"
	BasisAssignmentTeam = "ASSIGNMENT_TEAM"
)

// Team types, mirroring risk_team.team_type. Dashboards and analytics are
// rendered per register, so only a scope whose team can BE a register
// contributes to them.
const (
	TeamSourceRegister = "SOURCE_REGISTER"
	TeamAssignment     = "ASSIGNMENT"
	TeamBoth           = "BOTH"
)

// Grant is one role held in one scope, as returned by the Compliance Entity.
type Grant struct {
	RoleName  string `json:"roleName"`
	Module    string `json:"module"`
	ScopeType string `json:"scopeType"`
	ScopeID   int    `json:"scopeId"`
	ScopeName string `json:"scopeName"`
	// ScopeBasis: SOURCE_REGISTER | ASSIGNMENT_TEAM, from the role.
	ScopeBasis string `json:"scopeBasis"`
	// ScopeTeamType: the scoped team's own SOURCE_REGISTER | ASSIGNMENT | BOTH.
	ScopeTeamType string `json:"scopeTeamType"`
}

// Set is a caller's resolved authorisation state for one request.
//
// Construct it with Resolve. The zero value is a caller with no grants at all,
// which is a legitimate state — someone named as an action owner may hold no
// role — and it correctly answers false to everything.
type Set struct {
	grants []Grant

	// union is every privilege from every grant, regardless of scope.
	union map[string]bool
	// global is the privileges granted at GLOBAL scope, which therefore apply
	// in every scope including ones created after the grant was made.
	global map[string]bool
	// byTeam maps a team id to the privileges granted specifically there.
	// GLOBAL privileges are deliberately NOT copied into these entries: doing
	// so would make a global grant indistinguishable from a per-team one, and
	// would silently stop covering teams created later.
	byTeam map[int]map[string]bool
}

// PrivilegeResolver maps role names to the privileges they carry. Implemented
// by privilege.Store, whose mapping is cached and refreshed periodically —
// role→privilege changes only on a deploy, unlike grants, which are on the
// revocation path and are read fresh on every request.
type PrivilegeResolver interface {
	Resolve(roles []string) map[string]bool
}

// Resolve turns raw grants into a Set using the role→privilege mapping.
func Resolve(grants []Grant, r PrivilegeResolver) *Set {
	s := &Set{
		grants: grants,
		union:  map[string]bool{},
		global: map[string]bool{},
		byTeam: map[int]map[string]bool{},
	}
	for _, g := range grants {
		privs := r.Resolve([]string{g.RoleName})
		for p := range privs {
			s.union[p] = true
		}
		if g.ScopeType == ScopeGlobal {
			for p := range privs {
				s.global[p] = true
			}
			continue
		}
		if s.byTeam[g.ScopeID] == nil {
			s.byTeam[g.ScopeID] = map[string]bool{}
		}
		for p := range privs {
			s.byTeam[g.ScopeID][p] = true
		}
	}
	return s
}

// Has reports whether the caller holds priv in ANY scope.
//
// Not an authorisation decision by itself: it answers "could this user do X
// somewhere", not "may they do it here". Use HasIn wherever a team id exists.
func (s *Set) Has(priv string) bool {
	if s == nil {
		return false
	}
	return s.union[priv]
}

// HasIn reports whether the caller holds priv in the given team's scope,
// either through a GLOBAL grant or a grant on that specific team.
//
// This is the real enforcement. Pass the team whose authority governs the
// action — for a risk that is its SOURCE register, never its assignment team:
// assignment is an ordinary field any assigner can set, and letting it confer
// authority would turn "route this to Legal" into "give Legal approval rights
// over this risk".
func (s *Set) HasIn(priv string, teamID int) bool {
	if s == nil {
		return false
	}
	if s.global[priv] {
		return true
	}
	return s.byTeam[teamID][priv]
}

// IsGlobal reports whether the caller holds ANY GLOBAL grant.
//
// Rarely the right question. Use HasGlobal(priv) for visibility decisions: a
// platform admin holding only MANAGE_USERS globally satisfies IsGlobal but has
// no business seeing every risk, and treating them as unrestricted would let a
// second, narrow grant carry them past the route gate into an unscoped list.
func (s *Set) IsGlobal() bool {
	if s == nil {
		return false
	}
	return len(s.global) > 0
}

// HasGlobal reports whether the caller holds priv at GLOBAL scope — i.e. they
// hold it everywhere, including in teams created after the grant was made.
//
// This is what "sees every risk" must be built on: ask for the privilege that
// actually confers the thing (RISK_VIEW_RISKS for the registers list), not for
// the mere existence of some global grant.
func (s *Set) HasGlobal(priv string) bool {
	if s == nil {
		return false
	}
	return s.global[priv]
}

// SourceScopeIDs returns the team ids of grants that scope by SOURCE register —
// the registers whose risks the caller may see because they work in them.
// Matched against risk.source_register_id.
func (s *Set) SourceScopeIDs() []int {
	return s.scopeIDsWithBasis(BasisSourceRegister, false)
}

// AssignmentScopeIDs returns the team ids of grants that scope by ASSIGNMENT
// team — work routed to the caller's team. Matched against
// risk.assignment_team_id.
func (s *Set) AssignmentScopeIDs() []int {
	return s.scopeIDsWithBasis(BasisAssignmentTeam, false)
}

// RegisterScopeIDs returns the team ids the caller may see REGISTER-BASED pages
// for: dashboards and analytics, which are rendered per register and always
// aggregate by source_register_id.
//
// A grant counts only if its team can actually be a register — team_type
// SOURCE_REGISTER or BOTH. That is what makes "Risk Owner @ Asgardeo" (a BOTH
// team) contribute a dashboard while "Risk Owner @ HR" (ASSIGNMENT-only) does
// not: HR is not a register, so it has no register page to appear on.
func (s *Set) RegisterScopeIDs() []int {
	return s.scopeIDsWithBasis("", true)
}

// RegisterScopeIDsFor returns the register-capable team ids where the caller
// holds priv SPECIFICALLY — not merely where they hold some grant.
//
// RegisterScopeIDs alone cannot be used to scope a privilege-gated aggregate: it
// only knows "a register-scoped grant exists on this team", not which
// privileges that grant carries. A caller can hold different roles in
// different registers, so a register reachable through one role's grant is not
// necessarily one where THIS privilege is held. Every risk role today happens
// to bundle the view privileges with everything else it grants, which is why
// that gap has no visible effect yet — but nothing enforces the bundling, and
// a future narrower role would silently leak that register's aggregate data
// into a page the grant never authorised.
//
// Pair with HasGlobal(priv) for the "sees everything" bypass: a caller holding
// priv at GLOBAL scope needs no restriction, including for registers created
// after the grant was made.
func (s *Set) RegisterScopeIDsFor(priv string) []int {
	if s == nil {
		return nil
	}
	seen := map[int]bool{}
	out := []int{}
	for _, g := range s.grants {
		if g.ScopeType == ScopeGlobal {
			continue
		}
		if g.ScopeTeamType != TeamSourceRegister && g.ScopeTeamType != TeamBoth {
			continue
		}
		if !s.byTeam[g.ScopeID][priv] {
			continue
		}
		if !seen[g.ScopeID] {
			seen[g.ScopeID] = true
			out = append(out, g.ScopeID)
		}
	}
	sort.Ints(out)
	return out
}

// scopeIDsWithBasis collects distinct, sorted scope ids. basis "" means any.
// registerCapableOnly additionally requires the team to be usable as a register.
func (s *Set) scopeIDsWithBasis(basis string, registerCapableOnly bool) []int {
	if s == nil {
		return nil
	}
	seen := map[int]bool{}
	out := []int{}
	for _, g := range s.grants {
		if g.ScopeType == ScopeGlobal {
			continue
		}
		if basis != "" && g.ScopeBasis != basis {
			continue
		}
		if registerCapableOnly &&
			g.ScopeTeamType != TeamSourceRegister && g.ScopeTeamType != TeamBoth {
			continue
		}
		if !seen[g.ScopeID] {
			seen[g.ScopeID] = true
			out = append(out, g.ScopeID)
		}
	}
	sort.Ints(out)
	return out
}

// IsEmpty reports whether the caller holds no grants at all.
//
// Such a caller is not necessarily unauthorised: they reach exactly the risks
// they are personally named on (owner, assigner, management approver, or an
// action plan's action owner). This replaces the former isActionOwnerOnly
// classifier, which inferred the same state from privileges rather than
// knowing it.
func (s *Set) IsEmpty() bool {
	return s == nil || len(s.grants) == 0
}

// TeamIDs returns the team ids the caller holds a non-global grant on, sorted
// for stable query plans and reproducible test output. Empty for a GLOBAL-only
// caller, whose access is not expressible as a team list — check IsGlobal
// first, or an unrestricted caller will be scoped to nothing.
func (s *Set) TeamIDs() []int {
	if s == nil {
		return nil
	}
	ids := make([]int, 0, len(s.byTeam))
	for id := range s.byTeam {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// PrivilegesIn returns, sorted, the privileges the caller holds in one scope —
// their GLOBAL grants plus any grant on that specific team.
//
// This is what a client needs to render actions for one object truthfully.
// Handing the browser the union instead would show buttons that 403, and
// handing it the raw grants would mean reimplementing the access rule there.
func (s *Set) PrivilegesIn(teamID int) []string {
	if s == nil {
		return []string{}
	}
	out := make([]string, 0, len(s.global)+len(s.byTeam[teamID]))
	seen := make(map[string]bool, cap(out))
	for _, m := range []map[string]bool{s.global, s.byTeam[teamID]} {
		for p := range m {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Grants returns the caller's raw grants.
func (s *Set) Grants() []Grant {
	if s == nil {
		return nil
	}
	return s.grants
}

// PrivilegeMap returns the union as a map, the shape the privilege package's
// context helper expects.
//
// Publishing the union under that key is what lets the Audit Hub's 47 unscoped
// RequirePrivilege call sites keep working untouched while it migrates: they
// get the same answer they got from group-derived privileges, just sourced from
// grants instead. It is a compatibility bridge, not the enforcement — scoped
// checks go through HasIn.
func (s *Set) PrivilegeMap() map[string]bool {
	if s == nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(s.union))
	for p := range s.union {
		out[p] = true
	}
	return out
}

// NewForTest builds a Set from already-resolved privileges, bypassing the
// role→privilege lookup. Unit tests only.
func NewForTest(global map[string]bool, byTeam map[int]map[string]bool, grantCount int) *Set {
	s := &Set{
		union:  map[string]bool{},
		global: map[string]bool{},
		byTeam: map[int]map[string]bool{},
		grants: make([]Grant, grantCount),
	}
	for p := range global {
		s.global[p] = true
		s.union[p] = true
	}
	for team, privs := range byTeam {
		s.byTeam[team] = map[string]bool{}
		for p := range privs {
			s.byTeam[team][p] = true
			s.union[p] = true
		}
	}
	return s
}
