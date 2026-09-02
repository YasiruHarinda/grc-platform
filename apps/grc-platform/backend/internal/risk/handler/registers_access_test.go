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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// stubResolver is a role→privilege map standing in for the privilege.Store.
type stubResolver map[string][]string

func (r stubResolver) Resolve(roles []string) map[string]bool {
	out := map[string]bool{}
	for _, role := range roles {
		for _, p := range r[role] {
			out[p] = true
		}
	}
	return out
}

// contextForRealGrants builds a request context from real grant.Grant rows via
// grant.Resolve — unlike contextForGrants (grant.NewForTest), the resulting Set
// answers AllScopeIDs / scope enumeration, which riskVisibleToCaller and the
// list's team scoping depend on.
func contextForRealGrants(t *testing.T, grants ...grant.Grant) context.Context {
	t.Helper()
	resolver := stubResolver{
		"risk-owner": {privilege.ViewRisks, privilege.OwnerApproveRisk},
	}
	set := grant.Resolve(grants, resolver)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "test-caller-uuid"})
	ctx = grant.WithContext(ctx, set)
	return privilege.WithContext(ctx, set.PrivilegeMap())
}

func ownerGrantOn(registerID int) grant.Grant {
	return grant.Grant{
		RoleName:      "risk-owner",
		ScopeType:     "TEAM",
		ScopeID:       registerID,
		ScopeBasis:    "SOURCE_REGISTER",
		ScopeTeamType: grant.TeamSourceRegister,
	}
}

// These tests pin the identity axis at the HTTP layer: a person named as an
// Action Owner holds no grants and no privileges (see RISK_MODULE_DESIGN.md
// §3), and must still reach the Registers list and the risks they are named
// on. A RISK_VIEW_RISKS route gate used to 403 them before the per-caller
// scoping ran — the regression these guard against.

func intp(n int) *int { return &n }

// fakeRiskSvc implements riskservice.RiskService. Only List and GetByID carry
// behaviour; the workflow methods are unused by the read paths under test.
type fakeRiskSvc struct {
	page       *model.RiskListPage
	byID       map[int]*model.RiskDetail
	lastFilter model.ListRisksFilter
}

func (f *fakeRiskSvc) List(_ context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error) {
	f.lastFilter = filter
	if f.page != nil {
		return f.page, nil
	}
	return &model.RiskListPage{Items: []*model.RiskListItem{}}, nil
}

func (f *fakeRiskSvc) GetByID(_ context.Context, id int) (*model.RiskDetail, error) {
	return f.byID[id], nil
}

func (f *fakeRiskSvc) Create(context.Context, model.CreateRiskRequest, string) (*model.CreateRiskResponse, error) {
	return nil, nil
}
func (f *fakeRiskSvc) NextSequenceID(context.Context, int) (int, error) { return 0, nil }
func (f *fakeRiskSvc) Update(context.Context, int, model.UpdateRiskRequest, string) error {
	return nil
}
func (f *fakeRiskSvc) OwnerApprove(context.Context, int, string) error { return nil }
func (f *fakeRiskSvc) ManagementApprove(context.Context, int, string, *int, bool) error {
	return nil
}
func (f *fakeRiskSvc) Approve(context.Context, int, string) error { return nil }
func (f *fakeRiskSvc) Reject(context.Context, int, model.RejectRiskRequest, string, string) error {
	return nil
}
func (f *fakeRiskSvc) Complete(context.Context, int, string) error { return nil }
func (f *fakeRiskSvc) Resubmit(context.Context, int, string) error { return nil }
func (f *fakeRiskSvc) Close(context.Context, int, string) error    { return nil }
func (f *fakeRiskSvc) Cancel(context.Context, int, string) error   { return nil }

// fakeActionPlanSvc implements riskservice.ActionPlanService. List and GetByID
// carry behaviour; the rest are unused by the read paths under test.
type fakeActionPlanSvc struct {
	plans []*model.ActionPlan
}

func (f *fakeActionPlanSvc) List(_ context.Context, riskID int) ([]*model.ActionPlan, error) {
	out := []*model.ActionPlan{}
	for _, p := range f.plans {
		if p.RiskID == riskID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeActionPlanSvc) GetByID(_ context.Context, _, planID int) (*model.ActionPlan, error) {
	for _, p := range f.plans {
		if p.ID == planID {
			return p, nil
		}
	}
	return nil, nil
}
func (f *fakeActionPlanSvc) Create(context.Context, int, model.CreateActionPlanRequest, string) (*model.ActionPlan, error) {
	return nil, nil
}
func (f *fakeActionPlanSvc) ListSteps(context.Context, int) ([]*model.ActionPlanStep, error) {
	return []*model.ActionPlanStep{}, nil
}
func (f *fakeActionPlanSvc) UpdateStep(context.Context, int, int, int, model.UpdateActionPlanStepRequest, string, bool) error {
	return nil
}
func (f *fakeActionPlanSvc) Complete(context.Context, int, int, string, bool) (*model.ActionPlan, error) {
	return nil, nil
}

// TestHandleListRisks_ActionOwnerNoGrants is the bug in one test: a caller with
// no grants, named as an Action Owner (their uuid resolves to user id 7), gets
// a 200 whose result is scoped to action_owner_id = 7 — not a 403.
func TestHandleListRisks_ActionOwnerNoGrants(t *testing.T) {
	risk := &fakeRiskSvc{page: &model.RiskListPage{
		Items: []*model.RiskListItem{{ID: 1, RiskCode: "R-1"}},
		Total: 1,
	}}
	d := &Deps{
		Risk:  risk,
		Users: fakeUserRepo{uuid: "test-caller-uuid", id: 7},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil).
		WithContext(contextForGrants(t, nil, nil)) // no grants at all
	rec := httptest.NewRecorder()

	d.handleListRisks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (identity-axis Action Owner must reach the list)", rec.Code)
	}
	if risk.lastFilter.ActionOwnerID == nil || *risk.lastFilter.ActionOwnerID != 7 {
		t.Errorf("list filter ActionOwnerID = %v, want 7 — result was not scoped to the caller", risk.lastFilter.ActionOwnerID)
	}
}

// TestHandleListRisks_NoGrantsNoUserRow: authenticated, no grants, and the uuid
// resolves to no platform user row. That is an empty page (the truthful scoped
// result), never a 403 and never an unscoped list.
func TestHandleListRisks_NoGrantsNoUserRow(t *testing.T) {
	risk := &fakeRiskSvc{}
	d := &Deps{
		Risk:  risk,
		Users: fakeUserRepo{uuid: "somebody-else"}, // caller's uuid resolves to nothing
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil).
		WithContext(contextForGrants(t, nil, nil))
	rec := httptest.NewRecorder()

	d.handleListRisks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// TestHandleListRisks_TeamScoped: a caller holding a grant on one register (no
// GLOBAL grant) gets a 200 scoped to that register — the isTeamScopedOnly
// branch, which also used to sit behind the removed route gate.
func TestHandleListRisks_TeamScoped(t *testing.T) {
	risk := &fakeRiskSvc{}
	d := &Deps{Risk: risk, Users: fakeUserRepo{uuid: "test-caller-uuid", id: 7}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil).
		WithContext(contextForRealGrants(t, ownerGrantOn(asgardeo)))
	rec := httptest.NewRecorder()

	d.handleListRisks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !containsInt(risk.lastFilter.ScopeSourceRegisterIDs, asgardeo) {
		t.Errorf("ScopeSourceRegisterIDs = %v, want to contain %d", risk.lastFilter.ScopeSourceRegisterIDs, asgardeo)
	}
}

func containsInt(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestHandleGetRisk_IdentityAxis: a no-grant caller may open a risk they are
// named on as Action Owner (200), and is 404'd — not 403'd — on one they are
// neither named on nor hold a grant for.
func TestHandleGetRisk_IdentityAxis(t *testing.T) {
	const callerID = 7
	risk := &fakeRiskSvc{byID: map[int]*model.RiskDetail{
		1: {ID: 1, SourceRegisterID: asgardeo, AssignmentTeamID: choreo, OwnerID: 99, AssignerID: 98, ManagementApproverID: 97},
		2: {ID: 2, SourceRegisterID: asgardeo, AssignmentTeamID: choreo, OwnerID: 99, AssignerID: 98, ManagementApproverID: 97},
	}}
	plans := &fakeActionPlanSvc{plans: []*model.ActionPlan{
		{ID: 10, RiskID: 1, ActionOwnerID: intp(callerID)}, // caller owns a plan on risk 1 only
	}}
	d := &Deps{Risk: risk, ActionPlan: plans, Users: fakeUserRepo{uuid: "test-caller-uuid", id: callerID}}

	t.Run("named on the risk -> 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/1", nil).
			WithContext(contextForGrants(t, nil, nil))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()

		d.handleGetRisk(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("not named, no grant -> 404 not 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/2", nil).
			WithContext(contextForGrants(t, nil, nil))
		req.SetPathValue("id", "2")
		rec := httptest.NewRecorder()

		d.handleGetRisk(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

// TestHandleGetRisk_GrantOnRegister: a caller holding any grant on the risk's
// source register sees it, with no identity relationship required.
func TestHandleGetRisk_GrantOnRegister(t *testing.T) {
	risk := &fakeRiskSvc{byID: map[int]*model.RiskDetail{
		1: {ID: 1, SourceRegisterID: asgardeo, AssignmentTeamID: choreo, OwnerID: 99, AssignerID: 98, ManagementApproverID: 97},
	}}
	d := &Deps{Risk: risk, ActionPlan: &fakeActionPlanSvc{}, Users: fakeUserRepo{uuid: "test-caller-uuid", id: 7}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/1", nil).
		WithContext(contextForRealGrants(t, ownerGrantOn(asgardeo)))
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	d.handleGetRisk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// TestHandleListActionPlanSteps_RequiresVisibility guards the riskVisibleToCaller
// check added when the route privilege gate was removed: a no-grant caller not
// named on the risk cannot read an arbitrary plan's steps.
func TestHandleListActionPlanSteps_RequiresVisibility(t *testing.T) {
	risk := &fakeRiskSvc{byID: map[int]*model.RiskDetail{
		5: {ID: 5, SourceRegisterID: asgardeo, AssignmentTeamID: choreo, OwnerID: 99, AssignerID: 98, ManagementApproverID: 97},
	}}
	plans := &fakeActionPlanSvc{plans: []*model.ActionPlan{
		{ID: 50, RiskID: 5, ActionOwnerID: intp(42)}, // owned by someone else
	}}
	d := &Deps{Risk: risk, ActionPlan: plans, Users: fakeUserRepo{uuid: "test-caller-uuid", id: 7}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/5/action-plans/50/steps", nil).
		WithContext(contextForGrants(t, nil, nil))
	req.SetPathValue("id", "5")
	req.SetPathValue("planId", "50")
	rec := httptest.NewRecorder()

	d.handleListActionPlanSteps(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (caller neither named nor granted)", rec.Code)
	}
}
