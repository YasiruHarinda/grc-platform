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
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

// sheetTestRefData is a RefData with enough teams / categories / refs for the
// mapping tests (a superset of goodRefInputs).
func sheetTestRefData(t *testing.T) RefData {
	t.Helper()
	teams := []RiskTeam{
		{ID: 1, Name: "Asgardeo", Code: strptr("ASG"), Status: "ACTIVE"},
		{ID: 8, Name: "Legal", Code: nil, Status: "ACTIVE"},
	}
	cats := []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	refs := []ComplianceRef{{ID: 2, Name: "ISO"}, {ID: 5, Name: "SOC2"}, {ID: 6, Name: "HIPAA"}}
	scores := []RiskScore{{ID: 7, Likelihood: 3, Impact: 2}}
	rd, err := buildRefData(teams, cats, refs, scores, goodRoles())
	if err != nil {
		t.Fatalf("buildRefData: %v", err)
	}
	return rd
}

// buildCSV renders a header row from expectedHeaders plus the given data rows.
func buildCSV(t *testing.T, rows ...map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(expectedHeaders); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		rec := make([]string, len(expectedHeaders))
		for i, h := range expectedHeaders {
			rec[i] = r[h]
		}
		if err := w.Write(rec); err != nil {
			t.Fatal(err)
		}
	}
	w.Flush()
	return buf.String()
}

func findingsFor(fs []Finding, failure string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Failure == failure {
			out = append(out, f)
		}
	}
	return out
}

func TestMapRow_CleanRow(t *testing.T) {
	csvText := buildCSV(t, map[string]string{
		"Year":                          "2025.0",
		"Quarter":                       "Q3",
		"Source Register":               "Asgardeo",
		"Risk Title":                    "Supplier contract renewal delay",
		"Risk Description":              "Delay renewing supplier contracts.",
		"Security Compliance Reference": "HIPPA",
		"Risk Category":                 "Access Control & Credentials",
		"Risk Identified By":            "Employee",
		"Select Employee/ Name of External Person/ Tool": "Employee One",
		"Risk Identified Date":                           "45667.0",
		"Risk Assigned To":                               "User1@wso2.com",
		"Likelihood":                                     "3.0",
		"Impact":                                         "2.0",
		"Impact Description":                             "service interruption",
		"Implementation Date":                            "45838.0",
		"Reassessment Date":                              "30th Sep 2025",
		"Assignment Team":                                "Legal",
		"Risk Owner":                                     "user2@wso2.com",
		"Management Approver":                            "user3@wso2.com",
		"Action Owner":                                   "",
		"Action Plan Description":                        "Track to completion",
		"Action Steps":                                   "Action Item -1\nAction 2",
		"Treatment Strategy":                             "Accept",
		"Progress":                                       "WIP",
		"Git Issue URL":                                  "Text",
		"Email Subject":                                  "Risk identification on threat models",
		"Remarks":                                        "Text",
		"Workflow Status":                                "IN_REMEDIATION",
		"Migration ID":                                   "1",
	})

	rows, fs, err := parseSheet(strings.NewReader(csvText), sheetTestRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]

	if r.MigrationID != 1 || r.RiskYear != 2025 || r.RiskQuarter != "Q3" {
		t.Errorf("scalars: %+v", r)
	}
	if r.Likelihood != 3 || r.Impact != 2 {
		t.Errorf("likelihood/impact: %d/%d", r.Likelihood, r.Impact)
	}
	if r.TreatmentStrategy != "ACCEPT" || r.WorkflowStatus != "IN_REMEDIATION" || r.IdentifiedByType != "EMPLOYEE" {
		t.Errorf("enums: %q %q %q", r.TreatmentStrategy, r.WorkflowStatus, r.IdentifiedByType)
	}
	if r.ImplementationDate != "2025-06-30" || r.RiskIdentifiedDate != "2025-01-10" || r.ReassessmentDate != "2025-09-30" {
		t.Errorf("dates: impl=%q id=%q re=%q", r.ImplementationDate, r.RiskIdentifiedDate, r.ReassessmentDate)
	}
	if r.SourceRegisterID != 1 || r.AssignmentTeamID != 8 {
		t.Errorf("team ids: sr=%d at=%d", r.SourceRegisterID, r.AssignmentTeamID)
	}
	if len(r.RiskCategoryIDs) != 1 || r.RiskCategoryIDs[0] != 3 {
		t.Errorf("category ids: %v", r.RiskCategoryIDs)
	}
	if len(r.ComplianceRefIDs) != 1 || r.ComplianceRefIDs[0] != 6 {
		t.Errorf("compliance ids: %v (HIPPA should alias to HIPAA id 6)", r.ComplianceRefIDs)
	}
	if r.AssignerEmail != "user1@wso2.com" {
		t.Errorf("assigner email not lowercased: %q", r.AssignerEmail)
	}
	if r.GitIssueURL != "" {
		t.Errorf("git URL placeholder not dropped: %q", r.GitIssueURL)
	}
	if len(r.ActionSteps) != 2 {
		t.Errorf("action steps: %v", r.ActionSteps)
	}

	// The only finding for this row is the dropped "Text" git URL — a WARN.
	if len(fs) != 1 || fs[0].Severity != SevWarn || fs[0].Failure != "Git Issue URL" {
		t.Fatalf("findings = %+v, want a single Git Issue URL WARN", fs)
	}
}

func TestMapRow_MessyRowRejects(t *testing.T) {
	csvText := buildCSV(t, map[string]string{
		"Year":                          "twenty",
		"Quarter":                       "Q9",
		"Source Register":               "Nowhere",
		"Risk Title":                    "",
		"Security Compliance Reference": "ISO, BOGUS",
		"Risk Category":                 "Unknown Category",
		"Risk Identified By":            "Robot",
		"Risk Identified Date":          "1/2/2024",
		"Risk Assigned To":              "x@wso2.com",
		"Likelihood":                    "5.0",
		"Impact":                        "0",
		"Implementation Date":           "",
		"Reassessment Date":             "whenever",
		"Assignment Team":               "",
		"Risk Owner":                    "y@wso2.com",
		"Management Approver":           "z@wso2.com",
		"Treatment Strategy":            "",
		"Git Issue URL":                 "https://github.com/o/r/issues/9",
		"Workflow Status":               "PENDING",
		"Migration ID":                  "2",
	})

	rows, fs, err := parseSheet(strings.NewReader(csvText), sheetTestRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	if len(rows) != 1 || rows[0].MigrationID != 2 {
		t.Fatalf("rows = %+v", rows)
	}

	wantReject := []string{
		"Year", "Quarter", "Risk Title", "Source Register", "Risk Category",
		"Likelihood", "Impact", "Implementation Date", "Assignment Team",
		"Treatment Strategy", "Workflow Status", "Security Compliance Reference",
	}
	for _, f := range wantReject {
		got := findingsFor(fs, f)
		if len(got) == 0 || got[0].Severity != SevReject {
			t.Errorf("missing REJECT for %q; findings = %+v", f, fs)
		}
		if got[0].MigrationID != 2 {
			t.Errorf("finding for %q has MigrationID %d, want 2", f, got[0].MigrationID)
		}
	}

	for _, f := range []string{"Risk Identified By", "Reassessment Date"} {
		got := findingsFor(fs, f)
		if len(got) == 0 || got[0].Severity != SevWarn {
			t.Errorf("missing WARN for %q; findings = %+v", f, fs)
		}
	}

	// A valid git URL on an otherwise-rejected row is still captured.
	if rows[0].GitIssueURL != "https://github.com/o/r/issues/9" {
		t.Errorf("git URL = %q", rows[0].GitIssueURL)
	}

	// The row is excluded from migration.
	rep := NewReport()
	rep.Add(fs...)
	if _, bad := rep.RejectedMigrationIDs()[2]; !bad {
		t.Error("migration id 2 should be in RejectedMigrationIDs")
	}
}

func TestParseSheet_DuplicateMigrationIDRejects(t *testing.T) {
	// Two otherwise-valid rows that differ on the natural key (title) but share
	// a Migration ID — the natural-key collision check in reconstructState would
	// miss this, so parseSheet has to catch it.
	base := map[string]string{
		"Year": "2025", "Quarter": "Q3", "Source Register": "Asgardeo",
		"Security Compliance Reference": "ISO", "Risk Category": "Access Control & Credentials",
		"Risk Assigned To": "a@wso2.com", "Likelihood": "3", "Impact": "2",
		"Implementation Date": "45838.0", "Assignment Team": "Legal",
		"Risk Owner": "b@wso2.com", "Management Approver": "c@wso2.com",
		"Action Plan Description": "x", "Action Steps": "step 1",
		"Treatment Strategy": "Accept", "Workflow Status": "IN_REMEDIATION",
	}
	rowA := map[string]string{}
	rowB := map[string]string{}
	for k, v := range base {
		rowA[k], rowB[k] = v, v
	}
	rowA["Risk Title"], rowA["Migration ID"] = "First risk", "5"
	rowB["Risk Title"], rowB["Migration ID"] = "Second, unrelated risk", "5"

	rows, fs, err := parseSheet(strings.NewReader(buildCSV(t, rowA, rowB)), sheetTestRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	dup := findingsFor(fs, "Migration ID")
	if len(dup) != 1 || dup[0].Severity != SevReject {
		t.Fatalf("want one Migration ID REJECT, got %+v", fs)
	}
	if dup[0].CSVRow != 3 || dup[0].MigrationID != 5 {
		t.Errorf("dup finding = %+v, want CSVRow 3 / MigrationID 5", dup[0])
	}
	if !strings.Contains(dup[0].Detail, "line 2") {
		t.Errorf("detail = %q, want it to point at line 2", dup[0].Detail)
	}

	// Because RejectedMigrationIDs is keyed by ID, both rows are excluded.
	rep := NewReport()
	rep.Add(fs...)
	if _, bad := rep.RejectedMigrationIDs()[5]; !bad {
		t.Error("migration id 5 should be in RejectedMigrationIDs")
	}
}

func TestParseSheet_SkipsLegendAndBlankRows(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(expectedHeaders)
	legend := make([]string, len(expectedHeaders))
	legend[0] = "Required fields are coloured in yellow"
	_ = w.Write(legend)
	_ = w.Write(make([]string, len(expectedHeaders))) // fully blank
	w.Flush()

	rows, fs, err := parseSheet(strings.NewReader(buf.String()), sheetTestRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	if len(rows) != 0 || len(fs) != 0 {
		t.Fatalf("legend/blank rows not skipped: rows=%+v findings=%+v", rows, fs)
	}
}
