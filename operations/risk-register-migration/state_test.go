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

type stateStub struct {
	t           *testing.T
	risks       []Risk
	escalations map[int][]Escalation
	plans       map[int][]ActionPlanView
	grants      map[int][]Grant
	pageSize    int // force small pages to exercise paging; 0 = honour the request

	searchCalls int
}

func pathID(path, prefix, suffix string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix))
	return n
}

func (s *stateStub) client(t *testing.T) *EntityClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/risks/search":
			s.searchCalls++
			var req SearchRisksRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			lim := req.Pagination.Limit
			if s.pageSize > 0 && s.pageSize < lim {
				lim = s.pageSize
			}
			off := req.Pagination.Offset
			end := off + lim
			if end > len(s.risks) {
				end = len(s.risks)
			}
			var page []Risk
			if off < len(s.risks) {
				page = s.risks[off:end]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"risks": page, "total": len(s.risks), "limit": lim, "offset": off,
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/escalations"):
			id := pathID(r.URL.Path, "/risks/", "/escalations")
			_ = json.NewEncoder(w).Encode(map[string]any{"escalations": s.escalations[id]})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/action-plans"):
			id := pathID(r.URL.Path, "/risks/", "/action-plans")
			_ = json.NewEncoder(w).Encode(map[string]any{"plans": s.plans[id]})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/grants/user/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/grants/user/"))
			_ = json.NewEncoder(w).Encode(map[string]any{"userId": id, "grants": s.grants[id]})
		default:
			s.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return NewEntityClient(srv.URL, 5*time.Second)
}

const testMigrationDate = "2026-09-15"

func markerRisk(id int, title string, sr, year int, quarter, status string) Risk {
	return Risk{
		ID: id, RiskTitle: title, SourceRegID: sr, RiskYear: year, RiskQuarter: quarter,
		WorkflowStatus: status, CreatedBy: marker,
	}
}

// baseRow is an IN_REMEDIATION row wired to sheetTestRefData ids: owner 100 /
// assigner 101 / mgmt 102, source register 8 (Legal), assignment team 1.
func baseRow(migID int, title, status string) Row {
	return Row{
		MigrationID: migID, CSVLine: migID + 1, RiskTitle: title,
		RiskYear: 2025, RiskQuarter: "Q3",
		SourceRegisterID: 8, AssignmentTeamID: 1,
		OwnerID: 100, AssignerID: 101, ManagementApproverID: 102,
		WorkflowStatus: status, ImplementationDate: "2020-01-01",
		TreatmentStrategy: "REMEDIATE", Likelihood: 2, Impact: 2,
	}
}

func ownerAssignerGrants() map[int][]Grant {
	return map[int][]Grant{
		100: {{RoleID: 10, ScopeType: "RISK_TEAM", ScopeID: 1}},
		101: {{RoleID: 11, ScopeType: "RISK_TEAM", ScopeID: 8}},
	}
}

func run6(t *testing.T, s *stateStub, rd RefData, rows []Row) (map[int]ResumeState, *Report) {
	t.Helper()
	rep := NewReport()
	prog, err := reconstructState(context.Background(), s.client(t), rd, testMigrationDate, rows, rep)
	if err != nil {
		t.Fatalf("reconstructState: %v", err)
	}
	return prog, rep
}

func TestReconstructState_NoneAndCreated_WithPaging(t *testing.T) {
	rd := sheetTestRefData(t)
	s := &stateStub{
		t:        t,
		pageSize: 2, // 3 risks over pages of 2 -> two search calls
		risks: []Risk{
			markerRisk(1, "filler-a", 8, 2025, "Q3", "IN_REMEDIATION"),
			markerRisk(2, "filler-b", 8, 2025, "Q3", "CLOSED"),
			markerRisk(3, "Created risk", 8, 2025, "Q3", "PENDING_COMPLIANCE_REVIEW"),
		},
		escalations: map[int][]Escalation{},
	}
	rows := []Row{
		baseRow(1, "Ghost risk", "IN_REMEDIATION"),   // no marker match
		baseRow(2, "Created risk", "IN_REMEDIATION"), // matched, status not yet walked
	}

	prog, rep := run6(t, s, rd, rows)
	if rep.HasFindings() {
		t.Fatalf("unexpected findings: %+v", rep.findings)
	}
	if prog[1].Progress != ProgressNone {
		t.Errorf("row 1 = %v, want ProgressNone", prog[1])
	}
	if prog[2].Progress != ProgressCreated {
		t.Errorf("row 2 = %v, want ProgressCreated", prog[2])
	}
	if s.searchCalls != 2 {
		t.Errorf("searchCalls = %d, want 2 (paged)", s.searchCalls)
	}
}

func TestReconstructState_InRemediationStages(t *testing.T) {
	rd := sheetTestRefData(t)

	tests := []struct {
		name        string
		status      string // matched risk's workflow_status
		implDate    string
		escalations []Escalation
		grants      map[int][]Grant
		want        RowProgress
	}{
		{"overdue, no escalation -> StatusWalked", "IN_REMEDIATION", "2020-01-01", nil, nil, ProgressStatusWalked},
		{"overdue, open marker escalation, grants missing -> Escalated", "IN_REMEDIATION", "2020-01-01",
			[]Escalation{{ID: 1, Status: "OPEN", CreatedBy: marker}}, nil, ProgressEscalated},
		{"not overdue, grants missing -> Escalated", "IN_REMEDIATION", "2099-01-01", nil, nil, ProgressEscalated},
		{"not overdue, grants present -> Complete", "IN_REMEDIATION", "2099-01-01", nil, ownerAssignerGrants(), ProgressComplete},
		{"status not yet walked -> Created", "PENDING_COMPLIANCE_REVIEW", "2099-01-01", nil, ownerAssignerGrants(), ProgressCreated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &stateStub{
				t:           t,
				risks:       []Risk{markerRisk(50, "Stage risk", 8, 2025, "Q3", tc.status)},
				escalations: map[int][]Escalation{50: tc.escalations},
				grants:      tc.grants,
			}
			row := baseRow(1, "Stage risk", "IN_REMEDIATION")
			row.ImplementationDate = tc.implDate

			prog, rep := run6(t, s, rd, []Row{row})
			if rep.HasFindings() {
				t.Fatalf("unexpected findings: %+v", rep.findings)
			}
			if prog[1].Progress != tc.want {
				t.Errorf("got %v, want %v", prog[1], tc.want)
			}
		})
	}
}

func TestReconstructState_ConditionalManagementGrant(t *testing.T) {
	rd := sheetTestRefData(t)
	s := &stateStub{
		t:     t,
		risks: []Risk{markerRisk(60, "ACCEPT high", 8, 2025, "Q3", "IN_REMEDIATION")},
		grants: map[int][]Grant{
			100: {{RoleID: 10, ScopeType: "RISK_TEAM", ScopeID: 1}},
			101: {{RoleID: 11, ScopeType: "RISK_TEAM", ScopeID: 8}},
			// 102 (management) has no GLOBAL grant yet
		},
	}
	row := baseRow(1, "ACCEPT high", "IN_REMEDIATION")
	row.ImplementationDate = "2099-01-01" // not overdue
	row.TreatmentStrategy = "ACCEPT"
	row.Likelihood, row.Impact = 3, 3 // 9 >= 7 -> management grant required

	prog, _ := run6(t, s, rd, []Row{row})
	if prog[1].Progress != ProgressEscalated {
		t.Fatalf("missing management grant should hold at ProgressEscalated, got %v", prog[1])
	}

	// Add the management GLOBAL grant -> now complete.
	s.grants[102] = []Grant{{RoleID: 12, ScopeType: "GLOBAL", ScopeID: 0}}
	prog, _ = run6(t, s, rd, []Row{row})
	if prog[1].Progress != ProgressComplete {
		t.Fatalf("with all three grants -> want ProgressComplete, got %v", prog[1])
	}
}

func TestReconstructState_ClosedBucket(t *testing.T) {
	rd := sheetTestRefData(t)

	for _, tc := range []struct {
		name   string
		plans  []ActionPlanView
		want   RowProgress
		status string
	}{
		{"status at CLOSED, plan pending -> StatusWalked", []ActionPlanView{{ID: 1, PlanType: "STANDARD", Status: "PENDING"}}, ProgressStatusWalked, "CLOSED"},
		{"status at CLOSED, plan completed -> Complete", []ActionPlanView{{ID: 1, PlanType: "STANDARD", Status: "COMPLETED"}}, ProgressComplete, "CLOSED"},
		{"status not at CLOSED -> Created", []ActionPlanView{{ID: 1, PlanType: "STANDARD", Status: "COMPLETED"}}, ProgressCreated, "IN_REMEDIATION"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &stateStub{
				t:     t,
				risks: []Risk{markerRisk(70, "Closed risk", 8, 2025, "Q3", tc.status)},
				plans: map[int][]ActionPlanView{70: tc.plans},
			}
			row := baseRow(1, "Closed risk", "CLOSED")
			prog, rep := run6(t, s, rd, []Row{row})
			if rep.HasFindings() {
				t.Fatalf("unexpected findings: %+v", rep.findings)
			}
			if prog[1].Progress != tc.want {
				t.Errorf("got %v, want %v", prog[1], tc.want)
			}
		})
	}
}

func TestReconstructState_NaturalKeyCollisions(t *testing.T) {
	rd := sheetTestRefData(t)

	t.Run("two pending rows on one key", func(t *testing.T) {
		s := &stateStub{t: t}
		rows := []Row{
			baseRow(1, "Same title", "IN_REMEDIATION"),
			baseRow(2, "Same title", "IN_REMEDIATION"),
		}
		prog, rep := run6(t, s, rd, rows)
		rej := rep.RejectedMigrationIDs()
		if _, ok := rej[1]; !ok {
			t.Errorf("row 1 not rejected")
		}
		if _, ok := rej[2]; !ok {
			t.Errorf("row 2 not rejected")
		}
		if _, ok := prog[1]; ok {
			t.Errorf("collided row must not get a progress entry")
		}
	})

	t.Run("two marker risks on one key", func(t *testing.T) {
		s := &stateStub{
			t: t,
			risks: []Risk{
				markerRisk(80, "Dup risk", 8, 2025, "Q3", "IN_REMEDIATION"),
				markerRisk(81, "Dup risk", 8, 2025, "Q3", "IN_REMEDIATION"),
			},
		}
		prog, rep := run6(t, s, rd, []Row{baseRow(1, "Dup risk", "IN_REMEDIATION")})
		if _, ok := rep.RejectedMigrationIDs()[1]; !ok {
			t.Errorf("row matching two marker risks should be rejected; findings=%+v", rep.findings)
		}
		if _, ok := prog[1]; ok {
			t.Errorf("row must not get a progress entry")
		}
	})
}

func TestStatusAtLeast(t *testing.T) {
	if !statusAtLeast("IN_REMEDIATION", "IN_REMEDIATION") {
		t.Error("equal should be >=")
	}
	if !statusAtLeast("CLOSED", "IN_REMEDIATION") {
		t.Error("CLOSED is past IN_REMEDIATION")
	}
	if statusAtLeast("PENDING_COMPLIANCE_REVIEW", "IN_REMEDIATION") {
		t.Error("PENDING_COMPLIANCE_REVIEW is before IN_REMEDIATION")
	}
	if statusAtLeast("ESCALATED", "IN_REMEDIATION") {
		t.Error("a state off the migration path must not count as >=")
	}
}
