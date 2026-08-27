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
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// TestListCandidatesRequiresPrivilege is the regression test for the review
// follow-up on the teams/users privilege-gate fix: the three role-gated
// pickers (risk/management-approvers, risk/owner-candidates, risk/assigner-candidates)
// were left ungated, letting any authenticated caller — an external auditor
// included — enumerate every internal user holding the picked privilege, with
// resolved name, email and uuid, and sweep teamId to map who holds authority
// in which register. All three share handleListCandidates, so its guard is
// what closes the hole.
//
// Deps is empty on purpose: the guard must reject before touching Grants,
// Users or Directory.
func TestListCandidatesRequiresPrivilege(t *testing.T) {
	noRiskPrivilege := contextForGrants(t, map[string]bool{privilege.ViewAudits: true}, nil)

	tests := []struct {
		name    string
		path    string
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:    "management approvers",
			path:    "/api/v1/risk/management-approvers",
			handler: (&Deps{}).handleListManagementApprovers,
		},
		{
			name:    "risk owner candidates",
			path:    "/api/v1/risk/owner-candidates",
			handler: (&Deps{}).handleListRiskOwnerCandidates,
		},
		{
			name:    "risk assigner candidates",
			path:    "/api/v1/risk/assigner-candidates",
			handler: (&Deps{}).handleListRiskAssignerCandidates,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req = req.WithContext(noRiskPrivilege)
			rec := httptest.NewRecorder()

			tt.handler(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status = %d, want %d (body: %s)", tt.path, rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}
