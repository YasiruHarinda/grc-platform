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

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type migrateStub struct {
	t *testing.T

	// recorders
	createBody   *CreateRiskRequest
	statusHops   []string // workflowStatus values PATCHed, in order
	patchBodies  []PatchRiskRequest
	escalations  int
	grantBodies  []CreateGrantRequest
	planPatched  *PatchActionPlanRequest
	nextRiskID   int
	openEscas    []Escalation // returned by GET /risks/{id}/escalations
	standardPlan int          // STANDARD action plan id returned by GET action-plans

	// failure injection
	failCreateStatus int
	failCreateBody   string
}

func (m *migrateStub) client(t *testing.T) *EntityClient {
	t.Helper()
	if m.nextRiskID == 0 {
		m.nextRiskID = 900
	}
	if m.standardPlan == 0 {
		m.standardPlan = 7000
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/risks":
			if m.failCreateStatus != 0 {
				w.WriteHeader(m.failCreateStatus)
				_, _ = w.Write([]byte(m.failCreateBody))
				return
			}
			var body CreateRiskRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.createBody = &body
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": m.nextRiskID, "workflowStatus": "PENDING_RISK_OWNER_APPROVAL",
				"actionPlanId": m.standardPlan, "createdBy": marker,
			})

		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/risks/"):
			var body PatchRiskRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.patchBodies = append(m.patchBodies, body)
			if body.WorkflowStatus != nil {
				m.statusHops = append(m.statusHops, *body.WorkflowStatus)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": m.nextRiskID})

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/escalations"):
			_ = json.NewEncoder(w).Encode(map[string]any{"escalations": m.openEscas})

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/escalations"):
			m.escalations++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "status": "OPEN"})

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/action-plans"):
			_ = json.NewEncoder(w).Encode(map[string]any{"plans": []map[string]any{
				{"id": m.standardPlan, "planType": "STANDARD", "status": "PENDING"},
				{"id": 7001, "planType": "MANAGEMENT", "status": "PENDING"},
			}})

		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/action-plans/"):
			var body PatchActionPlanRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.planPatched = &body
			_ = json.NewEncoder(w).Encode(map[string]any{"id": m.standardPlan})

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/grants/user/"):
			var body CreateGrantRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			uid, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/grants/user/"))
			_ = uid
			m.grantBodies = append(m.grantBodies, body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})

		default:
			m.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return NewEntityClient(srv.URL, 5*time.Second)
}

func migrateCfg() Config { return Config{MigrationDate: "2026-09-15"} }

// migRow is a fully-resolved IN_REMEDIATION row (ids wired to sheetTestRefData).
func migRow(status string) Row {
	return Row{
		MigrationID: 1, CSVLine: 2, RiskTitle: "R1",
		RiskYear: 2025, RiskQuarter: "Q3",
		SourceRegisterID: 8, AssignmentTeamID: 1,
		OwnerID: 100, AssignerID: 101, ManagementApproverID: 102,
		WorkflowStatus: status, ImplementationDate: "2099-01-01", // not overdue by default
		TreatmentStrategy: "REMEDIATE", Likelihood: 2, Impact: 2,
		RiskDescription: "d", ActionSteps: []string{"step 1"},
	}
}

func TestMigrateRow_FreshInRemediation(t *testing.T) {
	m := &migrateStub{t: t}
	rd := sheetTestRefData(t)
	rep := NewReport()

	err := migrateRow(context.Background(), m.client(t), migrateCfg(), rd,
		migRow("IN_REMEDIATION"), ResumeState{Progress: ProgressNone}, rep)
	if err != nil {
		t.Fatalf("migrateRow: %v", err)
	}

	if m.createBody == nil || m.createBody.CreatedBy != marker || m.createBody.RiskTitle != "R1" {
		t.Fatalf("create body = %+v", m.createBody)
	}
	if want := []string{"PENDING_COMPLIANCE_REVIEW", "IN_REMEDIATION"}; !equalStrs(m.statusHops, want) {
		t.Errorf("status hops = %v, want %v", m.statusHops, want)
	}
	// The IN_REMEDIATION hop carries complianceApprovalDate and no approver.
	last := m.patchBodies[len(m.patchBodies)-1]
	if last.ComplianceApprovalDate == nil || *last.ComplianceApprovalDate != "2026-09-15" {
		t.Errorf("IN_REMEDIATION hop should set complianceApprovalDate: %+v", last)
	}
	if last.ComplianceApprovalBy != nil {
		t.Errorf("complianceApprovalBy must stay null: %+v", last)
	}
	if m.escalations != 0 {
		t.Errorf("not overdue -> no escalation, got %d", m.escalations)
	}
	if len(m.grantBodies) != 2 {
		t.Fatalf("want owner + assigner grants, got %+v", m.grantBodies)
	}
	assertGrant(t, m.grantBodies, CreateGrantRequest{RoleID: 10, ScopeType: "RISK_TEAM", ScopeID: 1, CreatedBy: marker})
	assertGrant(t, m.grantBodies, CreateGrantRequest{RoleID: 11, ScopeType: "RISK_TEAM", ScopeID: 8, CreatedBy: marker})
	if rep.migrated != 1 || rep.grantsWritten != 2 {
		t.Errorf("report: migrated=%d grants=%d", rep.migrated, rep.grantsWritten)
	}
}

func TestMigrateRow_OverdueGetsSuppressingEscalation(t *testing.T) {
	m := &migrateStub{t: t}
	rep := NewReport()
	row := migRow("IN_REMEDIATION")
	row.ImplementationDate = "2020-01-01" // < migrationDate

	if err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		row, ResumeState{Progress: ProgressNone}, rep); err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	if m.escalations != 1 {
		t.Errorf("overdue row should get one suppressing escalation, got %d", m.escalations)
	}
	if len(rep.suppressingEscalations) != 1 || rep.suppressingEscalations[0] != 1 {
		t.Errorf("suppressingEscalations = %v", rep.suppressingEscalations)
	}
}

func TestMigrateRow_OverdueButEscalationAlreadyPresent(t *testing.T) {
	m := &migrateStub{t: t, openEscas: []Escalation{{ID: 5, Status: "OPEN", CreatedBy: marker}}}
	rep := NewReport()
	row := migRow("IN_REMEDIATION")
	row.ImplementationDate = "2020-01-01"

	if err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		row, ResumeState{Progress: ProgressNone}, rep); err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	if m.escalations != 0 {
		t.Errorf("an OPEN marker escalation already exists -> no new POST, got %d", m.escalations)
	}
}

func TestMigrateRow_ConditionalManagementGrant(t *testing.T) {
	m := &migrateStub{t: t}
	rep := NewReport()
	row := migRow("IN_REMEDIATION")
	row.TreatmentStrategy = "ACCEPT"
	row.Likelihood, row.Impact = 3, 3 // 9 >= 7

	if err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		row, ResumeState{Progress: ProgressNone}, rep); err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	if len(m.grantBodies) != 3 {
		t.Fatalf("ACCEPT + high -> 3 grants, got %+v", m.grantBodies)
	}
	assertGrant(t, m.grantBodies, CreateGrantRequest{RoleID: 12, ScopeType: "GLOBAL", ScopeID: 0, CreatedBy: marker})
}

func TestMigrateRow_FreshClosed(t *testing.T) {
	m := &migrateStub{t: t}
	rep := NewReport()

	if err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		migRow("CLOSED"), ResumeState{Progress: ProgressNone}, rep); err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	want := []string{
		"PENDING_COMPLIANCE_REVIEW", "IN_REMEDIATION",
		"PENDING_OWNER_COMPLETION_APPROVAL", "PENDING_COMPLIANCE_CLOSURE", "CLOSED",
	}
	if !equalStrs(m.statusHops, want) {
		t.Errorf("status hops = %v, want %v", m.statusHops, want)
	}
	if len(m.grantBodies) != 0 {
		t.Errorf("CLOSED rows get no grants, got %+v", m.grantBodies)
	}
	if m.planPatched == nil || m.planPatched.Status == nil || *m.planPatched.Status != "COMPLETED" {
		t.Fatalf("plan not completed: %+v", m.planPatched)
	}
	if m.planPatched.CompletedDate == nil || *m.planPatched.CompletedDate != "2026-09-15" {
		t.Errorf("plan completedDate = %+v", m.planPatched.CompletedDate)
	}
}

func TestMigrateRow_ResumeStatusWalked_SkipsCreateAndWalk(t *testing.T) {
	m := &migrateStub{t: t}
	rep := NewReport()

	err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		migRow("IN_REMEDIATION"),
		ResumeState{Progress: ProgressStatusWalked, RiskID: 900, CurrentStatus: "IN_REMEDIATION"}, rep)
	if err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	if m.createBody != nil {
		t.Error("resume must not POST /risks")
	}
	if len(m.statusHops) != 0 {
		t.Errorf("resume at IN_REMEDIATION must not re-walk status, got %v", m.statusHops)
	}
	if len(m.grantBodies) != 2 {
		t.Errorf("resume should still ensure grants, got %+v", m.grantBodies)
	}
	if rep.migrated != 1 {
		t.Errorf("migrated = %d", rep.migrated)
	}
}

func TestMigrateRow_ResumeComplete_IsSkip(t *testing.T) {
	m := &migrateStub{t: t}
	rep := NewReport()

	err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		migRow("IN_REMEDIATION"),
		ResumeState{Progress: ProgressComplete, RiskID: 900, CurrentStatus: "IN_REMEDIATION"}, rep)
	if err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	if m.createBody != nil || len(m.statusHops) != 0 || len(m.grantBodies) != 0 {
		t.Error("ProgressComplete must make no calls")
	}
	if rep.skipped != 1 || rep.migrated != 0 {
		t.Errorf("report: skipped=%d migrated=%d", rep.skipped, rep.migrated)
	}
}

func TestMigrateRow_RowLevelCreateErrorRejectsAndContinues(t *testing.T) {
	m := &migrateStub{t: t, failCreateStatus: 400, failCreateBody: `{"code":400,"message":"assignerId is required"}`}
	rep := NewReport()

	err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		migRow("IN_REMEDIATION"), ResumeState{Progress: ProgressNone}, rep)
	if err != nil {
		t.Fatalf("a 400 must not be fatal, got %v", err)
	}
	if _, bad := rep.RejectedMigrationIDs()[1]; !bad {
		t.Fatalf("row should be rejected; findings=%+v", rep.findings)
	}
	if rep.migrated != 0 {
		t.Errorf("migrated = %d, want 0", rep.migrated)
	}
}

func TestMigrateRow_ServerErrorIsFatal(t *testing.T) {
	m := &migrateStub{t: t, failCreateStatus: 503, failCreateBody: `service unavailable`}
	rep := NewReport()

	err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		migRow("IN_REMEDIATION"), ResumeState{Progress: ProgressNone}, rep)
	if err == nil {
		t.Fatal("a 503 must be fatal (return err)")
	}
}

func TestMigrateRow_TreatmentEnumBackstop(t *testing.T) {
	m := &migrateStub{t: t, failCreateStatus: 400, failCreateBody: `{"code":400,"message":"Data truncated for column 'treatment_strategy'"}`}
	rep := NewReport()

	err := migrateRow(context.Background(), m.client(t), migrateCfg(), sheetTestRefData(t),
		migRow("IN_REMEDIATION"), ResumeState{Progress: ProgressNone}, rep)
	if err == nil || !strings.Contains(err.Error(), "enum") {
		t.Fatalf("treatment enum error should abort with a clear message, got %v", err)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertGrant(t *testing.T, got []CreateGrantRequest, want CreateGrantRequest) {
	t.Helper()
	for _, g := range got {
		if g == want {
			return
		}
	}
	t.Errorf("missing grant %+v in %+v", want, got)
}
