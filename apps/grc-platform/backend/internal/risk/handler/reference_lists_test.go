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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// fakeCategoryService, fakeScoreService and fakeComplianceService each
// implement only List — the read path these regression tests exercise. The
// other methods panic since these tests never reach them.
type fakeCategoryService struct{ cats []*model.RiskCategory }

func (f *fakeCategoryService) List(context.Context) ([]*model.RiskCategory, error) {
	return f.cats, nil
}
func (f *fakeCategoryService) Create(context.Context, model.CreateRiskCategoryRequest, string) (*model.RiskCategory, error) {
	panic("not needed by this test")
}
func (f *fakeCategoryService) Update(context.Context, int, model.UpdateRiskCategoryRequest, string) (*model.RiskCategory, error) {
	panic("not needed by this test")
}
func (f *fakeCategoryService) Delete(context.Context, int) error {
	panic("not needed by this test")
}

type fakeScoreService struct{ scores []*model.RiskScore }

func (f *fakeScoreService) List(context.Context) ([]*model.RiskScore, error) { return f.scores, nil }
func (f *fakeScoreService) Create(context.Context, model.CreateRiskScoreRequest, string) (*model.RiskScore, error) {
	panic("not needed by this test")
}
func (f *fakeScoreService) Update(context.Context, int, model.UpdateRiskScoreRequest, string) error {
	panic("not needed by this test")
}

type fakeComplianceService struct{ refs []*model.ComplianceReference }

func (f *fakeComplianceService) List(context.Context) ([]*model.ComplianceReference, error) {
	return f.refs, nil
}
func (f *fakeComplianceService) Create(context.Context, model.CreateComplianceRefRequest, string) (*model.ComplianceReference, error) {
	panic("not needed by this test")
}
func (f *fakeComplianceService) Update(context.Context, int, model.UpdateComplianceRefRequest, string) (*model.ComplianceReference, error) {
	panic("not needed by this test")
}
func (f *fakeComplianceService) Delete(context.Context, int) error {
	panic("not needed by this test")
}

// TestListReferenceDataRequiresPrivilege is the regression test for the
// review follow-up on the teams/users privilege-gate fix: risk-categories,
// risk-scores and compliance-references were the other three Admin Console
// reference-data lists left ungated, letting any authenticated caller — an
// external auditor included — enumerate them despite their write
// counterparts (where they exist) already requiring ManageRiskHub.
func TestListReferenceDataRequiresPrivilege(t *testing.T) {
	noRiskHubPrivilege := contextForGrants(t, map[string]bool{privilege.ViewAudits: true}, nil)

	tests := []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
		path    string
	}{
		{
			name: "risk categories",
			path: "/api/v1/risk-categories",
			handler: (&Deps{Category: &fakeCategoryService{
				cats: []*model.RiskCategory{{ID: 1, Name: "Operational"}},
			}}).handleListRiskCategories,
		},
		{
			name: "risk scores",
			path: "/api/v1/risk-scores",
			handler: (&Deps{Score: &fakeScoreService{
				scores: []*model.RiskScore{{ID: 1}},
			}}).handleListRiskScores,
		},
		{
			name: "compliance references",
			path: "/api/v1/compliance-references",
			handler: (&Deps{Compliance: &fakeComplianceService{
				refs: []*model.ComplianceReference{{ID: 1, Name: "ISO 27001"}},
			}}).handleListComplianceReferences,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req = req.WithContext(noRiskHubPrivilege)
			rec := httptest.NewRecorder()

			tt.handler(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status = %d, want %d (body: %s)", tt.path, rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

// TestListReferenceDataAllowsViewRisks confirms the gate doesn't lock out
// legitimate Risk Hub callers — every seeded Risk role carries ViewRisks
// (see Resources/shared_seed_data.sql), which is what the Add Risk form
// pickers (riskApi.ts) rely on to reach these same three endpoints.
func TestListReferenceDataAllowsViewRisks(t *testing.T) {
	viewRisks := contextForGrants(t, map[string]bool{privilege.ViewRisks: true}, nil)

	tests := []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
		path    string
	}{
		{
			name: "risk categories",
			path: "/api/v1/risk-categories",
			handler: (&Deps{Category: &fakeCategoryService{
				cats: []*model.RiskCategory{{ID: 1, Name: "Operational"}},
			}}).handleListRiskCategories,
		},
		{
			name: "risk scores",
			path: "/api/v1/risk-scores",
			handler: (&Deps{Score: &fakeScoreService{
				scores: []*model.RiskScore{{ID: 1}},
			}}).handleListRiskScores,
		},
		{
			name: "compliance references",
			path: "/api/v1/compliance-references",
			handler: (&Deps{Compliance: &fakeComplianceService{
				refs: []*model.ComplianceReference{{ID: 1, Name: "ISO 27001"}},
			}}).handleListComplianceReferences,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req = req.WithContext(viewRisks)
			rec := httptest.NewRecorder()

			tt.handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, want %d (body: %s)", tt.path, rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}
