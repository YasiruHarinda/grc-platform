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

// fixture_test.go drives testdata/risks.csv — a CSV built from the
// "Risk Form Structure" tab of Risk_Test_Full.xlsx, with the four person
// columns replaced by synthetic @wso2.com emails and the operator-added
// Workflow Status / Migration ID columns — through the whole pipeline
// (parse → resolve → reconstruct → migrate) against a stateful in-memory
// fake of the compliance-entity API.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ── fixture reference data ─────────────────────────────────────────────────

func fixtureRefData(t *testing.T) RefData {
	t.Helper()
	teams := []RiskTeam{
		{ID: 1, Name: "Asgardeo", Code: strptr("ASG"), Status: "ACTIVE"},
		{ID: 2, Name: "Choreo", Code: strptr("CHO"), Status: "ACTIVE"},
		{ID: 3, Name: "Clever Care", Code: strptr("CC"), Status: "ACTIVE"},
		{ID: 4, Name: "Business", Code: strptr("Biz"), Status: "ACTIVE"},
		{ID: 5, Name: "Digi Ops", Code: strptr("DiOp"), Status: "ACTIVE"},
		{ID: 6, Name: "Legal", Code: nil, Status: "ACTIVE"},
	}
	cats := []RiskCategory{
		{ID: 30, Name: "Access Control & Credentials"},
		{ID: 31, Name: "Logging, Monitoring & Detection"},
		{ID: 32, Name: "Data Exposure & Privacy (PII)"},
		{ID: 33, Name: "Process & Documentation Gaps"},
	}
	refs := []ComplianceRef{
		{ID: 40, Name: "ISO"}, {ID: 41, Name: "SOC2"},
		{ID: 42, Name: "HIPAA"}, {ID: 43, Name: "BUSINESS"},
	}
	var scores []RiskScore
	id := 900
	for l := 1; l <= 3; l++ {
		for i := 1; i <= 3; i++ {
			scores = append(scores, RiskScore{ID: id, Likelihood: l, Impact: i})
			id++
		}
	}
	rd, err := buildRefData(teams, cats, refs, scores, goodRoles())
	if err != nil {
		t.Fatalf("buildRefData: %v", err)
	}
	return rd
}

// fixtureSnapshot maps every email in testdata/risks.csv to a uuid — except
// user6@wso2.com (row 6), which is deliberately absent.
func fixtureSnapshot() []DirectoryUser {
	return []DirectoryUser{
		{UUID: "uuid-user1", Email: "user1@wso2.com"},
		{UUID: "uuid-user2", Email: "user2@wso2.com"},
		{UUID: "uuid-user3", Email: "user3@wso2.com"},
		{UUID: "uuid-user4", Email: "user4@wso2.com"},
		{UUID: "uuid-user5", Email: "user5@wso2.com"},
	}
}

func loadFixture(t *testing.T) ([]Row, []Finding) {
	t.Helper()
	f, err := os.Open("testdata/risks.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	rows, fs, err := parseSheet(f, fixtureRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	return rows, fs
}

// ── stateful in-memory fake of compliance-entity ─────────────────────────

type fakeEntity struct {
	t *testing.T

	risks       map[int]*Risk
	escalations map[int][]Escalation // by risk id
	planStatus  map[int]string       // by risk id; "" == PENDING
	grants      map[int][]Grant      // by user id
	users       map[string]int       // uuid -> user id

	// cumulative call log (snapshot lengths between runs to detect no-ops)
	createdRisks []CreateRiskRequest
	riskPatches  []PatchRiskRequest
	planPatches  []PatchActionPlanRequest
	escPosts     []int
	grantPosts   []grantCall
	usersCreated []string

	nextRiskID int
	nextUserID int
}

type grantCall struct {
	userID int
	body   CreateGrantRequest
}

func newFakeEntity(t *testing.T) *fakeEntity {
	return &fakeEntity{
		t: t, risks: map[int]*Risk{}, escalations: map[int][]Escalation{},
		planStatus: map[int]string{}, grants: map[int][]Grant{}, users: map[string]int{},
		nextRiskID: 1000, nextUserID: 500,
	}
}

func (fe *fakeEntity) client(t *testing.T) *EntityClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(fe.handle))
	t.Cleanup(srv.Close)
	return NewEntityClient(srv.URL, 5*time.Second)
}

func (fe *fakeEntity) planID(riskID int) int { return riskID * 10 }
func (fe *fakeEntity) riskForPlan(planID int) int {
	return planID / 10
}

func (fe *fakeEntity) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	p := r.URL.Path
	enc := json.NewEncoder(w)

	switch {
	case r.Method == http.MethodPost && p == "/risks/search":
		all := make([]Risk, 0, len(fe.risks))
		for _, rk := range fe.risks {
			all = append(all, *rk)
		}
		_ = enc.Encode(map[string]any{"risks": all, "total": len(all), "limit": 100, "offset": 0})

	case r.Method == http.MethodGet && strings.HasPrefix(p, "/users/by-uuid/"):
		uuid := strings.TrimPrefix(p, "/users/by-uuid/")
		if id, ok := fe.users[uuid]; ok {
			_ = enc.Encode(map[string]int{"id": id})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))

	case r.Method == http.MethodPost && p == "/users":
		var body CreateUserRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.CreatedBy != marker {
			fe.t.Errorf("CreateUser createdBy = %q", body.CreatedBy)
		}
		fe.nextUserID++
		fe.users[body.UUID] = fe.nextUserID
		fe.usersCreated = append(fe.usersCreated, body.UUID)
		w.WriteHeader(http.StatusCreated)
		_ = enc.Encode(map[string]int{"id": fe.nextUserID})

	case r.Method == http.MethodPost && p == "/risks":
		var body CreateRiskRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		fe.createdRisks = append(fe.createdRisks, body)
		fe.nextRiskID++
		fe.risks[fe.nextRiskID] = &Risk{
			ID: fe.nextRiskID, RiskTitle: body.RiskTitle, SourceRegID: body.SourceRegisterID,
			RiskYear: body.RiskYear, RiskQuarter: body.RiskQuarter,
			WorkflowStatus: "PENDING_RISK_OWNER_APPROVAL", CreatedBy: marker,
		}
		w.WriteHeader(http.StatusCreated)
		_ = enc.Encode(map[string]any{
			"id": fe.nextRiskID, "workflowStatus": "PENDING_RISK_OWNER_APPROVAL",
			"actionPlanId": fe.planID(fe.nextRiskID), "createdBy": marker,
		})

	case r.Method == http.MethodPatch && strings.HasPrefix(p, "/risks/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(p, "/risks/"))
		var body PatchRiskRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		fe.riskPatches = append(fe.riskPatches, body)
		if rk := fe.risks[id]; rk != nil && body.WorkflowStatus != nil {
			rk.WorkflowStatus = *body.WorkflowStatus
		}
		_ = enc.Encode(map[string]any{"id": id})

	case r.Method == http.MethodGet && strings.HasSuffix(p, "/escalations"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, "/risks/"), "/escalations"))
		_ = enc.Encode(map[string]any{"escalations": fe.escalations[id]})

	case r.Method == http.MethodPost && strings.HasSuffix(p, "/escalations"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, "/risks/"), "/escalations"))
		fe.escalations[id] = append(fe.escalations[id], Escalation{ID: len(fe.escalations[id]) + 1, Status: "OPEN", CreatedBy: marker})
		fe.escPosts = append(fe.escPosts, id)
		w.WriteHeader(http.StatusCreated)
		_ = enc.Encode(map[string]any{"id": 1, "status": "OPEN"})

	case r.Method == http.MethodGet && strings.HasSuffix(p, "/action-plans"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, "/risks/"), "/action-plans"))
		status := fe.planStatus[id]
		if status == "" {
			status = "PENDING"
		}
		_ = enc.Encode(map[string]any{"plans": []map[string]any{
			{"id": fe.planID(id), "planType": "STANDARD", "status": status},
		}})

	case r.Method == http.MethodPatch && strings.HasPrefix(p, "/action-plans/"):
		planID, _ := strconv.Atoi(strings.TrimPrefix(p, "/action-plans/"))
		var body PatchActionPlanRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		fe.planPatches = append(fe.planPatches, body)
		if body.Status != nil {
			fe.planStatus[fe.riskForPlan(planID)] = *body.Status
		}
		_ = enc.Encode(map[string]any{"id": planID})

	case r.Method == http.MethodGet && strings.HasPrefix(p, "/grants/user/"):
		uid, _ := strconv.Atoi(strings.TrimPrefix(p, "/grants/user/"))
		_ = enc.Encode(map[string]any{"grants": fe.grants[uid]})

	case r.Method == http.MethodPost && strings.HasPrefix(p, "/grants/user/"):
		uid, _ := strconv.Atoi(strings.TrimPrefix(p, "/grants/user/"))
		var body CreateGrantRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		fe.grants[uid] = append(fe.grants[uid], Grant{RoleID: body.RoleID, ScopeType: body.ScopeType, ScopeID: body.ScopeID})
		fe.grantPosts = append(fe.grantPosts, grantCall{userID: uid, body: body})
		w.WriteHeader(http.StatusCreated)
		_ = enc.Encode(map[string]any{"id": 1})

	default:
		fe.t.Errorf("unexpected %s %s", r.Method, p)
		w.WriteHeader(http.StatusNotFound)
	}
}

// runPipeline threads a fresh Report through resolve → reconstruct → migrate
// against fe, exactly as run() does after a non-dry-run preflight.
func runPipeline(t *testing.T, fe *fakeEntity, rows []Row, rd RefData) *Report {
	t.Helper()
	ec := fe.client(t)
	rep := NewReport()

	res := NewResolver(ec, fixtureSnapshot(), false)
	res.Build()
	rfs, err := res.Apply(context.Background(), rows)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rep.Add(rfs...)

	pending := selectMigratable(rows, rep)
	prog, err := reconstructState(context.Background(), ec, rd, "2026-09-15", pending, rep)
	if err != nil {
		t.Fatalf("reconstructState: %v", err)
	}
	rejected := rep.RejectedMigrationIDs()
	for _, row := range pending {
		if _, bad := rejected[row.MigrationID]; bad {
			continue
		}
		if err := migrateRow(context.Background(), ec, Config{MigrationDate: "2026-09-15"}, rd, row, prog[row.MigrationID], rep); err != nil {
			t.Fatalf("migrateRow %d: %v", row.MigrationID, err)
		}
	}
	return rep
}

// ── tests ─────────────────────────────────────────────────────────────────

func TestFixture_Parse(t *testing.T) {
	rows, fs := loadFixture(t)

	// Legend row and the trailing blank row are skipped; 6 data rows remain.
	if len(rows) != 6 {
		t.Fatalf("got %d rows, want 6 (Migration IDs 1..6)", len(rows))
	}
	for i, r := range rows {
		if r.MigrationID != i+1 {
			t.Errorf("row %d has MigrationID %d", i, r.MigrationID)
		}
	}

	// Row 1: a clean IN_REMEDIATION row exercising serial dates, an ordinal
	// date, the HIPPA alias, and the "Text" git-URL placeholder.
	r1 := rows[0]
	if r1.RiskYear != 2025 || r1.RiskQuarter != "Q3" || r1.WorkflowStatus != "IN_REMEDIATION" {
		t.Errorf("row 1 scalars: %+v", r1)
	}
	if r1.SourceRegisterID != 1 || r1.AssignmentTeamID != 4 {
		t.Errorf("row 1 team ids: sr=%d at=%d", r1.SourceRegisterID, r1.AssignmentTeamID)
	}
	if len(r1.RiskCategoryIDs) != 1 || r1.RiskCategoryIDs[0] != 30 {
		t.Errorf("row 1 category: %v", r1.RiskCategoryIDs)
	}
	if len(r1.ComplianceRefIDs) != 1 || r1.ComplianceRefIDs[0] != 42 {
		t.Errorf("row 1 compliance (HIPPA→HIPAA=42): %v", r1.ComplianceRefIDs)
	}
	if r1.ImplementationDate != "2025-06-30" || r1.RiskIdentifiedDate != "2025-01-10" || r1.ReassessmentDate != "2025-09-30" {
		t.Errorf("row 1 dates: impl=%q id=%q re=%q", r1.ImplementationDate, r1.RiskIdentifiedDate, r1.ReassessmentDate)
	}
	if r1.TreatmentStrategy != "ACCEPT" || r1.IdentifiedByType != "EMPLOYEE" {
		t.Errorf("row 1 enums: %q %q", r1.TreatmentStrategy, r1.IdentifiedByType)
	}
	if r1.GitIssueURL != "" {
		t.Errorf("row 1 git URL placeholder not dropped: %q", r1.GitIssueURL)
	}
	if len(r1.ActionSteps) != 2 {
		t.Errorf("row 1 action steps (embedded newline): %v", r1.ActionSteps)
	}
	if r1.OwnerEmail != "user3@wso2.com" || r1.AssignerEmail != "user1@wso2.com" {
		t.Errorf("row 1 emails: owner=%q assigner=%q", r1.OwnerEmail, r1.AssignerEmail)
	}

	// Row 4 ("Avoid") maps to the AVOID enum.
	if rows[3].TreatmentStrategy != "AVOID" {
		t.Errorf("row 4 treatment: %q", rows[3].TreatmentStrategy)
	}

	// Row 6: the messy row — every listed column REJECTs, three WARN.
	wantReject := []string{
		"Year", "Quarter", "Source Register", "Risk Title", "Risk Category",
		"Likelihood", "Impact", "Implementation Date", "Assignment Team",
		"Treatment Strategy", "Workflow Status", "Security Compliance Reference",
	}
	for _, code := range wantReject {
		got := findingsForRow(fs, 6, code)
		if len(got) == 0 || got[0].Severity != SevReject {
			t.Errorf("row 6: missing REJECT for %q", code)
		}
	}
	for _, code := range []string{"Risk Identified By", "Risk Identified Date", "Reassessment Date"} {
		got := findingsForRow(fs, 6, code)
		if len(got) == 0 || got[0].Severity != SevWarn {
			t.Errorf("row 6: missing WARN for %q", code)
		}
	}

	// Rows 1..5 never REJECT at parse time.
	for _, id := range []int{1, 2, 3, 4, 5} {
		for _, f := range findingsForMig(fs, id) {
			if f.Severity == SevReject {
				t.Errorf("row %d unexpectedly REJECTed: %+v", id, f)
			}
		}
	}
}

func TestFixture_DryRunPipeline(t *testing.T) {
	rows, fs := loadFixture(t)
	rep := NewReport()
	rep.Add(fs...)

	res := NewResolver(nil, fixtureSnapshot(), true) // dry run: entity never touched
	res.Build()
	rfs, err := res.Apply(context.Background(), rows)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rep.Add(rfs...)

	if got := findingsForRow(rep.findings, 6, "Risk Owner"); len(got) == 0 || got[0].Severity != SevReject {
		t.Errorf("row 6 Risk Owner should be an unresolved REJECT; findings=%+v", rep.findings)
	}

	pending := selectMigratable(rows, rep)
	gotIDs := map[int]bool{}
	for _, r := range pending {
		gotIDs[r.MigrationID] = true
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if !gotIDs[id] {
			t.Errorf("row %d should be migratable", id)
		}
	}
	if gotIDs[6] {
		t.Errorf("row 6 must not be migratable")
	}

	fe := newFakeEntity(t) // empty entity — no existing marker risks
	prog, err := reconstructState(context.Background(), fe.client(t), fixtureRefData(t), "2026-09-15", pending, rep)
	if err != nil {
		t.Fatalf("reconstructState: %v", err)
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if prog[id].Progress != ProgressNone {
			t.Errorf("row %d progress = %v, want ProgressNone", id, prog[id].Progress)
		}
	}
}

func TestFixture_RealRunPipeline_ThenResumeIsNoOp(t *testing.T) {
	rows, _ := loadFixture(t)
	rd := fixtureRefData(t)
	fe := newFakeEntity(t)

	// ── first run: writes everything ──────────────────────────────────────
	rep := runPipeline(t, fe, rows, rd)

	if len(fe.createdRisks) != 5 {
		t.Fatalf("created %d risks, want 5", len(fe.createdRisks))
	}
	for _, cr := range fe.createdRisks {
		if cr.CreatedBy != marker || len(cr.ActionSteps) == 0 {
			t.Errorf("bad created risk: %+v", cr)
		}
	}
	if len(fe.usersCreated) != 5 {
		t.Errorf("provisioned %d users, want 5 distinct: %v", len(fe.usersCreated), fe.usersCreated)
	}

	inRem, closed := 0, 0
	for id, hops := range fe.statusHopsByRisk() {
		switch len(hops) {
		case 2:
			inRem++
			if hops[1] != "IN_REMEDIATION" {
				t.Errorf("risk %d 2-hop walk ends %q", id, hops[1])
			}
		case 5:
			closed++
			if hops[4] != "CLOSED" {
				t.Errorf("risk %d 5-hop walk ends %q", id, hops[4])
			}
		default:
			t.Errorf("risk %d unexpected hops %v", id, hops)
		}
	}
	if inRem != 3 || closed != 2 {
		t.Errorf("walks: inRemediation=%d closed=%d, want 3 and 2", inRem, closed)
	}

	// All three IN_REMEDIATION rows are overdue vs 2026-09-15 → 3 suppressing
	// escalations, matching the report.
	if len(fe.escPosts) != 3 || len(rep.suppressingEscalations) != 3 {
		t.Errorf("suppressing escalations: posts=%d report=%v", len(fe.escPosts), rep.suppressingEscalations)
	}

	// Grants: rows 1 & 3 → owner + assigner (2 each); row 5 is ACCEPT with
	// L3×I3=9 ≥ 7 → also the management GLOBAL grant. 2 + 2 + 3 = 7.
	if len(fe.grantPosts) != 7 {
		t.Errorf("grants = %d, want 7", len(fe.grantPosts))
	}
	global := 0
	for _, g := range fe.grantPosts {
		if g.body.CreatedBy != marker {
			t.Errorf("grant createdBy = %q", g.body.CreatedBy)
		}
		if g.body.ScopeType == "GLOBAL" {
			global++
			if g.body.RoleID != rd.RoleIDByName[roleRiskManagement] || g.body.ScopeID != 0 {
				t.Errorf("GLOBAL grant = %+v", g.body)
			}
		}
	}
	if global != 1 {
		t.Errorf("GLOBAL management grants = %d, want 1 (row 5)", global)
	}

	if len(fe.planPatches) != 2 {
		t.Errorf("plan completions = %d, want 2", len(fe.planPatches))
	}
	for _, pp := range fe.planPatches {
		if pp.Status == nil || *pp.Status != "COMPLETED" || pp.CompletedDate == nil || *pp.CompletedDate != "2026-09-15" {
			t.Errorf("plan patch = %+v", pp)
		}
	}
	if rep.migrated != 5 || rep.migratedByBucket["IN_REMEDIATION"] != 3 || rep.migratedByBucket["CLOSED"] != 2 {
		t.Errorf("report: migrated=%d byBucket=%v", rep.migrated, rep.migratedByBucket)
	}

	// ── second run against the same entity: a clean no-op ─────────────────
	creates, patches, grants, plans, escs := len(fe.createdRisks), len(fe.riskPatches), len(fe.grantPosts), len(fe.planPatches), len(fe.escPosts)

	rep2 := runPipeline(t, fe, rows, rd)

	if rep2.migrated != 0 || rep2.skipped != 5 {
		t.Errorf("resume: migrated=%d skipped=%d, want 0 and 5", rep2.migrated, rep2.skipped)
	}
	if len(fe.createdRisks) != creates || len(fe.riskPatches) != patches ||
		len(fe.grantPosts) != grants || len(fe.planPatches) != plans || len(fe.escPosts) != escs {
		t.Errorf("resume wrote something new: creates %d→%d patches %d→%d grants %d→%d plans %d→%d escs %d→%d",
			creates, len(fe.createdRisks), patches, len(fe.riskPatches), grants, len(fe.grantPosts),
			plans, len(fe.planPatches), escs, len(fe.escPosts))
	}
}

// statusHopsByRisk replays the recorded PATCHes into per-risk hop lists. The
// fake records patch bodies in order and applies them to fe.risks, so the
// order of expected-status transitions is faithful.
func (fe *fakeEntity) statusHopsByRisk() map[int][]string {
	// The fake doesn't tag patches with a risk id, so reconstruct from the
	// ExpectedStatus chain: each IN_REMEDIATION/CLOSED walk starts at
	// PENDING_RISK_OWNER_APPROVAL. Group consecutive patches into walks.
	out := map[int][]string{}
	risk := 0
	for _, pb := range fe.riskPatches {
		if pb.WorkflowStatus == nil {
			continue
		}
		if pb.ExpectedStatus != nil && *pb.ExpectedStatus == "PENDING_RISK_OWNER_APPROVAL" {
			risk++ // a new walk
		}
		out[risk] = append(out[risk], *pb.WorkflowStatus)
	}
	return out
}

func findingsForRow(fs []Finding, mig int, code string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.MigrationID == mig && f.Failure == code {
			out = append(out, f)
		}
	}
	return out
}

func findingsForMig(fs []Finding, mig int) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.MigrationID == mig {
			out = append(out, f)
		}
	}
	return out
}
