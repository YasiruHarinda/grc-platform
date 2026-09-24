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
	"strings"
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
	noPlan    bool           // serve the risk without a STANDARD action plan
	patch     map[string]any // nil until a PATCH arrives
}

func (f *fakeRiskEntity) serve(t *testing.T) *riskRepository {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/risks/7/detail":
			detail := map[string]any{
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
			}
			if !f.noPlan {
				detail["actionPlan"] = map[string]any{"id": 5, "actionOwnerId": 13, "status": "COMPLETED", "planType": "STANDARD"}
			}
			_ = json.NewEncoder(w).Encode(detail)
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

// correction is what the Update Assignees dialog sends: two unchanged fields
// and three real changes.
func correction() model.UpdateAssigneesRequest {
	return model.UpdateAssigneesRequest{
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

func TestUpdateAssigneesInsideWindowSendsOnlyAssignees(t *testing.T) {
	for _, status := range []string{model.StatusClosed, model.StatusInRemediation, model.StatusEscalated} {
		t.Run(status, func(t *testing.T) {
			f := &fakeRiskEntity{status: status, createdBy: model.MigrationMarker, createdOn: time.Now().Add(-time.Hour)}
			if err := f.serve(t).UpdateAssignees(context.Background(), 7, correction(), "editor-uuid"); err != nil {
				t.Fatalf("UpdateAssignees: %v", err)
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
			if strings.Join(keys, ",") != strings.Join(want, ",") {
				t.Fatalf("PATCH keys = %v, want %v", keys, want)
			}
			if f.patch["expectedStatus"] != status {
				t.Errorf("expectedStatus = %v, want %s", f.patch["expectedStatus"], status)
			}
			if plan := f.patch["actionPlan"].(map[string]any); len(plan) != 1 || plan["actionOwnerId"] != float64(23) {
				t.Errorf("actionPlan = %v, want only actionOwnerId 23", plan)
			}
			if got, want := strings.Join(changedFields(t, f.patch), ","), "action_owner_id,assignment_team_id,owner_id"; got != want {
				t.Errorf("changeLog fields = %s, want %s", got, want)
			}
		})
	}
}

func TestUpdateAssigneesOutsideWindowIsRejected(t *testing.T) {
	tests := []struct {
		name      string
		createdBy string
		createdOn time.Time
		wantBody  string
	}{
		{"migrated, window closed", model.MigrationMarker, time.Now().Add(-15 * 24 * time.Hour), "window has closed"},
		{"created by a user", "3f1c9a2e-user-uuid", time.Now().Add(-time.Hour), "only risks created by the risk register migration"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRiskEntity{status: model.StatusInRemediation, createdBy: tc.createdBy, createdOn: tc.createdOn}
			err := f.serve(t).UpdateAssignees(context.Background(), 7, correction(), "editor-uuid")

			var apiErr *apierror.Error
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
				t.Fatalf("err = %v, want a 409", err)
			}
			// The two causes must read differently, not share one message.
			if !strings.Contains(apiErr.Body, tc.wantBody) {
				t.Errorf("409 body = %q, want it to mention %q", apiErr.Body, tc.wantBody)
			}
			if f.patch != nil {
				t.Errorf("PATCH was sent: %v", f.patch)
			}
		})
	}
}

// A normal edit still cannot touch a CLOSED risk, even a migrated one inside
// its window — the correction goes through UpdateAssignees only.
func TestUpdateClosedMigratedRiskIsStillRejected(t *testing.T) {
	f := &fakeRiskEntity{status: model.StatusClosed, createdBy: model.MigrationMarker, createdOn: time.Now().Add(-time.Hour)}
	err := f.serve(t).Update(context.Background(), 7, model.UpdateRiskRequest{RiskTitle: "Title", RiskDescription: "Description", EmailSubject: "Subject"}, "editor-uuid")

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v, want a 409", err)
	}
	if f.patch != nil {
		t.Errorf("PATCH was sent: %v", f.patch)
	}
}

// A normal edit that changes people now leaves a history entry for each.
func TestUpdateLogsAssigneeChanges(t *testing.T) {
	f := &fakeRiskEntity{status: model.StatusPendingOwnerApproval, createdBy: "3f1c9a2e-user-uuid", createdOn: time.Now()}
	req := model.UpdateRiskRequest{
		RiskTitle:       "Title",
		RiskDescription: "Description",
		EmailSubject:    "Subject",
		OwnerID:         intPtr(21),
		AssignerID:      intPtr(10), // unchanged
	}
	if err := f.serve(t).Update(context.Background(), 7, req, "editor-uuid"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := strings.Join(changedFields(t, f.patch), ","); got != "owner_id" {
		t.Errorf("changeLog fields = %s, want owner_id", got)
	}
}

// With no STANDARD plan the entity would silently write no Action Owner, so
// the correction is refused instead of recording a change that never happened.
func TestUpdateAssigneesActionOwnerWithoutPlanIsRejected(t *testing.T) {
	f := &fakeRiskEntity{status: model.StatusInRemediation, createdBy: model.MigrationMarker, createdOn: time.Now().Add(-time.Hour), noPlan: true}
	err := f.serve(t).UpdateAssignees(context.Background(), 7, model.UpdateAssigneesRequest{ActionOwnerID: intPtr(23)}, "editor-uuid")

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v, want a 409", err)
	}
	if f.patch != nil {
		t.Errorf("PATCH was sent: %v", f.patch)
	}
}

// The other four fields still correct fine on a plan-less risk.
func TestUpdateAssigneesWithoutPlanStillAppliesOtherFields(t *testing.T) {
	f := &fakeRiskEntity{status: model.StatusInRemediation, createdBy: model.MigrationMarker, createdOn: time.Now().Add(-time.Hour), noPlan: true}
	if err := f.serve(t).UpdateAssignees(context.Background(), 7, model.UpdateAssigneesRequest{OwnerID: intPtr(21)}, "editor-uuid"); err != nil {
		t.Fatalf("UpdateAssignees: %v", err)
	}
	if got := strings.Join(changedFields(t, f.patch), ","); got != "owner_id" {
		t.Errorf("changeLog fields = %s, want owner_id", got)
	}
}
