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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// TestHandleUpdateAssignees covers the handler's own gates: who may call it and
// what a well-formed body is. The correction window itself is the repository's
// job and is covered in repository/entity.
func TestHandleUpdateAssignees(t *testing.T) {
	const assignerID = 7

	assignerIn := func(team int) map[int]map[string]bool {
		return map[int]map[string]bool{team: {privilege.ViewRisks: true, privilege.UpdateRisk: true}}
	}
	complianceAdminIn := func(team int) map[int]map[string]bool {
		return map[int]map[string]bool{team: {
			privilege.ViewRisks: true, privilege.UpdateRisk: true, privilege.ComplianceApproveRisk: true,
		}}
	}

	cases := []struct {
		name       string
		callerID   int
		byTeam     map[int]map[string]bool
		body       string
		serviceErr error
		wantStatus int
		wantCalled bool
	}{
		{"named assigner", assignerID, assignerIn(asgardeo), `{"owner_id":21}`, nil, http.StatusNoContent, true},
		{"compliance admin who is not the assigner", 99, complianceAdminIn(asgardeo), `{"owner_id":21}`, nil, http.StatusNoContent, true},
		{"someone else with RISK_UPDATE", 99, assignerIn(asgardeo), `{"owner_id":21}`, nil, http.StatusForbidden, false},
		{"named assigner without RISK_UPDATE", assignerID, ownerIn(asgardeo), `{"owner_id":21}`, nil, http.StatusForbidden, false},
		{"named assigner, RISK_UPDATE only in another register", assignerID, assignerIn(choreo), `{"owner_id":21}`, nil, http.StatusForbidden, false},
		{"empty body", assignerID, assignerIn(asgardeo), `{}`, nil, http.StatusBadRequest, false},
		{"non-positive id", assignerID, assignerIn(asgardeo), `{"owner_id":21,"assignment_team_id":0}`, nil, http.StatusBadRequest, false},
		{
			"window closed", assignerID, assignerIn(asgardeo), `{"owner_id":21}`,
			&apierror.Error{StatusCode: http.StatusConflict, Body: "this risk's assignees can no longer be corrected"},
			http.StatusConflict, true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			risk := &fakeRiskSvc{
				byID: map[int]*model.RiskDetail{
					1: {ID: 1, SourceRegisterID: asgardeo, AssignmentTeamID: choreo, AssignerID: assignerID, OwnerID: 11, ManagementApproverID: 12},
				},
				assigneesErr: c.serviceErr,
			}
			d := &Deps{Risk: risk, Users: fakeUserRepo{uuid: "test-caller-uuid", id: c.callerID}}

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/risks/1/assignees", strings.NewReader(c.body)).
				WithContext(contextForGrants(t, nil, c.byTeam))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("id", "1")
			rec := httptest.NewRecorder()

			d.handleUpdateAssignees(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, c.wantStatus, rec.Body.String())
			}
			if called := risk.assigneesCalls > 0; called != c.wantCalled {
				t.Fatalf("service called = %v, want %v", called, c.wantCalled)
			}
			if c.wantCalled && (risk.lastAssignees.OwnerID == nil || *risk.lastAssignees.OwnerID != 21) {
				t.Errorf("service got owner_id %v, want 21", risk.lastAssignees.OwnerID)
			}
		})
	}
}
