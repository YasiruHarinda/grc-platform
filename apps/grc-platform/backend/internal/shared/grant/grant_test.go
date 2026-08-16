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

package grant

import "testing"

// fakeResolver maps a role name straight to a caller-supplied privilege set,
// bypassing privilege.Store so these tests describe roles directly.
type fakeResolver map[string]map[string]bool

func (f fakeResolver) Resolve(roles []string) map[string]bool {
	out := map[string]bool{}
	for _, r := range roles {
		for p := range f[r] {
			out[p] = true
		}
	}
	return out
}

// TestRegisterScopeIDsForDoesNotLeakOtherPrivilegesRegisters is the case a
// register-based aggregate (dashboard, analytics) must get right: a caller
// holding two DIFFERENT roles in two DIFFERENT registers, only one of which
// carries the page's privilege, must not have the other register's data
// folded in just because some grant exists there.
//
// Every risk role today happens to bundle RISK_VIEW_DASHBOARD /
// RISK_VIEW_ANALYTICS with everything else it grants, so this is not reachable
// with the current role catalogue — but nothing enforces that bundling, and
// RegisterScopeIDs (unlike RegisterScopeIDsFor) has no way to tell the two
// registers apart. This test is what catches a future role that breaks the
// assumption.
func TestRegisterScopeIDsForDoesNotLeakOtherPrivilegesRegisters(t *testing.T) {
	const (
		asgardeo = 1
		choreo   = 2
	)
	resolver := fakeResolver{
		// Holds the page privilege here...
		"risk-compliance-team": {"RISK_VIEW_ANALYTICS": true},
		// ...but NOT here — a narrower role scoped to a different register.
		"risk-assessor-only": {"RISK_ASSESS": true},
	}
	set := Resolve([]Grant{
		{RoleName: "risk-compliance-team", ScopeType: ScopeRiskTeam, ScopeID: asgardeo, ScopeBasis: BasisSourceRegister, ScopeTeamType: TeamBoth},
		{RoleName: "risk-assessor-only", ScopeType: ScopeRiskTeam, ScopeID: choreo, ScopeBasis: BasisSourceRegister, ScopeTeamType: TeamBoth},
	}, resolver)

	got := set.RegisterScopeIDsFor("RISK_VIEW_ANALYTICS")
	if len(got) != 1 || got[0] != asgardeo {
		t.Fatalf("RegisterScopeIDsFor(RISK_VIEW_ANALYTICS) = %v, want [%d] only — "+
			"Choreo must not leak in just because SOME grant exists there", got, asgardeo)
	}

	// The un-filtered method is a different question — "any register-scoped
	// grant at all" — and is expected to include both. It documents why the
	// two methods cannot be used interchangeably; RegisterScopeIDs is NOT a
	// safe substitute for a privilege-gated aggregate.
	unfiltered := set.RegisterScopeIDs()
	if len(unfiltered) != 2 {
		t.Fatalf("RegisterScopeIDs() = %v, want both registers (this is the unsafe one)", unfiltered)
	}
}

// TestRegisterScopeIDsForHonoursTeamType confirms an ASSIGNMENT-only team (HR,
// Legal) never contributes to a register-based page, even when the caller
// holds the exact privilege there — there is no register page for it to
// appear on.
func TestRegisterScopeIDsForHonoursTeamType(t *testing.T) {
	const hr = 9
	resolver := fakeResolver{"owner": {"RISK_VIEW_DASHBOARD": true}}
	set := Resolve([]Grant{
		{RoleName: "owner", ScopeType: ScopeRiskTeam, ScopeID: hr, ScopeBasis: BasisAssignmentTeam, ScopeTeamType: TeamAssignment},
	}, resolver)

	if got := set.RegisterScopeIDsFor("RISK_VIEW_DASHBOARD"); len(got) != 0 {
		t.Fatalf("RegisterScopeIDsFor = %v, want empty — HR is ASSIGNMENT-only, not a register", got)
	}
}

// TestRegisterScopeIDsForGlobalIsNotIncluded documents that GLOBAL grants are
// deliberately absent from the returned list: HasGlobal(priv) is the caller's
// separate bypass for "unrestricted", not a member of this list. A caller
// holding the privilege GLOBALLY plus scoped in one register should still get
// exactly that one register back — the handler decides to skip filtering
// entirely once HasGlobal is true, rather than this method trying to represent
// "every register including ones created later" as a finite list.
func TestRegisterScopeIDsForGlobalIsNotIncluded(t *testing.T) {
	const asgardeo = 1
	resolver := fakeResolver{"admin": {"RISK_VIEW_ANALYTICS": true}}
	set := Resolve([]Grant{
		{RoleName: "admin", ScopeType: ScopeGlobal, ScopeID: 0},
		{RoleName: "admin", ScopeType: ScopeRiskTeam, ScopeID: asgardeo, ScopeBasis: BasisSourceRegister, ScopeTeamType: TeamBoth},
	}, resolver)

	if !set.HasGlobal("RISK_VIEW_ANALYTICS") {
		t.Fatal("HasGlobal should be true — this is the caller's real bypass signal")
	}
	if got := set.RegisterScopeIDsFor("RISK_VIEW_ANALYTICS"); len(got) != 1 || got[0] != asgardeo {
		t.Fatalf("RegisterScopeIDsFor = %v, want [%d] — GLOBAL is not itself a team id", got, asgardeo)
	}
}
