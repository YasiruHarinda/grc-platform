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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// fakeTeamService returns a fixed team list, ignoring the filter — these
// tests are about the caller-scoping in handleListTeams, not the DB-side type
// filter.
type fakeTeamService struct{ teams []*model.Team }

func (f *fakeTeamService) List(context.Context, model.ListTeamsFilter) ([]*model.Team, error) {
	return f.teams, nil
}

func (f *fakeTeamService) Create(context.Context, model.CreateTeamRequest, string) (*model.Team, error) {
	panic("not needed by this test")
}

func (f *fakeTeamService) Update(context.Context, int, model.UpdateTeamRequest, string) error {
	panic("not needed by this test")
}

// TestListTeamsPrivilegeFilterAppliesEvenWhenCallerSeesEveryRisk reproduces a
// live bug: a caller holding RISK_VIEW_RISKS GLOBAL (e.g. the Management
// role) but RISK_CREATE scoped to only one register asked
// GET /teams?mine=true&privilege=RISK_CREATE (the Add Risk source-register
// picker) and got back every register, not just the one they can actually
// create in — because the old code skipped the ?privilege= filter entirely
// whenever seesEveryRisk(ctx) was true, conflating "may view every risk" with
// "may create everywhere".
func TestListTeamsPrivilegeFilterAppliesEvenWhenCallerSeesEveryRisk(t *testing.T) {
	ctx := contextForGrants(t,
		map[string]bool{privilege.ViewRisks: true},                      // GLOBAL — makes seesEveryRisk true
		map[int]map[string]bool{asgardeo: {privilege.CreateRisk: true}}, // RISK_CREATE only on Asgardeo
	)

	d := &Deps{Team: &fakeTeamService{teams: []*model.Team{
		{ID: asgardeo, Name: "Asgardeo", TeamType: "BOTH", Status: "ACTIVE"},
		{ID: choreo, Name: "Choreo", TeamType: "BOTH", Status: "ACTIVE"},
	}}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/teams?type=SOURCE_REGISTER&mine=true&privilege="+privilege.CreateRisk, nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.handleListTeams(rec, req)

	var got []*model.Team
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].ID != asgardeo {
		t.Fatalf("handleListTeams(mine=true&privilege=RISK_CREATE) = %+v, want only Asgardeo (id %d) — "+
			"a global ViewRisks grant must not bypass the RISK_CREATE scope check", got, asgardeo)
	}
}

// TestListTeamsRequiresPrivilege guards against a caller with no Risk Hub
// privilege at all — an external auditor, or any authenticated user with no
// grant in this module — enumerating the risk register/org-team structure.
func TestListTeamsRequiresPrivilege(t *testing.T) {
	ctx := contextForGrants(t, map[string]bool{privilege.ViewAudits: true}, nil)

	d := &Deps{Team: &fakeTeamService{teams: []*model.Team{
		{ID: asgardeo, Name: "Asgardeo", TeamType: "BOTH", Status: "ACTIVE"},
	}}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/teams", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.handleListTeams(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("handleListTeams() with no Risk Hub privilege = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// TestListTeamsIncludeInactiveRequiresManageRiskHub reproduces the audit-teams
// precedent (audit/handler/team.go): a caller who can see the risk team list
// (ViewRisks) but doesn't administer it (no ManageRiskHub) must not be able to
// pull inactive teams too, just by adding ?includeInactive=true.
func TestListTeamsIncludeInactiveRequiresManageRiskHub(t *testing.T) {
	ctx := contextForGrants(t, map[string]bool{privilege.ViewRisks: true}, nil)

	d := &Deps{Team: &fakeTeamService{teams: []*model.Team{
		{ID: asgardeo, Name: "Asgardeo", TeamType: "BOTH", Status: "INACTIVE"},
	}}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/teams?includeInactive=true", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.handleListTeams(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("handleListTeams(includeInactive=true) with ViewRisks but no ManageRiskHub = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
