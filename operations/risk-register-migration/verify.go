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

// verify.go re-reads the entity after every real run and diffs it against the
// CSV, so a run's report answers "does the CSV match the entity right now",
// not just "did this run's own writes succeed". It runs unconditionally after
// the write loop (main.go), covering every migratable row — including ones
// already complete from an earlier run and Skipped this time — plus a cheap
// negative check that every rejected row still has no matching risk.
//
// It reads exclusively through the compliance-entity API, the same as the
// rest of this tool: never MySQL directly (see main.go's package comment).
// Findings land as Severity SevMismatch in the same Report as REJECT/WARN, so
// a mismatch shows up in errors.csv/report.txt and trips the existing
// exitFindings exit code — no new exit code, no new CLI flag.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
)

// verifyProgressEvery controls how often verifyMigration logs a progress line
// while checking migratable rows. Verification adds a GET /detail (+
// escalations/grants) round trip per row on top of the write loop that just
// ran; without a periodic line, a large real run can go quiet for a while
// after "risk-register-import starting" — indistinguishable, from the log
// alone, from a hang.
const verifyProgressEvery = 10

// mismatchFinding builds one SevMismatch Finding for row, carrying the same
// MigrationID/CSVRow/RiskTitle triple every verifyMigration finding needs.
func mismatchFinding(row Row, failure, detail string) Finding {
	return Finding{
		MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
		Severity: SevMismatch, Failure: failure, Detail: detail,
	}
}

// verifyMigration is the verification pass (plan §verification). Called once,
// after the write loop, on every real run.
func verifyMigration(ctx context.Context, log *slog.Logger, ec *EntityClient, rd RefData, migrationDate string, allRows []Row, rep *Report) error {
	rejected := rep.RejectedMigrationIDs()

	existing, err := searchMarkerRisks(ctx, ec, allRows)
	if err != nil {
		return fmt.Errorf("verification: search marker risks: %w", err)
	}
	risksByKey := map[string][]Risk{}
	for _, r := range existing {
		k := naturalKey(r.RiskTitle, r.SourceRegID, r.RiskYear, r.RiskQuarter)
		risksByKey[k] = append(risksByKey[k], r)
	}

	migratableTotal := 0
	for _, row := range allRows {
		if _, bad := rejected[row.MigrationID]; !bad {
			migratableTotal++
		}
	}

	var (
		okCount, mismatchCount, rejectConfirmed, checked int
	)
	// expectedGrantsByUser accumulates, across every migratable row, the D10
	// grants any user is expected to hold — so the unexpected-extra-grant pass
	// below can tell "not expected by THIS row" from "not expected by any row"
	// for a person who is owner/assigner/mgmt-approver on more than one row.
	expectedGrantsByUser := map[int]map[string]struct{}{}
	// grantCache is shared across every verifyRow call and verifyNoExtraGrants
	// below, keyed by user id. Without it, an owner/assigner/mgmt-approver who
	// appears on many IN_REMEDIATION rows gets ListGrants called once per row
	// (verifyRow used to keep its own row-scoped map) plus once more in
	// verifyNoExtraGrants — this fetches each user's grants at most once for
	// the whole verification pass.
	grantCache := map[int][]Grant{}

	for _, row := range allRows {
		key := naturalKey(row.RiskTitle, row.SourceRegisterID, row.RiskYear, row.RiskQuarter)
		matches := risksByKey[key]

		if _, bad := rejected[row.MigrationID]; bad {
			if len(matches) > 0 {
				mismatchCount++
				rep.Add(mismatchFinding(row, "rejected row has a risk",
					fmt.Sprintf("CSV row was rejected but risk id %d exists in the entity (workflow_status=%s)",
						matches[0].ID, matches[0].WorkflowStatus)))
			} else {
				rejectConfirmed++
			}
			continue
		}

		checked++
		if checked%verifyProgressEvery == 0 || checked == migratableTotal {
			log.Info("verifying", "checked", checked, "of", migratableTotal)
		}

		for _, g := range expectedGrants(row, rd) {
			if expectedGrantsByUser[g.userID] == nil {
				expectedGrantsByUser[g.userID] = map[string]struct{}{}
			}
			expectedGrantsByUser[g.userID][grantKey(g.roleID, g.scopeType, g.scopeID)] = struct{}{}
		}

		switch len(matches) {
		case 0:
			mismatchCount++
			rep.Add(mismatchFinding(row, "risk missing", "no matching risk found in the entity after a real run"))
		case 1:
			mismatches, err := verifyRow(ctx, ec, rd, migrationDate, row, matches[0].ID, grantCache)
			if err != nil {
				// verifyRow only ever reads (GetRiskDetail/ListEscalations/
				// ListGrants/ListActionPlans); a transient failure here means
				// this row couldn't be checked, not that the entity is wrong.
				// Record it and keep verifying the rest — a 500 on row 50 of
				// 500 shouldn't turn rows 51+ into a silent, unverified gap.
				log.Warn("could not verify row", "riskId", matches[0].ID, "migrationId", row.MigrationID, "err", err)
				mismatchCount++
				rep.Add(mismatchFinding(row, "verification read failed",
					fmt.Sprintf("risk %d: %v", matches[0].ID, err)))
				continue
			}
			if len(mismatches) == 0 {
				okCount++
				continue
			}
			mismatchCount += len(mismatches)
			for _, m := range mismatches {
				rep.Add(mismatchFinding(row, m.field, fmt.Sprintf("expected %s, got %s", m.expected, m.actual)))
			}
		default:
			mismatchCount++
			rep.Add(mismatchFinding(row, "ambiguous risk match",
				fmt.Sprintf("%d marker risks match this row's natural key", len(matches))))
		}
	}

	// Register every resolved person named anywhere in the CSV — including on
	// a rejected or CLOSED row — as a grant-check candidate, even though they
	// have no expected grants (an empty set is still registered). Without
	// this, a person who held a grant from an earlier CSV but is now on a
	// rejected row (or whose row is now CLOSED, or ACCEPT-but-no-longer-high)
	// would never be looked up at all, and a stale marker-created grant on
	// them would go undetected. This still can't see a person dropped from
	// the CSV entirely: compliance-entity has no bulk "every marker-created
	// grant" query, only per-user GET /grants/user/{id}.
	for _, row := range allRows {
		for _, uid := range []int{row.OwnerID, row.AssignerID, row.ManagementApproverID} {
			if uid == 0 {
				continue
			}
			if expectedGrantsByUser[uid] == nil {
				expectedGrantsByUser[uid] = map[string]struct{}{}
			}
		}
	}

	extra, err := verifyNoExtraGrants(ctx, ec, expectedGrantsByUser, grantCache)
	if err != nil {
		return err
	}
	mismatchCount += len(extra)
	rep.Add(extra...)

	log.Info("verification complete",
		"verifiedOk", okCount, "mismatches", mismatchCount, "rejectedConfirmedAbsent", rejectConfirmed)
	return nil
}

// verifyNoExtraGrants flags a marker-created grant that isn't expected by any
// migratable row for that user — e.g. a stale grant left over from a CSV that
// used to grant management approval on a since-lowered score. Only ever
// inspects createdBy==marker rows: a grant from unrelated platform activity
// (an admin, another migration) is never this tool's business.
func verifyNoExtraGrants(ctx context.Context, ec *EntityClient, expectedByUser map[int]map[string]struct{}, grantCache map[int][]Grant) ([]Finding, error) {
	userIDs := make([]int, 0, len(expectedByUser))
	for id := range expectedByUser {
		userIDs = append(userIDs, id)
	}
	sort.Ints(userIDs)

	var out []Finding
	for _, userID := range userIDs {
		grants, fetched := grantCache[userID]
		if !fetched {
			var err error
			grants, err = ec.ListGrants(ctx, userID)
			if err != nil {
				return nil, fmt.Errorf("verification: list grants for user %d: %w", userID, err)
			}
			grantCache[userID] = grants
		}
		expected := expectedByUser[userID]
		for _, g := range grants {
			if g.CreatedBy != marker {
				continue
			}
			key := grantKey(g.RoleID, g.ScopeType, g.ScopeID)
			if _, want := expected[key]; want {
				continue
			}
			out = append(out, Finding{
				// This finding is about a person, not a CSV row — CSVRow: -1
				// is a sentinel so it reads as "no row" in errors.csv rather
				// than as csv_row 0 (report.go sorts MigrationID then CSVRow
				// ascending, so a real zero value would also read as row one
				// of the file). The user id is in Detail below.
				CSVRow:   -1,
				Severity: SevMismatch, Failure: "unexpected grant",
				Detail: fmt.Sprintf("user %d holds a migration-created grant (role=%d scope=%s/%d) not expected by any migratable row",
					userID, g.RoleID, g.ScopeType, g.ScopeID),
			})
		}
	}
	return out, nil
}

// fieldMismatch is one field-level disagreement found by verifyRow.
type fieldMismatch struct {
	field, expected, actual string
}

// verifyRow fetches riskID's full detail (+ escalations, + the STANDARD
// plan's completed date for a CLOSED row) and diffs every field the migration
// is responsible for against row. It never fetches missing-grant state for
// non-IN_REMEDIATION rows (CLOSED gets none) and leaves the unexpected-extra
// grant check to verifyNoExtraGrants, which needs the cross-row view.
func verifyRow(ctx context.Context, ec *EntityClient, rd RefData, migrationDate string, row Row, riskID int, grantCache map[int][]Grant) ([]fieldMismatch, error) {
	detail, err := ec.GetRiskDetail(ctx, riskID)
	if err != nil {
		return nil, err
	}

	var out []fieldMismatch
	add := func(field, expected, actual string) {
		if expected != actual {
			out = append(out, fieldMismatch{field, expected, actual})
		}
	}
	// addSet checks a field was set at all, rather than equal to a specific
	// value. Used for the two D9 audit dates: migrateRow only sets them once,
	// on the run that first walks the row to that hop, and a resume's status
	// PATCH is a no-op once already there (state.go's statusAtLeast guard) —
	// so a resumed row's date reflects whichever earlier run first set it,
	// not this run's -migration-date. Comparing for exact equality with the
	// current run's migrationDate would false-positive on every
	// already-complete row whenever a resume happens to use a different date.
	addSet := func(field, actual string) {
		if actual == "" {
			out = append(out, fieldMismatch{field, "a date (set once, on the run that first reached this status)", "empty"})
		}
	}

	add("Risk Title", row.RiskTitle, detail.RiskTitle)
	add("Risk Description", row.RiskDescription, strOrNil(detail.RiskDescription))
	add("Source Register", strconv.Itoa(row.SourceRegisterID), strconv.Itoa(detail.SourceRegisterID))
	add("Assignment Team", strconv.Itoa(row.AssignmentTeamID), strconv.Itoa(detail.AssignmentTeamID))
	add("Risk Assigned To", strconv.Itoa(row.AssignerID), strconv.Itoa(detail.AssignerID))
	add("Risk Owner", strconv.Itoa(row.OwnerID), strconv.Itoa(detail.OwnerID))
	add("Management Approver", strconv.Itoa(row.ManagementApproverID), strconv.Itoa(detail.ManagementApproverID))
	add("Workflow Status", row.WorkflowStatus, detail.WorkflowStatus)
	add("Treatment Strategy", row.TreatmentStrategy, strOrNil(detail.TreatmentStrategy))
	add("Implementation Date", row.ImplementationDate, strOrNil(detail.ImplementationDate))
	add("Reassessment Date", row.ReassessmentDate, strOrNil(detail.ReassessmentDate))
	add("Risk Identified Date", row.RiskIdentifiedDate, strOrNil(detail.RiskIdentifiedDate))
	add("Risk Identified By", row.IdentifiedByType, strOrNil(detail.IdentifiedByType))
	add("Select Employee/ Name of External Person/ Tool", row.IdentifiedByName, strOrNil(detail.IdentifiedByName))
	add("Impact Description", row.ImpactDescription, strOrNil(detail.ImpactDescription))
	add("Progress", row.Progress, strOrNil(detail.Progress))
	add("Git Issue URL", row.GitIssueURL, strOrNil(detail.GitIssueURL))
	add("Email Subject", row.EmailSubject, strOrNil(detail.EmailSubject))
	add("Remarks", row.Remarks, strOrNil(detail.Remarks))

	// D9 closure/compliance-approval audit fields: every bucket walks through
	// IN_REMEDIATION, so complianceApprovalDate is always set (to whichever
	// run's migration date first reached that hop — see addSet above);
	// complianceApprovalBy is never set via this tool's API path.
	addSet("Compliance Approval Date", strOrNil(detail.ComplianceApprovalDate))
	add("Compliance Approval By", "", intOrNil(detail.ComplianceApprovalBy))

	if detail.GrossScore == nil {
		out = append(out, fieldMismatch{"Gross Likelihood/Impact", fmt.Sprintf("%d/%d", row.GrossLikelihood, row.GrossImpact), "no gross score"})
	} else {
		add("Gross Likelihood", strconv.Itoa(row.GrossLikelihood), strconv.Itoa(detail.GrossScore.Likelihood))
		add("Gross Impact", strconv.Itoa(row.GrossImpact), strconv.Itoa(detail.GrossScore.Impact))
	}

	// The gross score alone doesn't prove the residual write happened — a row
	// whose Residual differs from its Gross (needsResidualAssessment) must
	// have a marker-authored risk_assessment carrying that exact value,
	// checked the same way migrateRow/fetchRiskState detect it on resume
	// (findMarkerAssessment), or the residual value silently never landed.
	if needsResidualAssessment(row) {
		assessments, err := ec.ListAssessments(ctx, riskID)
		if err != nil {
			return nil, err
		}
		a, found := findMarkerAssessment(assessments)
		if !found {
			out = append(out, fieldMismatch{"Residual Likelihood/Impact",
				fmt.Sprintf("%d/%d", row.ResidualLikelihood, row.ResidualImpact), "no residual assessment"})
		} else {
			add("Residual Likelihood", strconv.Itoa(row.ResidualLikelihood), strconv.Itoa(a.ResidualLikelihood))
			add("Residual Impact", strconv.Itoa(row.ResidualImpact), strconv.Itoa(a.ResidualImpact))
		}
	}

	if diff := diffIntSets(row.ComplianceRefIDs, complianceRefIDs(detail.ComplianceReferences)); diff != "" {
		out = append(out, fieldMismatch{"Security Compliance Reference", fmt.Sprint(row.ComplianceRefIDs), diff})
	}
	if diff := diffIntSets(row.RiskCategoryIDs, riskCategoryIDs(detail.RiskCategories)); diff != "" {
		out = append(out, fieldMismatch{"Risk Category", fmt.Sprint(row.RiskCategoryIDs), diff})
	}

	if detail.ActionPlan == nil {
		out = append(out, fieldMismatch{"Action Plan", "a STANDARD action plan", "none"})
	} else {
		add("Action Owner", intOrNil(row.ActionOwnerID), intOrNil(detail.ActionPlan.ActionOwnerID))
		add("Action Plan Description", row.ActionPlanDescription, strOrNil(detail.ActionPlan.Description))
		if diff := diffActionSteps(row.ActionSteps, detail.ActionPlan.Steps); diff != "" {
			out = append(out, fieldMismatch{"Action Steps", fmt.Sprint(row.ActionSteps), diff})
		}

		if row.WorkflowStatus == "CLOSED" {
			add("Action Plan Status", "COMPLETED", detail.ActionPlan.Status)
			plans, err := ec.ListActionPlans(ctx, riskID)
			if err != nil {
				return nil, err
			}
			addSet("Action Plan Completed Date", standardPlanCompletedDate(plans))
		}
	}

	escs, err := ec.ListEscalations(ctx, riskID)
	if err != nil {
		return nil, err
	}
	wantEscalation := wantsSuppressingEscalation(row, migrationDate)
	hasEscalation := hasOpenMarkerEscalation(escs)
	switch {
	case wantEscalation && !hasEscalation:
		out = append(out, fieldMismatch{"Overdue Escalation", "an OPEN suppressing escalation", "none"})
	case !wantEscalation && hasEscalation:
		out = append(out, fieldMismatch{"Overdue Escalation", "no suppressing escalation", "an OPEN marker escalation exists"})
	}

	if row.WorkflowStatus == "IN_REMEDIATION" {
		for _, want := range expectedGrants(row, rd) {
			if _, fetched := grantCache[want.userID]; !fetched {
				grants, err := ec.ListGrants(ctx, want.userID)
				if err != nil {
					return nil, err
				}
				grantCache[want.userID] = grants
			}
			if !hasGrant(grantCache[want.userID], want.roleID, want.scopeType, want.scopeID) {
				out = append(out, fieldMismatch{
					"Grant",
					grantKey(want.roleID, want.scopeType, want.scopeID),
					fmt.Sprintf("missing for user %d", want.userID),
				})
			}
		}
	}

	return out, nil
}

func hasGrant(grants []Grant, roleID int, scopeType string, scopeID int) bool {
	for _, g := range grants {
		if g.RoleID == roleID && g.ScopeType == scopeType && g.ScopeID == scopeID {
			return true
		}
	}
	return false
}

func standardPlanCompletedDate(plans []ActionPlanView) string {
	for _, p := range plans {
		if p.PlanType == "STANDARD" {
			return strOrNil(p.CompletedDate)
		}
	}
	return ""
}

func complianceRefIDs(refs []ComplianceRef) []int {
	ids := make([]int, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	return ids
}

func riskCategoryIDs(cats []RiskCategory) []int {
	ids := make([]int, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	return ids
}

// diffActionSteps compares want (CSV order) against got, sorted by StepNo
// ascending, positionally — the migration writes steps in CSV order, so a
// faithful write preserves it exactly. Sorting here rather than trusting the
// entity to already return them that way keeps the check correct regardless
// of what order the API happens to respond in.
func diffActionSteps(want []string, got []RiskActionStepView) string {
	sorted := append([]RiskActionStepView(nil), got...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StepNo < sorted[j].StepNo })

	gotDescs := make([]string, len(sorted))
	for i, s := range sorted {
		gotDescs[i] = strOrNil(s.Description)
	}
	if len(want) != len(gotDescs) {
		return fmt.Sprintf("%d steps, want %d", len(gotDescs), len(want))
	}
	for i := range want {
		if want[i] != gotDescs[i] {
			return fmt.Sprintf("step %d: want %q got %q", i+1, want[i], gotDescs[i])
		}
	}
	return ""
}

// diffIntSets compares two int slices as sets (order-independent, duplicates
// collapsed) and describes the difference, or "" when they match.
func diffIntSets(want, got []int) string {
	ws := map[int]bool{}
	for _, w := range want {
		ws[w] = true
	}
	gs := map[int]bool{}
	for _, g := range got {
		gs[g] = true
	}
	var missing, extra []int
	for w := range ws {
		if !gs[w] {
			missing = append(missing, w)
		}
	}
	for g := range gs {
		if !ws[g] {
			extra = append(extra, g)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	sort.Ints(missing)
	sort.Ints(extra)
	return fmt.Sprintf("missing=%v extra=%v", missing, extra)
}

func strOrNil(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func intOrNil(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}
