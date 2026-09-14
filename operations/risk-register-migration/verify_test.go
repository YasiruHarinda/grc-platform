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
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func intptr(i int) *int { return &i }

// verifyBaseRow is a hand-built IN_REMEDIATION row, resolved against
// fixtureRefData's ids, used to drive migrateRow directly (bypassing CSV
// parsing) so each sub-test can tamper with exactly one piece of the fake
// entity's resulting state.
func verifyBaseRow() Row {
	return Row{
		MigrationID: 1, CSVLine: 2,
		RiskYear: 2025, RiskQuarter: "Q3",
		SourceRegisterID: 1, // Asgardeo (fixtureRefData)
		AssignmentTeamID: 4, // Business
		RiskTitle:        "Verify test risk",
		RiskDescription:  "a description",
		ComplianceRefIDs: []int{40}, // ISO
		RiskCategoryIDs:  []int{30}, // Access Control & Credentials
		Likelihood:       2, Impact: 2,
		ImpactDescription:  "impact desc",
		ImplementationDate: "2025-06-30", // overdue vs migrationDate 2026-09-15
		ReassessmentDate:   "2025-12-01",
		AssignerID:         601, OwnerID: 602, ManagementApproverID: 603, ActionOwnerID: intptr(604),
		ActionPlanDescription: "the plan", ActionSteps: []string{"step one", "step two"},
		TreatmentStrategy: "REMEDIATE",
		WorkflowStatus:    "IN_REMEDIATION",
	}
}

// verifyBaseClosedRow is the CLOSED-bucket counterpart, for the plan
// completed-date check that only applies to that bucket.
func verifyBaseClosedRow() Row {
	row := verifyBaseRow()
	row.MigrationID = 2
	row.RiskTitle = "Verify test risk (closed)"
	row.ActionOwnerID = nil // optional for CLOSED
	row.WorkflowStatus = "CLOSED"
	return row
}

// setupVerifyFixture drives row through migrateRow against a fresh fake
// entity — the same production code path a real run uses — and returns the
// resulting risk id plus the fake/client for tampering and re-verification.
func setupVerifyFixture(t *testing.T, rd RefData, row Row) (*fakeEntity, *EntityClient, int) {
	t.Helper()
	fe := newFakeEntity(t)
	ec := fe.client(t)
	rep := NewReport()
	beforeID := fe.nextRiskID
	if err := migrateRow(context.Background(), ec, Config{MigrationDate: "2026-09-15"}, rd, row, ResumeState{Progress: ProgressNone}, rep); err != nil {
		t.Fatalf("migrateRow: %v", err)
	}
	if len(rep.findings) != 0 {
		t.Fatalf("unexpected findings setting up baseline: %+v", rep.findings)
	}
	if fe.nextRiskID == beforeID {
		t.Fatalf("migrateRow did not create a risk")
	}
	return fe, ec, fe.nextRiskID
}

func mismatchFields(ms []fieldMismatch) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.field
	}
	return out
}

func TestVerifyRow_CleanBaselineHasNoMismatches(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseRow()
	_, ec, riskID := setupVerifyFixture(t, rd, row)

	got, err := verifyRow(context.Background(), ec, rd, "2026-09-15", row, riskID, map[int][]Grant{})
	if err != nil {
		t.Fatalf("verifyRow: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unexpected mismatches on a clean baseline: %+v", got)
	}
}

func TestVerifyRow_ClosedBaselineHasNoMismatches(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseClosedRow()
	_, ec, riskID := setupVerifyFixture(t, rd, row)

	got, err := verifyRow(context.Background(), ec, rd, "2026-09-15", row, riskID, map[int][]Grant{})
	if err != nil {
		t.Fatalf("verifyRow: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unexpected mismatches on a clean CLOSED baseline: %+v", got)
	}
}

// TestVerifyRow_DetectsFieldMismatches tampers with exactly one piece of the
// fake entity's post-migration state per case and confirms verifyRow reports
// exactly that field.
func TestVerifyRow_DetectsFieldMismatches(t *testing.T) {
	rd := fixtureRefData(t)

	tests := []struct {
		name      string
		row       Row
		tamper    func(fe *fakeEntity, riskID int)
		wantField string
	}{
		{
			name: "title diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.RiskTitle = "Some other title"
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Risk Title",
		},
		{
			name: "owner id diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.OwnerID = 999
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Risk Owner",
		},
		{
			name: "treatment strategy diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.TreatmentStrategy = ptr("ACCEPT")
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Treatment Strategy",
		},
		{
			name: "likelihood diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.Likelihood = 3
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Likelihood",
		},
		{
			name: "risk category set diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.RiskCategoryIDs = []int{31} // fixture has row created with 30
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Risk Category",
		},
		{
			name: "compliance reference set diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.ComplianceReferenceIDs = []int{41} // fixture has row created with 40
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Security Compliance Reference",
		},
		{
			name: "action steps diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.ActionSteps = []ActionStepInput{{Description: "a different single step"}}
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Action Steps",
		},
		{
			name: "action owner diverged",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				req := fe.createReqByRisk[riskID]
				req.ActionOwnerID = intptr(1)
				fe.createReqByRisk[riskID] = req
			},
			wantField: "Action Owner",
		},
		{
			name: "workflow status reverted underneath",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				fe.risks[riskID].WorkflowStatus = "PENDING_COMPLIANCE_REVIEW"
			},
			wantField: "Workflow Status",
		},
		{
			name: "suppressing escalation missing",
			row:  verifyBaseRow(), // overdue -> migrateRow created one
			tamper: func(fe *fakeEntity, riskID int) {
				fe.escalations[riskID] = nil
			},
			wantField: "Overdue Escalation",
		},
		{
			name: "owner grant missing",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				delete(fe.grants, 602) // OwnerID
			},
			wantField: "Grant",
		},
		{
			name: "closed action plan not completed",
			row:  verifyBaseClosedRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				fe.planStatus[riskID] = "IN_PROGRESS"
			},
			wantField: "Action Plan Status",
		},
		{
			name: "closed action plan completed date never set",
			row:  verifyBaseClosedRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				delete(fe.planCompletedDate, riskID)
			},
			wantField: "Action Plan Completed Date",
		},
		{
			name: "compliance approval date never set",
			row:  verifyBaseRow(),
			tamper: func(fe *fakeEntity, riskID int) {
				delete(fe.complianceApprovalDate, riskID)
			},
			wantField: "Compliance Approval Date",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fe, ec, riskID := setupVerifyFixture(t, rd, tc.row)
			tc.tamper(fe, riskID)

			got, err := verifyRow(context.Background(), ec, rd, "2026-09-15", tc.row, riskID, map[int][]Grant{})
			if err != nil {
				t.Fatalf("verifyRow: %v", err)
			}
			fields := mismatchFields(got)
			found := false
			for _, f := range fields {
				if f == tc.wantField {
					found = true
				}
			}
			if !found {
				t.Errorf("mismatches = %v, want one for field %q", fields, tc.wantField)
			}
		})
	}
}

func TestVerifyMigration_RiskMissingAfterRealRun(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseRow()
	fe := newFakeEntity(t) // empty — the risk was never actually created
	ec := fe.client(t)
	rep := NewReport()

	if err := verifyMigration(context.Background(), discardLogger(), ec, rd, "2026-09-15", []Row{row}, rep); err != nil {
		t.Fatalf("verifyMigration: %v", err)
	}
	got := findingsForRow(rep.findings, row.MigrationID, "risk missing")
	if len(got) != 1 || got[0].Severity != SevMismatch {
		t.Errorf("findings = %+v, want one MISMATCH 'risk missing'", rep.findings)
	}
}

func TestVerifyMigration_RejectedRowWithAMatchingRiskIsFlagged(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseRow()
	_, ec, _ := setupVerifyFixture(t, rd, row) // the risk genuinely exists

	// Treat the row as rejected despite the risk existing — simulates a CSV
	// re-export that now rejects a row an earlier CSV successfully migrated.
	rep := NewReport()
	rep.Add(Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
		Severity: SevReject, Failure: "test forced reject"})

	if err := verifyMigration(context.Background(), discardLogger(), ec, rd, "2026-09-15", []Row{row}, rep); err != nil {
		t.Fatalf("verifyMigration: %v", err)
	}
	got := findingsForRow(rep.findings, row.MigrationID, "rejected row has a risk")
	if len(got) != 1 || got[0].Severity != SevMismatch {
		t.Errorf("findings = %+v, want one MISMATCH 'rejected row has a risk'", rep.findings)
	}
}

func TestVerifyMigration_RejectedRowConfirmedAbsentIsSilent(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseRow()
	fe := newFakeEntity(t) // never created
	ec := fe.client(t)

	rep := NewReport()
	rep.Add(Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
		Severity: SevReject, Failure: "some csv problem"})

	if err := verifyMigration(context.Background(), discardLogger(), ec, rd, "2026-09-15", []Row{row}, rep); err != nil {
		t.Fatalf("verifyMigration: %v", err)
	}
	for _, f := range rep.findings {
		if f.Severity == SevMismatch {
			t.Errorf("unexpected mismatch for a correctly-rejected, correctly-absent row: %+v", f)
		}
	}
}

func TestVerifyMigration_UnexpectedGrantIsFlagged(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseRow()
	fe, ec, _ := setupVerifyFixture(t, rd, row)

	// A marker-created grant on the owner (user 602) that no migratable row
	// expects — e.g. left over from an edit that lowered the row's score.
	fe.grants[602] = append(fe.grants[602], Grant{
		RoleID: rd.RoleIDByName[roleRiskManagement], ScopeType: "GLOBAL", ScopeID: 0, CreatedBy: marker,
	})

	rep := NewReport()
	if err := verifyMigration(context.Background(), discardLogger(), ec, rd, "2026-09-15", []Row{row}, rep); err != nil {
		t.Fatalf("verifyMigration: %v", err)
	}
	found := false
	for _, f := range rep.findings {
		if f.Severity == SevMismatch && f.Failure == "unexpected grant" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want a MISMATCH 'unexpected grant'", rep.findings)
	}
}

// TestVerifyMigration_CleanFixtureRun_NoMismatches runs the whole
// testdata/risks.csv register through the real pipeline and then verifies
// it — an end-to-end guard that the two production code paths (write,
// verify) agree on what a correct migration looks like.
func TestVerifyMigration_CleanFixtureRun_NoMismatches(t *testing.T) {
	rows, _ := loadFixture(t)
	rd := fixtureRefData(t)
	fe := newFakeEntity(t)

	rep := runPipeline(t, fe, rows, rd)

	if err := verifyMigration(context.Background(), discardLogger(), fe.client(t), rd, "2026-09-15", rows, rep); err != nil {
		t.Fatalf("verifyMigration: %v", err)
	}
	for _, f := range rep.findings {
		if f.Severity == SevMismatch {
			t.Errorf("unexpected mismatch on a clean fixture run: %+v", f)
		}
	}
}

// TestVerifyRow_ResumeWithDifferentMigrationDateIsNotAMismatch guards the
// bug found in review: migrateRow only sets the D9 audit dates once, on the
// run that first walks a row to that hop — a resume's status/plan PATCH is a
// no-op once already there (state.go's statusAtLeast guard), so an
// already-complete row's stored date reflects whichever earlier run first
// set it. verifyRow must not compare that stored date against the CURRENT
// run's migrationDate, or every already-correct row would false-positive
// whenever a resume is triggered with a different -migration-date.
func TestVerifyRow_ResumeWithDifferentMigrationDateIsNotAMismatch(t *testing.T) {
	rd := fixtureRefData(t)

	t.Run("IN_REMEDIATION", func(t *testing.T) {
		row := verifyBaseRow()
		_, ec, riskID := setupVerifyFixture(t, rd, row) // written with migrationDate 2026-09-15

		got, err := verifyRow(context.Background(), ec, rd, "2026-10-01", row, riskID, map[int][]Grant{}) // a later resume's date
		if err != nil {
			t.Fatalf("verifyRow: %v", err)
		}
		for _, m := range got {
			if m.field == "Compliance Approval Date" {
				t.Errorf("unexpected mismatch comparing a resumed row's stored date against a new run's migrationDate: %+v", m)
			}
		}
	})

	t.Run("CLOSED", func(t *testing.T) {
		row := verifyBaseClosedRow()
		_, ec, riskID := setupVerifyFixture(t, rd, row)

		got, err := verifyRow(context.Background(), ec, rd, "2026-10-01", row, riskID, map[int][]Grant{})
		if err != nil {
			t.Fatalf("verifyRow: %v", err)
		}
		for _, m := range got {
			if m.field == "Compliance Approval Date" || m.field == "Action Plan Completed Date" {
				t.Errorf("unexpected mismatch comparing a resumed row's stored date against a new run's migrationDate: %+v", m)
			}
		}
	})
}

// TestVerifyMigration_ExtraGrantDetectedEvenWhenRowNowRejected guards the
// second review finding: before the fix, expectedGrantsByUser was only
// populated from migratable rows, so a person who held a grant from an
// earlier CSV but whose row is now rejected was never looked up at all —
// their stale marker-created grant went undetected. Registering every
// resolved person named anywhere in the CSV (rejected rows included) as a
// zero-expectation candidate closes that gap.
func TestVerifyMigration_ExtraGrantDetectedEvenWhenRowNowRejected(t *testing.T) {
	rd := fixtureRefData(t)
	row := verifyBaseRow()
	_, ec, _ := setupVerifyFixture(t, rd, row) // creates the risk + the owner grant for user 602

	rep := NewReport()
	rep.Add(Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
		Severity: SevReject, Failure: "test forced reject"})

	if err := verifyMigration(context.Background(), discardLogger(), ec, rd, "2026-09-15", []Row{row}, rep); err != nil {
		t.Fatalf("verifyMigration: %v", err)
	}
	found := false
	for _, f := range rep.findings {
		if f.Severity == SevMismatch && f.Failure == "unexpected grant" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want a MISMATCH 'unexpected grant' for the now-orphaned owner grant", rep.findings)
	}
}
