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

package entity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// fakeRiskEntity serves one risk's detail and records the PATCH it receives.
type fakeRiskEntity struct {
	status    string
	createdBy string
	createdOn time.Time
	patch     map[string]any // nil until a PATCH arrives
}

func (f *fakeRiskEntity) serve(t *testing.T) *riskRepository {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/risks/7/detail":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                   7,
				"riskTitle":            "Title",
				"riskDescription":      "Description",
				"workflowStatus":       f.status,
				"createdBy":            f.createdBy,
				"createdOn":            f.createdOn,
				"assignerId":           10,
				"ownerId":              11,
				"managementApproverId": 12,
				"assignmentTeamId":     3,
				"sourceRegisterId":     2,
				"emailSubject":         "Subject",
				"actionPlan":           map[string]any{"id": 5, "actionOwnerId": 13, "status": "COMPLETED", "planType": "STANDARD"},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/risks/7":
			if err := json.NewDecoder(r.Body).Decode(&f.patch); err != nil {
				t.Errorf("decode patch body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &riskRepository{c: entityclient.New(srv.URL)}
}

func intPtr(v int) *int { return &v }

// correction is what the Update Assignees dialog sends: the five fields plus
// the echoed required fields.
func correction() model.UpdateRiskRequest {
	return model.UpdateRiskRequest{
		RiskTitle:            "Title",
		RiskDescription:      "Description",
		EmailSubject:         "Subject",
		AssignerID:           intPtr(10), // unchanged
		OwnerID:              intPtr(21),
		ManagementApproverID: intPtr(12), // unchanged
		AssignmentTeamID:     intPtr(4),
		ActionOwnerID:        intPtr(23),
	}
}

func changedFields(t *testing.T, patch map[string]any) []string {
	t.Helper()
	raw, _ := patch["changeLog"].([]any)
	var fields []string
	for _, e := range raw {
		fields = append(fields, e.(map[string]any)["fieldChanged"].(string))
	}
	sort.Strings(fields)
	return fields
}

func TestGetByIDAssigneesEditableUntil(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour).UTC().Truncate(time.Second)

	tests := []struct {
		name      string
		createdBy string
		createdOn time.Time
		want      string
	}{
		{"migrated, inside the window", model.MigrationMarker, created, created.Add(model.AssigneeCorrectionWindow).Format(time.RFC3339)},
		{"migrated, window closed", model.MigrationMarker, time.Now().Add(-15 * 24 * time.Hour), ""},
		{"created by a user", "3f1c9a2e-user-uuid", created, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRiskEntity{status: model.StatusInRemediation, createdBy: tc.createdBy, createdOn: tc.createdOn}
			d, err := f.serve(t).GetByID(context.Background(), 7)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			got := ""
			if d.AssigneesEditableUntil != nil {
				got = *d.AssigneesEditableUntil
			}
			if got != tc.want {
				t.Errorf("assignees_editable_until = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUpdateClosedMigratedRiskInsideWindowAppliesOnlyAssignees(t *testing.T) {
	f := &fakeRiskEntity{status: model.StatusClosed, createdBy: model.MigrationMarker, createdOn: time.Now().Add(-time.Hour)}
	repo := f.serve(t)

	req := correction()
	// Everything outside the five fields must be dropped on a closed risk.
	req.RiskTitle = "Rewritten title"
	req.ImplementationDate = "2027-01-01"
	req.Remarks = "should not land"

	if err := repo.Update(context.Background(), 7, req, "editor-uuid"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.patch == nil {
		t.Fatal("no PATCH sent")
	}

	var keys []string
	for k := range f.patch {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"actionPlan", "assignerId", "assignmentTeamId", "changeLog", "expectedStatus", "managementApproverId", "ownerId", "updatedBy"}
	if len(keys) != len(want) {
		t.Fatalf("PATCH keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("PATCH keys = %v, want %v", keys, want)
		}
	}
	if f.patch["expectedStatus"] != model.StatusClosed {
		t.Errorf("expectedStatus = %v, want CLOSED", f.patch["expectedStatus"])
	}
	if plan := f.patch["actionPlan"].(map[string]any); len(plan) != 1 || plan["actionOwnerId"] != float64(23) {
		t.Errorf("actionPlan = %v, want only actionOwnerId 23", plan)
	}

	gotFields := changedFields(t, f.patch)
	wantFields := []string{"action_owner_id", "assignment_team_id", "owner_id"}
	if len(gotFields) != len(wantFields) {
		t.Fatalf("changeLog fields = %v, want %v", gotFields, wantFields)
	}
	for i := range wantFields {
		if gotFields[i] != wantFields[i] {
			t.Fatalf("changeLog fields = %v, want %v", gotFields, wantFields)
		}
	}
}

func TestUpdateClosedRiskOutsideWindowIsRejected(t *testing.T) {
	tests := []struct {
		name      string
		createdBy string
		createdOn time.Time
	}{
		{"migrated, window closed", model.MigrationMarker, time.Now().Add(-15 * 24 * time.Hour)},
		{"created by a user", "3f1c9a2e-user-uuid", time.Now().Add(-time.Hour)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRiskEntity{status: model.StatusClosed, createdBy: tc.createdBy, createdOn: tc.createdOn}
			err := f.serve(t).Update(context.Background(), 7, correction(), "editor-uuid")

			var apiErr *apierror.Error
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
				t.Fatalf("err = %v, want a 409", err)
			}
			if f.patch != nil {
				t.Errorf("PATCH was sent: %v", f.patch)
			}
		})
	}
}

func TestUpdateOpenRiskLogsAssigneeChangesWithoutReapproval(t *testing.T) {
	f := &fakeRiskEntity{status: model.StatusInRemediation, createdBy: model.MigrationMarker, createdOn: time.Now().Add(-time.Hour)}
	if err := f.serve(t).Update(context.Background(), 7, correction(), "editor-uuid"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, ok := f.patch["workflowStatus"]; ok {
		t.Errorf("assignee correction moved the workflow: %v", f.patch["workflowStatus"])
	}
	if f.patch["expectedStatus"] != model.StatusInRemediation {
		t.Errorf("expectedStatus = %v, want IN_REMEDIATION", f.patch["expectedStatus"])
	}
	got := changedFields(t, f.patch)
	want := []string{"action_owner_id", "assignment_team_id", "owner_id"}
	if len(got) != len(want) {
		t.Fatalf("changeLog fields = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("changeLog fields = %v, want %v", got, want)
		}
	}
}
