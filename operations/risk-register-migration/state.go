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
	"fmt"
	"strconv"
	"strings"
)

// RowProgress is how far a row got on a previous run. There is no ledger file
// (a Manual Task's disk is ephemeral); this is reconstructed from the entity
// every run (plan §8). Each value means "this stage is done — resume from the
// next one". ProgressGranted is defined for symmetry but coincides with
// ProgressComplete: grants are the last step for an IN_REMEDIATION row.
type RowProgress int

const (
	ProgressNone         RowProgress = iota // no matching marker risk
	ProgressCreated                         // POST /risks done; workflow_status not yet at the target bucket
	ProgressStatusWalked                    // workflow_status at the target bucket; escalation / plan not yet done
	ProgressEscalated                       // (IN_REMEDIATION) suppressing escalation present or not needed; grants outstanding
	ProgressGranted                         // all expected grants present (see note above)
	ProgressComplete                        // nothing left to do for this row
)

// ResumeState is what reconstructState learned about one row's matching risk —
// everything migrateRow needs to pick up where a previous run stopped.
type ResumeState struct {
	Progress      RowProgress
	RiskID        int    // 0 when Progress == ProgressNone
	CurrentStatus string // the matched risk's workflow_status; "" when there is no match
}

// riskState is the per-risk snapshot reconstructState pulls together.
type riskState struct {
	risk        *Risk
	escalations []Escalation
	planDone    bool
	// grantsByUser is keyed by user.id -> set of grantKey(roleID, scopeType, scopeID).
	grantsByUser map[int]map[string]struct{}
}

// statusRank is the linear order of the workflow states this migration walks
// (plan §5). A state off this path (ESCALATED, PENDING_AMENDMENT, …) is absent
// and treated as "before the target", so migrateRow re-drives the walk and the
// entity's own transition check has the final say.
var statusRank = map[string]int{
	"PENDING_RISK_OWNER_APPROVAL":       0,
	"PENDING_COMPLIANCE_REVIEW":         1,
	"IN_REMEDIATION":                    2,
	"PENDING_OWNER_COMPLETION_APPROVAL": 3,
	"PENDING_COMPLIANCE_CLOSURE":        4,
	"CLOSED":                            5,
}

func statusAtLeast(current, target string) bool {
	c, ok := statusRank[current]
	if !ok {
		return false
	}
	return c >= statusRank[target]
}

// searchPageLimit is the entity's maxLimit for /risks/search (internal/service
// pagination.go). The resume query pages at this size.
const searchPageLimit = 100

// reconstructState matches every pending row to any marker risk already in the
// entity and derives how far it got. It queries /risks/search scoped to the
// union of the pending rows' registers / years / quarters (that endpoint has no
// createdBy filter, so the marker is applied client-side), pages through every
// result, and indexes by the natural key (title + source register + year +
// quarter). A natural-key collision — two pending rows on one key, or two
// marker risks on one key — is a data-quality problem: the affected rows get a
// REJECT and are not migrated.
func reconstructState(ctx context.Context, ec *EntityClient, rd RefData, migrationDate string, rows []Row, rep *Report) (map[int]ResumeState, error) {
	out := make(map[int]ResumeState, len(rows))
	if len(rows) == 0 {
		return out, nil
	}

	existing, err := searchMarkerRisks(ctx, ec, rows)
	if err != nil {
		return nil, err
	}
	risksByKey := map[string][]Risk{}
	for _, r := range existing {
		k := naturalKey(r.RiskTitle, r.SourceRegID, r.RiskYear, r.RiskQuarter)
		risksByKey[k] = append(risksByKey[k], r)
	}

	rowIDsByKey := map[string][]int{}
	for _, row := range rows {
		k := naturalKey(row.RiskTitle, row.SourceRegisterID, row.RiskYear, row.RiskQuarter)
		rowIDsByKey[k] = append(rowIDsByKey[k], row.MigrationID)
	}

	for _, row := range rows {
		k := naturalKey(row.RiskTitle, row.SourceRegisterID, row.RiskYear, row.RiskQuarter)

		if len(rowIDsByKey[k]) > 1 {
			rep.Add(Finding{
				MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
				Severity: SevReject, Failure: "natural key",
				Detail: fmt.Sprintf("Migration IDs %v share (title, source register, year, quarter) — indistinguishable on resume",
					rowIDsByKey[k]),
			})
			continue
		}

		switch matches := risksByKey[k]; len(matches) {
		case 0:
			out[row.MigrationID] = ResumeState{Progress: ProgressNone}
		case 1:
			st, err := fetchRiskState(ctx, ec, rd, &matches[0], row)
			if err != nil {
				return nil, err
			}
			out[row.MigrationID] = ResumeState{
				Progress:      stageOf(row, rd, migrationDate, st),
				RiskID:        matches[0].ID,
				CurrentStatus: matches[0].WorkflowStatus,
			}
		default:
			rep.Add(Finding{
				MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
				Severity: SevReject, Failure: "natural key",
				Detail: fmt.Sprintf("%d marker risks already match this row's (title, source register, year, quarter)", len(matches)),
			})
		}
	}
	return out, nil
}

// searchMarkerRisks pages /risks/search over the union of the rows' registers /
// years / quarters (AND-ed IN lists on the entity side — a cartesian superset
// of what we want, narrowed precisely by natural key afterwards) and keeps only
// the results this migration created.
func searchMarkerRisks(ctx context.Context, ec *EntityClient, rows []Row) ([]Risk, error) {
	srSet := map[int]struct{}{}
	ySet := map[int]struct{}{}
	qSet := map[string]struct{}{}
	for _, r := range rows {
		srSet[r.SourceRegisterID] = struct{}{}
		ySet[r.RiskYear] = struct{}{}
		qSet[r.RiskQuarter] = struct{}{}
	}
	req := SearchRisksRequest{
		SourceRegisterIDs: intKeys(srSet),
		RiskYears:         intKeys(ySet),
		RiskQuarterKeys:   strKeys(qSet),
		Pagination:        Pagination{Limit: searchPageLimit},
	}

	var out []Risk
	for offset := 0; ; {
		req.Pagination.Offset = offset
		resp, err := ec.SearchRisks(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, r := range resp.Risks {
			if r.CreatedBy == marker {
				out = append(out, r)
			}
		}
		offset += len(resp.Risks)
		if len(resp.Risks) == 0 || offset >= resp.Total {
			break
		}
	}
	return out, nil
}

// expectedGrant is one grant D10 says an IN_REMEDIATION row must carry.
type expectedGrant struct {
	userID    int
	roleID    int
	scopeType string
	scopeID   int
}

// expectedGrants returns the D10 grants for a row (none for CLOSED). The
// management grant is conditional on ACCEPT + likelihood×impact ≥ 7.
func expectedGrants(row Row, rd RefData) []expectedGrant {
	if row.WorkflowStatus != "IN_REMEDIATION" {
		return nil
	}
	gs := []expectedGrant{
		{row.OwnerID, rd.RoleIDByName[roleRiskOwner], "RISK_TEAM", row.AssignmentTeamID},
		{row.AssignerID, rd.RoleIDByName[roleRiskAssigner], "RISK_TEAM", row.SourceRegisterID},
	}
	if row.TreatmentStrategy == "ACCEPT" && row.Likelihood*row.Impact >= 7 {
		gs = append(gs, expectedGrant{row.ManagementApproverID, rd.RoleIDByName[roleRiskManagement], "GLOBAL", 0})
	}
	return gs
}

func grantKey(roleID int, scopeType string, scopeID int) string {
	return fmt.Sprintf("%d/%s/%d", roleID, scopeType, scopeID)
}

func grantsSatisfied(row Row, rd RefData, grantsByUser map[int]map[string]struct{}) bool {
	for _, g := range expectedGrants(row, rd) {
		set, ok := grantsByUser[g.userID]
		if !ok {
			return false
		}
		if _, ok := set[grantKey(g.roleID, g.scopeType, g.scopeID)]; !ok {
			return false
		}
	}
	return true
}

// fetchRiskState pulls just what stageOf needs for one matched risk: its
// escalations always; the STANDARD action plan's status for a CLOSED row; the
// grant sets of the expected grantees for an IN_REMEDIATION row.
func fetchRiskState(ctx context.Context, ec *EntityClient, rd RefData, risk *Risk, row Row) (riskState, error) {
	st := riskState{risk: risk, grantsByUser: map[int]map[string]struct{}{}}

	escs, err := ec.ListEscalations(ctx, risk.ID)
	if err != nil {
		return st, err
	}
	st.escalations = escs

	if row.WorkflowStatus == "CLOSED" {
		plans, err := ec.ListActionPlans(ctx, risk.ID)
		if err != nil {
			return st, err
		}
		st.planDone = standardPlanCompleted(plans)
	}

	if row.WorkflowStatus == "IN_REMEDIATION" {
		seen := map[int]struct{}{}
		for _, g := range expectedGrants(row, rd) {
			if _, done := seen[g.userID]; done {
				continue
			}
			seen[g.userID] = struct{}{}
			gs, err := ec.ListGrants(ctx, g.userID)
			if err != nil {
				return st, err
			}
			set := make(map[string]struct{}, len(gs))
			for _, gr := range gs {
				set[grantKey(gr.RoleID, gr.ScopeType, gr.ScopeID)] = struct{}{}
			}
			st.grantsByUser[g.userID] = set
		}
	}
	return st, nil
}

func standardPlanCompleted(plans []ActionPlanView) bool {
	for _, p := range plans {
		if p.PlanType == "STANDARD" {
			return p.Status == "COMPLETED"
		}
	}
	return false
}

func hasOpenMarkerEscalation(escs []Escalation) bool {
	for _, e := range escs {
		if e.Status == "OPEN" && e.CreatedBy == marker {
			return true
		}
	}
	return false
}

// stageOf turns an entity snapshot into a RowProgress: the highest stage
// already finished for that row (plan §8).
func stageOf(row Row, rd RefData, migrationDate string, st riskState) RowProgress {
	if !statusAtLeast(st.risk.WorkflowStatus, row.WorkflowStatus) {
		return ProgressCreated // risk exists; the workflow_status walk is unfinished
	}

	if row.WorkflowStatus == "IN_REMEDIATION" {
		if row.ImplementationDate < migrationDate && !hasOpenMarkerEscalation(st.escalations) {
			return ProgressStatusWalked // the D8 suppressing escalation is still needed
		}
		if !grantsSatisfied(row, rd, st.grantsByUser) {
			return ProgressEscalated // D10 grants outstanding
		}
		return ProgressComplete
	}

	// CLOSED: the only step left after the status walk is completing the plan.
	if !st.planDone {
		return ProgressStatusWalked
	}
	return ProgressComplete
}

func intKeys(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func strKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// walkPath is the ordered list of PATCH hops from the born state
// (PENDING_RISK_OWNER_APPROVAL) to the target bucket (plan §5). Every hop is a
// legal transition per the entity's allowedRiskTransitions.
func walkPath(target string) []string {
	switch target {
	case "IN_REMEDIATION":
		return []string{"PENDING_COMPLIANCE_REVIEW", "IN_REMEDIATION"}
	case "CLOSED":
		return []string{
			"PENDING_COMPLIANCE_REVIEW", "IN_REMEDIATION",
			"PENDING_OWNER_COMPLETION_APPROVAL", "PENDING_COMPLIANCE_CLOSURE", "CLOSED",
		}
	default:
		return nil
	}
}

// migrateRow drives one row to its target state, doing only the steps rs.Progress
// says are still outstanding. Every step is guarded so a resumed run re-enters
// safely: status PATCHes are skipped once statusAtLeast(current, hop); the
// suppressing escalation is guarded by a GET; grant POSTs are idempotent
// server-side; the plan PATCH is a plain column set.
//
//  1. POST /risks (createdBy=marker) — only when Progress == ProgressNone
//  2. walk workflow_status along walkPath(target) via PATCH /risks/{id}
//     IN_REMEDIATION hop also sets complianceApprovalDate = migrationDate (D9)
//  3. IN_REMEDIATION + implementation_date < migrationDate + no OPEN marker
//     escalation → POST /risks/{id}/escalations (suppresses the nightly job, D8)
//  4. IN_REMEDIATION → the D10 grants (owner, assigner, conditional management)
//  5. CLOSED → PATCH the STANDARD action plan COMPLETED, completedDate = migrationDate
//
// A row-level failure (400/404/422) is recorded on rep and returns nil so the
// run continues. A 5xx / 409 / transport error returns non-nil: the run stops
// and a re-run resumes. A create that fails on the treatment enum returns a
// sentinel structural error (the AVOID backstop, §7).
func migrateRow(ctx context.Context, ec *EntityClient, cfg Config, rd RefData, row Row, rs ResumeState, rep *Report) error {
	if rs.Progress == ProgressComplete {
		rep.Skipped()
		return nil
	}

	riskID := rs.RiskID
	current := rs.CurrentStatus

	// ── 1. create ───────────────────────────────────────────────────────────
	if rs.Progress == ProgressNone {
		created, err := ec.CreateRisk(ctx, buildCreateRiskRequest(row))
		if err != nil {
			if isTreatmentEnumError(err) {
				return fmt.Errorf("treatment-strategy enum rejected by this database "+
					"(VOID→AVOID migration not applied?): %w", err)
			}
			if isRowLevel(err) {
				rejectRow(rep, row, "POST /risks", err)
				return nil
			}
			return err
		}
		riskID = created.ID
		current = created.WorkflowStatus // PENDING_RISK_OWNER_APPROVAL
	}

	// ── 2. status walk ─────────────────────────────────────────────────────
	target := row.WorkflowStatus
	for _, hop := range walkPath(target) {
		if statusAtLeast(current, hop) {
			continue
		}
		req := PatchRiskRequest{
			WorkflowStatus: ptr(hop),
			ExpectedStatus: ptr(current),
			UpdatedBy:      marker,
		}
		if hop == "IN_REMEDIATION" {
			req.ComplianceApprovalDate = ptr(cfg.MigrationDate) // D9; complianceApprovalBy stays null
		}
		if _, err := ec.PatchRisk(ctx, riskID, req); err != nil {
			if isRowLevel(err) {
				rejectRow(rep, row, "PATCH /risks → "+hop, err)
				return nil
			}
			return err
		}
		current = hop
	}

	// ── 3. overdue-suppression escalation (IN_REMEDIATION only, D8) ─────────
	if target == "IN_REMEDIATION" && row.ImplementationDate < cfg.MigrationDate {
		escs, err := ec.ListEscalations(ctx, riskID)
		if err != nil {
			return err
		}
		if !hasOpenMarkerEscalation(escs) {
			if err := ec.CreateEscalation(ctx, riskID, CreateEscalationRequest{CreatedBy: marker}); err != nil {
				if isRowLevel(err) {
					rejectRow(rep, row, "POST /risks/{id}/escalations", err)
					return nil
				}
				return err
			}
			rep.SuppressingEscalation(row.MigrationID)
		}
	}

	// ── 4. grants (IN_REMEDIATION only, D10) ───────────────────────────────
	if target == "IN_REMEDIATION" {
		for _, g := range expectedGrants(row, rd) {
			err := ec.CreateGrant(ctx, g.userID, CreateGrantRequest{
				RoleID: g.roleID, ScopeType: g.scopeType, ScopeID: g.scopeID, CreatedBy: marker,
			})
			if err != nil {
				if isRowLevel(err) {
					rejectRow(rep, row, "POST /grants/user/{id}", err)
					return nil
				}
				return err
			}
			rep.GrantWritten()
		}
	}

	// ── 5. plan completion (CLOSED only, D9) ──────────────────────────────
	if target == "CLOSED" {
		planID, err := standardPlanID(ctx, ec, riskID)
		if err != nil {
			return err
		}
		if planID == 0 {
			rejectRow(rep, row, "action plan", fmt.Errorf("risk %d has no STANDARD action plan", riskID))
			return nil
		}
		err = ec.PatchActionPlan(ctx, planID, PatchActionPlanRequest{
			Status:        ptr("COMPLETED"),
			CompletedDate: ptr(cfg.MigrationDate),
			UpdatedBy:     marker,
		})
		if err != nil {
			if isRowLevel(err) {
				rejectRow(rep, row, "PATCH /action-plans/{planId}", err)
				return nil
			}
			return err
		}
	}

	rep.Migrated(target)
	return nil
}

// buildCreateRiskRequest maps a fully-resolved Row to the POST /risks body.
// Every optional column goes through ptrOrNil so a blank cell writes NULL
// rather than "".
func buildCreateRiskRequest(row Row) CreateRiskRequest {
	req := CreateRiskRequest{
		RiskTitle:              row.RiskTitle,
		RiskDescription:        ptrOrNil(row.RiskDescription),
		SourceRegisterID:       row.SourceRegisterID,
		AssignmentTeamID:       row.AssignmentTeamID,
		AssignerID:             row.AssignerID,
		OwnerID:                row.OwnerID,
		ManagementApproverID:   row.ManagementApproverID,
		RiskYear:               row.RiskYear,
		RiskQuarter:            row.RiskQuarter,
		Likelihood:             row.Likelihood,
		Impact:                 row.Impact,
		TreatmentStrategy:      ptrOrNil(row.TreatmentStrategy),
		ImplementationDate:     ptrOrNil(row.ImplementationDate),
		ReassessmentDate:       ptrOrNil(row.ReassessmentDate),
		ImpactDescription:      ptrOrNil(row.ImpactDescription),
		RiskIdentifiedDate:     ptrOrNil(row.RiskIdentifiedDate),
		IdentifiedByType:       ptrOrNil(row.IdentifiedByType),
		IdentifiedByName:       ptrOrNil(row.IdentifiedByName),
		GitIssueURL:            ptrOrNil(row.GitIssueURL),
		EmailSubject:           ptrOrNil(row.EmailSubject),
		Remarks:                ptrOrNil(row.Remarks),
		Progress:               ptrOrNil(row.Progress),
		ActionOwnerID:          row.ActionOwnerID,
		ActionPlanDescription:  ptrOrNil(row.ActionPlanDescription),
		ComplianceReferenceIDs: row.ComplianceRefIDs,
		RiskCategoryIDs:        row.RiskCategoryIDs,
		CreatedBy:              marker,
	}
	for _, s := range row.ActionSteps {
		req.ActionSteps = append(req.ActionSteps, ActionStepInput{Description: s})
	}
	return req
}

// standardPlanID returns the id of the risk's STANDARD action plan, or 0 if it
// has none.
func standardPlanID(ctx context.Context, ec *EntityClient, riskID int) (int, error) {
	plans, err := ec.ListActionPlans(ctx, riskID)
	if err != nil {
		return 0, err
	}
	for _, p := range plans {
		if p.PlanType == "STANDARD" {
			return p.ID, nil
		}
	}
	return 0, nil
}

// isRowLevel reports whether err is a per-row problem (400 / 404 / 422) that
// should REJECT the row and let the run continue. A 409, 5xx or transport
// error is fatal — reconstructState's view diverged, or the entity is unwell.
func isRowLevel(err error) bool {
	if ae, ok := AsAPIError(err); ok {
		return ae.IsValidation() || ae.IsNotFound()
	}
	return false
}

// isTreatmentEnumError spots the AVOID-enum backstop case (§7): a create that
// fails with any 4xx/5xx whose body mentions the treatment strategy.
func isTreatmentEnumError(err error) bool {
	ae, ok := AsAPIError(err)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(ae.Body), "treatment")
}

func rejectRow(rep *Report, row Row, failure string, err error) {
	rep.Add(Finding{
		MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
		Severity: SevReject, Failure: failure, Detail: err.Error(),
	})
}

func ptr(s string) *string { return &s }

func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func naturalKey(title string, sourceRegID, year int, quarter string) string {
	return strings.TrimSpace(title) + "\x00" + strconv.Itoa(sourceRegID) + "\x00" + strconv.Itoa(year) + "\x00" + quarter
}
