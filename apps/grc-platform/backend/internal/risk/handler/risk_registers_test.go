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

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// contextForGrants builds a context the way the Auth middleware would: a
// resolved grant Set plus the union published under the privilege key.
//
// global and byTeam are privilege sets, already resolved — the role→privilege
// indirection is exercised elsewhere and only obscures what these tests are
// about, which is WHERE a privilege applies rather than which role carries it.
func contextForGrants(t *testing.T, global map[string]bool, byTeam map[int]map[string]bool) context.Context {
	t.Helper()
	grantCount := 0
	if len(global) > 0 {
		grantCount++
	}
	grantCount += len(byTeam)

	set := grant.NewForTest(global, byTeam, grantCount)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "test-caller-uuid"})
	ctx = grant.WithContext(ctx, set)
	return privilege.WithContext(ctx, set.PrivilegeMap())
}

// Register ids used throughout.
const (
	asgardeo = 1
	choreo   = 2
)

// ownerIn / complianceIn build the grant shapes these tests need, scoped to
// one register.
func ownerIn(team int) map[int]map[string]bool {
	return map[int]map[string]bool{team: {privilege.ViewRisks: true, privilege.OwnerApproveRisk: true}}
}
func complianceIn(team int) map[int]map[string]bool {
	return map[int]map[string]bool{team: {privilege.ViewRisks: true, privilege.ComplianceApproveRisk: true}}
}

// TestCallerClassification covers the three questions that used to be answered
// by hand-maintained privilege allowlists (seesEveryRisk, isTeamScopedOnly,
// isActionOwnerOnly). They are now read straight off the grants, so no test
// needs updating when a privilege is added — which was the failure mode of the
// versions these replace.
func TestCallerClassification(t *testing.T) {
	cases := []struct {
		name                            string
		global                          map[string]bool
		byTeam                          map[int]map[string]bool
		wantEvery, wantScoped, wantNone bool
	}{
		{
			// A real risk role always carries ViewRisks; that is the privilege
			// "sees every risk" is actually about.
			name:      "GLOBAL grant sees every risk",
			global:    map[string]bool{privilege.ViewRisks: true, privilege.ComplianceApproveRisk: true},
			wantEvery: true,
		},
		{
			// REGRESSION: a platform admin holds MANAGE_USERS globally and
			// nothing else. Treating "holds some GLOBAL grant" as unrestricted
			// handed them every risk in the system as soon as any second, narrow
			// grant carried them past the route gate.
			name:       "GLOBAL grant of a non-risk role does NOT see every risk",
			global:     map[string]bool{privilege.ManageUsers: true},
			byTeam:     ownerIn(asgardeo),
			wantScoped: true,
		},
		{
			name:       "team-scoped grant is scoped",
			byTeam:     ownerIn(asgardeo),
			wantScoped: true,
		},
		{
			name:       "grants in two registers is still scoped",
			byTeam:     map[int]map[string]bool{asgardeo: {privilege.OwnerApproveRisk: true}, choreo: {privilege.CreateRisk: true}},
			wantScoped: true,
		},
		{
			// An Action Owner may be any employee, holding no role at all.
			name:     "no grants at all",
			wantNone: true,
		},
		{
			// A GLOBAL grant wins: they see everything, so they are neither
			// team-scoped nor grant-less, however many team grants they also hold.
			name:      "GLOBAL plus team grants is not team-scoped",
			global:    map[string]bool{privilege.ViewRisks: true, privilege.ComplianceApproveRisk: true},
			byTeam:    ownerIn(asgardeo),
			wantEvery: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := contextForGrants(t, c.global, c.byTeam)
			if got := seesEveryRisk(ctx); got != c.wantEvery {
				t.Errorf("seesEveryRisk = %v, want %v", got, c.wantEvery)
			}
			if got := isTeamScopedOnly(ctx); got != c.wantScoped {
				t.Errorf("isTeamScopedOnly = %v, want %v", got, c.wantScoped)
			}
			if got := holdsNoGrants(ctx); got != c.wantNone {
				t.Errorf("holdsNoGrants = %v, want %v", got, c.wantNone)
			}
			// The three are mutually exclusive and exhaustive, which the
			// allowlist versions could not guarantee — they could both be true
			// for a caller holding an unlisted privilege combination.
			n := 0
			for _, b := range []bool{c.wantEvery, c.wantScoped, c.wantNone} {
				if b {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("test case is malformed: exactly one classification must hold")
			}
		})
	}
}

// TestCanOverrideAssigneeIsScoped is the security case that motivated the whole
// migration. The override bypasses every per-risk identity gate, so a compliance
// approver scoped to one register must not be able to use it in another.
//
// On the unscoped union this test cannot fail, which is exactly why it exists.
func TestCanOverrideAssigneeIsScoped(t *testing.T) {
	ctx := contextForGrants(t, nil, complianceIn(asgardeo))

	if !canOverrideAssigneeIn(ctx, asgardeo) {
		t.Error("compliance approver should be able to override in their own register")
	}
	if canOverrideAssigneeIn(ctx, choreo) {
		t.Error("SECURITY: overrode in a register where they hold no grant")
	}

	// A GLOBAL compliance admin overrides anywhere, including registers created
	// after the grant was made.
	global := contextForGrants(t, map[string]bool{privilege.ComplianceApproveRisk: true}, nil)
	for _, team := range []int{asgardeo, choreo, 9999} {
		if !canOverrideAssigneeIn(global, team) {
			t.Errorf("GLOBAL compliance admin should override in register %d", team)
		}
	}

	// Holding a different privilege is never enough.
	owner := contextForGrants(t, nil, ownerIn(asgardeo))
	if canOverrideAssigneeIn(owner, asgardeo) {
		t.Error("a risk owner must not be able to override the identity gate")
	}
}

// TestRolesDoNotMergeAcrossRegisters is the requirement in one test: one user,
// Risk Owner in Asgardeo and Risk Assigner in Choreo, must hold each power only
// where it was granted.
func TestRolesDoNotMergeAcrossRegisters(t *testing.T) {
	ctx := contextForGrants(t, nil, map[int]map[string]bool{
		asgardeo: {privilege.OwnerApproveRisk: true},
		choreo:   {privilege.CreateRisk: true},
	})
	set := auth.Grants(ctx)

	if !set.HasIn(privilege.OwnerApproveRisk, asgardeo) {
		t.Error("should hold OwnerApprove in Asgardeo")
	}
	if set.HasIn(privilege.OwnerApproveRisk, choreo) {
		t.Error("SECURITY: OwnerApprove leaked into Choreo")
	}
	if !set.HasIn(privilege.CreateRisk, choreo) {
		t.Error("should hold CreateRisk in Choreo")
	}
	if set.HasIn(privilege.CreateRisk, asgardeo) {
		t.Error("SECURITY: CreateRisk leaked into Asgardeo")
	}
	// The union — published via PrivilegeMap for the privilege-context bridge —
	// still says yes to both, which is why it must never be the enforcement on
	// a per-risk action.
	union := set.PrivilegeMap()
	if !union[privilege.OwnerApproveRisk] || !union[privilege.CreateRisk] {
		t.Error("the union should contain both privileges")
	}
}

// fakeUserRepo resolves exactly one uuid to one id, so requireRiskActor and
// describeActor can be exercised without the Compliance Entity. Every other
// method is unused here.
type fakeUserRepo struct {
	uuid        string
	id          int
	email       string
	displayName string
	err         error
}

func (f fakeUserRepo) GetByUUID(_ context.Context, uuid string) (*user.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if uuid != f.uuid {
		return nil, nil // "not found" is a domain condition, not an error
	}
	return &user.User{ID: f.id, Email: f.email, DisplayName: f.displayName}, nil
}
func (f fakeUserRepo) GetByID(context.Context, int) (*user.User, error) { return nil, nil }
func (f fakeUserRepo) Upsert(context.Context, string, string) (*user.User, error) {
	return nil, nil
}
func (f fakeUserRepo) UpsertTyped(context.Context, string, string, string) (*user.User, error) {
	return nil, nil
}
func (f fakeUserRepo) UpdateStatus(context.Context, int, string, string) (*user.User, error) {
	return nil, nil
}
func (f fakeUserRepo) List(context.Context) ([]*user.User, error) { return nil, nil }

// The role-name-keyed TestCanOverrideAssignee that lived here is superseded by
// TestCanOverrideAssigneeIsScoped above, which asks the question that now
// matters: not "which role may override" but "override WHERE".

func TestRequireRiskActor(t *testing.T) {
	const callerUUID = "test-caller-uuid" // the uuid contextForGrants puts on the request
	cases := []struct {
		name       string
		byTeam     map[int]map[string]bool
		callerID   int // id the caller's uuid resolves to; 0 = no platform user row
		wantUserID int // the id named on the risk
		wantOK     bool
		wantStatus int
	}{
		{"named actor passes", ownerIn(asgardeo), 7, 7, true, http.StatusOK},
		{"different user is refused", ownerIn(asgardeo), 7, 9, false, http.StatusForbidden},
		// The override is the whole point of the compliance-admin escape hatch:
		// a risk whose named owner has left must not deadlock.
		{"compliance admin overrides a mismatch", complianceIn(asgardeo), 7, 9, true, http.StatusOK},
		// ...but only in the register they hold it in. A compliance approver
		// scoped to Choreo gets no override on an Asgardeo risk.
		{"compliance admin cannot override in another register", complianceIn(choreo), 7, 9, false, http.StatusForbidden},
		// No platform user row can never match, so it must refuse rather than
		// fall through — nil is not "any user".
		{"caller with no user row is refused", ownerIn(asgardeo), 0, 9, false, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := fakeUserRepo{uuid: callerUUID, id: c.callerID}
			if c.callerID == 0 {
				repo.uuid = "someone-elses-uuid" // caller's uuid resolves to nothing
			}
			d := &Deps{Users: repo}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/1/owner-approve", nil).
				WithContext(contextForGrants(t, nil, c.byTeam))
			rec := httptest.NewRecorder()

			// The risk under test is sourced in Asgardeo.
			got := d.requireRiskActor(rec, req, c.wantUserID, asgardeo, "Risk Owner")
			if got != c.wantOK {
				t.Errorf("requireRiskActor = %v, want %v", got, c.wantOK)
			}
			if !c.wantOK && rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}
			if c.wantOK && rec.Code != http.StatusOK {
				t.Errorf("passing check wrote status %d, want nothing written", rec.Code)
			}
		})
	}
}

// fakeSCIMServer stands in for Asgardeo's SCIM2 API, resolving exactly one
// uuid (if any) to a name/email — enough to drive describeActor's formatting
// logic through its cases without duplicating internal/directory's own, more
// thorough coverage of the cache/staleness mechanics themselves.
func fakeSCIMServer(t *testing.T, uuid, givenName, familyName, email string) *directory.Service {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("POST /t/wso2/scim2/Users/.search", func(w http.ResponseWriter, r *http.Request) {
		resources := []map[string]any{}
		if uuid != "" {
			resources = append(resources, map[string]any{
				"id": uuid, "userName": email,
				"name": map[string]any{"givenName": givenName, "familyName": familyName},
			})
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Resources": resources})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := scim.NewClient(srv.URL, srv.URL+"/oauth2/token", "id", "secret", "scope", "wso2")
	return directory.New(c, time.Hour)
}

// describeActor decides how the person who triggered a notification appears
// in the email. Every failure path must still yield something sendable — a
// cosmetic lookup is never worth losing a notification over. The identity
// directory is the only source now (see resolvePerson) — no stored fallback
// left to fall through to.
func TestDescribeActor(t *testing.T) {
	const actorUUID = "actor-uuid"
	cases := []struct {
		name string
		dir  *directory.Service
		in   string
		want string
	}{
		{
			"name and email when resolvable",
			fakeSCIMServer(t, actorUUID, "Ruwan", "Silva", "ruwan@wso2.com"),
			actorUUID,
			"Ruwan Silva (ruwan@wso2.com)",
		},
		{
			// Resolution failed (uuid unknown to the directory — the fake
			// server's own uuid is empty, so it answers every search with no
			// resources, regardless of what was queried): the raw uuid is the
			// fallback, not blank — the daily escalation job's "system"
			// sentinel actor depends on exactly this (see escalation_job.go).
			"raw uuid when the directory doesn't know them",
			fakeSCIMServer(t, "", "", "", ""),
			actorUUID,
			actorUUID,
		},
		{
			"bare email when the directory has no name on file",
			fakeSCIMServer(t, actorUUID, "", "", "ruwan@wso2.com"),
			actorUUID,
			"ruwan@wso2.com",
		},
		{
			"raw uuid when the directory is unconfigured",
			nil,
			actorUUID,
			actorUUID,
		},
		{"empty in, empty out", nil, "", ""},
		{"whitespace is trimmed", nil, "  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &Deps{Directory: c.dir}
			if got := d.describeActor(context.Background(), c.in); got != c.want {
				t.Errorf("describeActor(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// errStub is a shared stub error for tests across this package that need a
// distinguishable non-nil error without caring about its content.
var errStub = errors.New("entity unavailable")

// TestLocalDevAllowAllClassification guards the local-dev mode
// (AUTH_TOKEN_VALIDATOR_ENABLED=false), where no privilege store is configured
// and every privilege check is meant to pass.
//
// The classification helpers read the grant set directly rather than going
// through auth.HasPrivilege, so they must honour that mode themselves —
// otherwise a local developer is authenticated, allowed past every route gate,
// and then scoped to nothing: an empty dashboard and a risk list filtered to
// rows they happen to be the action owner of.
func TestLocalDevAllowAllClassification(t *testing.T) {
	// Exactly what the middleware leaves behind in that mode: a user, but no
	// privilege map and no grant set.
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Email: "dev@wso2.com"})

	if !seesEveryRisk(ctx) {
		t.Error("local dev must see every risk — the mode allows every privilege check")
	}
	if isTeamScopedOnly(ctx) {
		t.Error("local dev must not be team-scoped")
	}
	if holdsNoGrants(ctx) {
		t.Error("local dev must not be treated as a caller with no grants")
	}
}
