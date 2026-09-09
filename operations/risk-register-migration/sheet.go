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
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Row is one register row after header-name mapping and value transforms
// (plan §6). Person columns hold work emails at this point; the *ID fields are
// filled later by the resolver (resolve.go).
type Row struct {
	MigrationID int // added "Migration ID" column — errors.csv correlation only
	CSVLine     int // 1-based line in the source file, for findings

	RiskYear           int
	RiskQuarter        string // Q1..Q4
	SourceRegister     string // raw name/code from the sheet
	AssignmentTeam     string // raw name/code from the sheet
	RiskTitle          string
	RiskDescription    string
	ComplianceRefs     []string // raw tokens, pre-lookup
	RiskCategory       string   // raw name, pre-lookup
	IdentifiedByType   string   // EMPLOYEE | EXTERNAL_PERSON | TOOL | ""
	IdentifiedByName   string
	RiskIdentifiedDate string // YYYY-MM-DD | ""
	Likelihood         int    // 1..3
	Impact             int    // 1..3
	ImpactDescription  string
	ImplementationDate string // YYYY-MM-DD (required — see parseSheet)
	ReassessmentDate   string // YYYY-MM-DD | ""

	AssignerEmail           string
	OwnerEmail              string
	ManagementApproverEmail string
	ActionOwnerEmail        string // "" allowed only for CLOSED rows

	ActionPlanDescription string
	ActionSteps           []string
	TreatmentStrategy     string // ACCEPT | TRANSFER | REMEDIATE | AVOID
	Progress              string
	GitIssueURL           string
	EmailSubject          string
	Remarks               string

	WorkflowStatus string // IN_REMEDIATION | CLOSED

	// ── filled by resolve.go ────────────────────────────────────────────────
	SourceRegisterID     int
	AssignmentTeamID     int
	ComplianceRefIDs     []int
	RiskCategoryIDs      []int
	AssignerID           int
	OwnerID              int
	ManagementApproverID int
	ActionOwnerID        *int
}

// actionStepPlaceholder stands in when the source register row has no action
// steps — the entity requires at least one (see mapRow).
const actionStepPlaceholder = "Migrated from the historical risk register; original action steps were not recorded."

// expectedHeaders are matched case- and space-insensitively (plan §4). The two
// trailing entries are the operator-added columns.
var expectedHeaders = []string{
	"Year", "Quarter", "Source Register", "Risk Title", "Risk Description",
	"Security Compliance Reference", "Risk Category", "Risk Identified By",
	"Select Employee/ Name of External Person/ Tool", "Risk Identified Date",
	"Risk Assigned To", "Likelihood", "Impact", "Impact Description",
	"Implementation Date", "Reassessment Date", "Assignment Team", "Risk Owner",
	"Management Approver", "Action Owner", "Action Plan Description",
	"Action Steps", "Treatment Strategy", "Progress", "Git Issue URL",
	"Email Subject", "Remarks",
	"Workflow Status", "Migration ID",
}

// parseSheet reads the CSV, maps every row, and applies the value rules in §6.
// A structural problem (bad/short header, unreadable file) returns an error and
// aborts the run; a row-level problem becomes a REJECT/WARN Finding and the row
// is still returned (callers drop REJECTed rows via the Report).
func parseSheet(r io.Reader, refs RefData) ([]Row, []Finding, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	records, err := cr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("csv is empty")
	}

	idx, err := mapHeader(records[0])
	if err != nil {
		return nil, nil, err
	}

	var rows []Row
	var findings []Finding
	seenID := map[int]int{} // MigrationID -> first CSV line that used it
	for i, rec := range records[1:] {
		line := i + 2 // 1-based, past the header
		if isSkippable(rec, idx) {
			continue
		}
		row, rf := mapRow(rec, idx, line, refs)
		findings = append(findings, rf...)
		// A repeated Migration ID silently collides downstream: reconstructState
		// keys resume state by it (map[int]ResumeState) and RejectedMigrationIDs
		// is a map[int]struct{}. REJECT every row that shares an ID — the natural
		// key check can't catch two genuinely different risks with a mistyped ID.
		if row.MigrationID != 0 {
			if first, dup := seenID[row.MigrationID]; dup {
				findings = append(findings, Finding{
					MigrationID: row.MigrationID, CSVRow: line, RiskTitle: row.RiskTitle,
					Severity: SevReject, Failure: "Migration ID",
					Detail:   fmt.Sprintf("duplicate of the row on line %d", first),
				})
			} else {
				seenID[row.MigrationID] = line
			}
		}
		rows = append(rows, row)
	}
	return rows, findings, nil
}

// mapHeader returns a column-name → index map, tolerating trailing spaces and
// case. It errors if any expected column is missing.
func mapHeader(header []string) (map[string]int, error) {
	got := make(map[string]int, len(header))
	for i, h := range header {
		got[normHeader(h)] = i
	}
	var missing []string
	for _, want := range expectedHeaders {
		if _, ok := got[normHeader(want)]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("csv is missing column(s): %s", strings.Join(missing, "; "))
	}
	return got, nil
}

func normHeader(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// isSkippable drops the legend row and fully blank rows (plan §4).
func isSkippable(rec []string, idx map[string]int) bool {
	joined := strings.ToLower(strings.Join(rec, ""))
	if strings.TrimSpace(joined) == "" {
		return true
	}
	if strings.Contains(joined, "required fields are coloured") {
		return true
	}
	// A row with neither a Year nor a Risk Title is not a data row.
	return get(rec, idx, "Year") == "" && get(rec, idx, "Risk Title") == ""
}

// mapRow applies every rule in plan §6: the value transforms in transform.go
// and the reference-data lookups against refs (loaded in preflight, T3). A
// row-level problem becomes a REJECT or WARN Finding; the Row is still returned
// (callers drop REJECTed rows via the Report). People columns are left as
// lowercased emails here — resolve.go (T5) turns them into user ids.
func mapRow(rec []string, idx map[string]int, line int, refs RefData) (Row, []Finding) {
	row := Row{CSVLine: line}
	var fs []Finding
	title := strings.TrimSpace(get(rec, idx, "Risk Title"))
	reject := func(field, detail string) {
		fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: line, RiskTitle: title,
			Severity: SevReject, Failure: field, Detail: detail})
	}
	warn := func(field, detail string) {
		fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: line, RiskTitle: title,
			Severity: SevWarn, Failure: field, Detail: detail})
	}

	// Migration ID first, so every finding below can carry it.
	if n, err := parseIntish(get(rec, idx, "Migration ID")); err == nil {
		row.MigrationID = n
	} else {
		reject("Migration ID", "missing or non-integer")
	}

	// ── verbatim text ──────────────────────────────────────────────────────
	row.RiskTitle = title
	if row.RiskTitle == "" {
		reject("Risk Title", "empty")
	}
	row.RiskDescription = strings.TrimSpace(get(rec, idx, "Risk Description"))
	row.ImpactDescription = strings.TrimSpace(get(rec, idx, "Impact Description"))
	row.Progress = strings.TrimSpace(get(rec, idx, "Progress"))
	row.EmailSubject = strings.TrimSpace(get(rec, idx, "Email Subject"))
	row.Remarks = strings.TrimSpace(get(rec, idx, "Remarks"))
	row.IdentifiedByName = strings.TrimSpace(get(rec, idx, "Select Employee/ Name of External Person/ Tool"))
	row.ActionPlanDescription = strings.TrimSpace(get(rec, idx, "Action Plan Description"))
	row.ActionSteps = splitSteps(get(rec, idx, "Action Steps"))
	if len(row.ActionSteps) == 0 {
		// The entity's POST /risks rejects a stepless plan (a plan with no steps
		// can never be marked COMPLETED). Insert a placeholder and flag it.
		row.ActionSteps = []string{actionStepPlaceholder}
		warn("Action Steps", "no action steps in the source register — a placeholder step was inserted (the entity requires at least one)")
	}

	// ── people (emails; resolved to ids in resolve.go, T5) ─────────────────
	row.AssignerEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Risk Assigned To")))
	row.OwnerEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Risk Owner")))
	row.ManagementApproverEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Management Approver")))
	row.ActionOwnerEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Action Owner")))

	// ── parsed scalars ────────────────────────────────────────────────────
	if y, err := parseYear(get(rec, idx, "Year")); err == nil {
		row.RiskYear = y
	} else {
		reject("Year", err.Error())
	}
	if q, err := parseQuarter(get(rec, idx, "Quarter")); err == nil {
		row.RiskQuarter = q
	} else {
		reject("Quarter", err.Error())
	}
	if l, err := parseScore(get(rec, idx, "Likelihood")); err == nil {
		row.Likelihood = l
	} else {
		reject("Likelihood", err.Error())
	}
	if im, err := parseScore(get(rec, idx, "Impact")); err == nil {
		row.Impact = im
	} else {
		reject("Impact", err.Error())
	}
	if ts, err := mapTreatment(get(rec, idx, "Treatment Strategy")); err == nil {
		row.TreatmentStrategy = ts
	} else {
		reject("Treatment Strategy", err.Error())
	}
	if ws, err := mapWorkflowStatus(get(rec, idx, "Workflow Status")); err == nil {
		row.WorkflowStatus = ws
	} else {
		reject("Workflow Status", err.Error())
	}
	if typ, ok := mapIdentifiedByType(get(rec, idx, "Risk Identified By")); ok {
		row.IdentifiedByType = typ
	} else if raw := strings.TrimSpace(get(rec, idx, "Risk Identified By")); raw == "" {
		warn("Risk Identified By", "blank — identified_by_type left null")
	} else {
		warn("Risk Identified By", fmt.Sprintf("unrecognised value %q — identified_by_type left null", raw))
	}

	// ── dates ─────────────────────────────────────────────────────────────
	if d, err := parseDate(get(rec, idx, "Implementation Date")); err != nil {
		reject("Implementation Date", err.Error())
	} else if d == "" {
		reject("Implementation Date", "empty — a real deadline is required")
	} else {
		row.ImplementationDate = d
	}
	if d, err := parseDate(get(rec, idx, "Risk Identified Date")); err != nil {
		warn("Risk Identified Date", err.Error()+" — left null")
	} else {
		row.RiskIdentifiedDate = d
	}
	if d, err := parseDate(get(rec, idx, "Reassessment Date")); err != nil {
		warn("Reassessment Date", err.Error()+" — left null")
	} else {
		row.ReassessmentDate = d
	}

	// ── git issue url (placeholder scrub) ────────────────────────────────
	if raw := strings.TrimSpace(get(rec, idx, "Git Issue URL")); raw != "" {
		if u, ok := scrubHTTPURL(raw); ok {
			row.GitIssueURL = u
		} else {
			warn("Git Issue URL", fmt.Sprintf("%q is not an http(s) URL — dropped", raw))
		}
	}

	// ── reference-data lookups (RefData from preflight, T3) ──────────────
	row.ComplianceRefs = splitTokens(get(rec, idx, "Security Compliance Reference"))
	for _, tok := range row.ComplianceRefs {
		if id, ok := refs.ComplianceIDByName[normalizeComplianceToken(tok)]; ok {
			row.ComplianceRefIDs = append(row.ComplianceRefIDs, id)
		} else {
			reject("Security Compliance Reference", fmt.Sprintf("unknown reference %q", tok))
		}
	}

	if cat := strings.TrimSpace(get(rec, idx, "Risk Category")); cat == "" {
		reject("Risk Category", "empty")
	} else {
		row.RiskCategory = cat
		if id, ok := refs.CategoryIDByName[strings.ToLower(cat)]; ok {
			row.RiskCategoryIDs = []int{id}
		} else {
			reject("Risk Category", fmt.Sprintf("unknown category %q", cat))
		}
	}

	if sr := strings.TrimSpace(get(rec, idx, "Source Register")); sr == "" {
		reject("Source Register", "empty")
	} else {
		row.SourceRegister = sr
		if id, ok := refs.TeamIDByKey[strings.ToLower(sr)]; ok {
			row.SourceRegisterID = id
			if refs.TeamCodeByID[id] == "" {
				reject("Source Register", fmt.Sprintf("team %q has no code and cannot be a source register", sr))
			}
		} else {
			reject("Source Register", fmt.Sprintf("unknown team %q", sr))
		}
	}

	if at := strings.TrimSpace(get(rec, idx, "Assignment Team")); at == "" {
		reject("Assignment Team", "empty")
	} else {
		row.AssignmentTeam = at
		if id, ok := refs.TeamIDByKey[strings.ToLower(at)]; ok {
			row.AssignmentTeamID = id
		} else {
			reject("Assignment Team", fmt.Sprintf("unknown team %q", at))
		}
	}

	return row, fs
}

// splitTokens splits a compliance-reference cell on comma / semicolon / newline.
func splitTokens(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(f))
	for _, t := range f {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// splitSteps splits an action-steps cell on newlines, keeping any "1." / "2."
// prefixes as written.
func splitSteps(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func get(rec []string, idx map[string]int, col string) string {
	i, ok := idx[normHeader(col)]
	if !ok || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func sortByMigrationID(rows []Row) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].MigrationID < rows[j].MigrationID })
}
