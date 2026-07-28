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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskRepository defines persistence operations for the risk table.
type RiskRepository interface {
	SearchRisks(ctx context.Context, req domain.SearchRisksRequest) ([]domain.Risk, int, error)
	GetRiskByID(ctx context.Context, id int) (*domain.Risk, error)
	CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (*domain.Risk, error)
	UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (*domain.Risk, error)
	NextSequenceNumber(ctx context.Context, sourceRegisterID int) (int, error)
	GetRiskDetail(ctx context.Context, id int) (*domain.RiskDetail, error)
}

type riskRepo struct{ db *sql.DB }

// NewRiskRepository constructs a RiskRepository.
func NewRiskRepository(db *sql.DB) RiskRepository { return &riskRepo{db: db} }

const riskSelectCols = `
  r.id, r.risk_code, r.risk_year, r.risk_quarter, r.risk_title, r.risk_description,
  r.source_register_id,  src.name  AS source_register_name,
  r.assignment_team_id,  asgn.name AS assignment_team_name,
  r.assigner_id,         u_asgn.display_name AS assigner_name,
  r.owner_id,            u_own.display_name  AS owner_name,
  r.workflow_status, r.treatment_strategy,
  r.gross_score_id, rs.risk_level AS gross_risk_level,
  DATE_FORMAT(r.implementation_date, '%Y-%m-%d'),
  DATE_FORMAT(r.reassessment_date,   '%Y-%m-%d'),
  r.created_at, r.updated_at,
  DATE_FORMAT(r.risk_identified_date, '%Y-%m-%d'),
  r.identified_by_type, r.identified_by_name,
  r.impact_description, r.action_plan_id, r.progress,
  r.compliance_approval_by,
  DATE_FORMAT(r.compliance_approval_date, '%Y-%m-%d'),
  r.git_issue_url, r.email_subject, r.remarks,
  r.risk_type, r.rejection_comment, r.rejection_stage,
  -- owner_first_approved_at is a datetime, not a date: it is stamped with NOW()
  -- when a risk owner first approves. Formatting it as a plain date silently
  -- discarded the time of day.
  DATE_FORMAT(r.owner_first_approved_at, '%Y-%m-%dT%H:%i:%sZ'),
  r.created_by, r.updated_by,
  eff.risk_level AS effective_risk_level, eff.color_code AS effective_color_code`

// riskFromClause uses LEFT JOINs throughout. A risk whose register, owner or
// assigner row has gone missing is a data problem, but it must still be
// listable and readable — an inner join would silently drop it from every
// result, which is far harder to notice than a blank name. The GRC backend's
// own list query left-joins for the same reason.
const riskFromClause = `
FROM risk r
LEFT JOIN risk_team  src   ON src.id   = r.source_register_id
LEFT JOIN risk_team  asgn  ON asgn.id  = r.assignment_team_id
LEFT JOIN ` + "`user`" + ` u_asgn ON u_asgn.id = r.assigner_id
LEFT JOIN ` + "`user`" + ` u_own  ON u_own.id  = r.owner_id
LEFT JOIN risk_score rs ON rs.id   = r.gross_score_id` + effectiveScoreJoin

func (r *riskRepo) SearchRisks(ctx context.Context, req domain.SearchRisksRequest) ([]domain.Risk, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND (r.risk_code LIKE ? OR r.risk_title LIKE ?)"
		p := "%" + likeEscape(req.SearchQuery) + "%"
		args = append(args, p, p)
	}
	if len(req.WorkflowStatusKeys) > 0 {
		ph := strings.Repeat("?,", len(req.WorkflowStatusKeys))
		ph = ph[:len(ph)-1]
		where += " AND r.workflow_status IN (" + ph + ")"
		for _, s := range req.WorkflowStatusKeys {
			args = append(args, s)
		}
	}
	if len(req.SourceRegisterIDs) > 0 {
		ph := strings.Repeat("?,", len(req.SourceRegisterIDs))
		ph = ph[:len(ph)-1]
		where += " AND r.source_register_id IN (" + ph + ")"
		for _, id := range req.SourceRegisterIDs {
			args = append(args, id)
		}
	}
	if len(req.AssignmentTeamIDs) > 0 {
		ph := strings.Repeat("?,", len(req.AssignmentTeamIDs))
		ph = ph[:len(ph)-1]
		where += " AND r.assignment_team_id IN (" + ph + ")"
		for _, id := range req.AssignmentTeamIDs {
			args = append(args, id)
		}
	}
	if len(req.RiskYears) > 0 {
		ph := strings.Repeat("?,", len(req.RiskYears))
		ph = ph[:len(ph)-1]
		where += " AND r.risk_year IN (" + ph + ")"
		for _, y := range req.RiskYears {
			args = append(args, y)
		}
	}
	if len(req.RiskQuarterKeys) > 0 {
		ph := strings.Repeat("?,", len(req.RiskQuarterKeys))
		ph = ph[:len(ph)-1]
		where += " AND r.risk_quarter IN (" + ph + ")"
		for _, q := range req.RiskQuarterKeys {
			args = append(args, q)
		}
	}

	if len(req.RiskLevelKeys) > 0 {
		ph := strings.Repeat("?,", len(req.RiskLevelKeys))
		ph = ph[:len(ph)-1]
		// eff, not rs: filter on the level the row actually shows.
		where += " AND eff.risk_level IN (" + ph + ")"
		for _, l := range req.RiskLevelKeys {
			args = append(args, l)
		}
	}
	if len(req.RiskTypeKeys) > 0 {
		ph := strings.Repeat("?,", len(req.RiskTypeKeys))
		ph = ph[:len(ph)-1]
		where += " AND r.risk_type IN (" + ph + ")"
		for _, t := range req.RiskTypeKeys {
			args = append(args, t)
		}
	}
	if len(req.OwnerIDs) > 0 {
		ph := strings.Repeat("?,", len(req.OwnerIDs))
		ph = ph[:len(ph)-1]
		where += " AND r.owner_id IN (" + ph + ")"
		for _, id := range req.OwnerIDs {
			args = append(args, id)
		}
	}
	if req.ActionOwnerID != nil {
		where += " AND EXISTS (SELECT 1 FROM risk_action_plan ap WHERE ap.risk_id = r.id AND ap.action_owner_id = ?)"
		args = append(args, *req.ActionOwnerID)
	}
	// created_at is a datetime, so the bounds are widened to whole days;
	// otherwise "submitted up to the 5th" would exclude everything after
	// midnight on the 5th.
	if req.SubmittedFrom != "" {
		where += " AND r.created_at >= ?"
		args = append(args, req.SubmittedFrom+" 00:00:00")
	}
	if req.SubmittedTo != "" {
		where += " AND r.created_at <= ?"
		args = append(args, req.SubmittedTo+" 23:59:59")
	}
	if req.DueFrom != "" {
		where += " AND r.implementation_date >= ?"
		args = append(args, req.DueFrom)
	}
	if req.DueTo != "" {
		where += " AND r.implementation_date <= ?"
		args = append(args, req.DueTo)
	}
	if req.DueOverdueOnly {
		where += " AND r.implementation_date IS NOT NULL AND r.implementation_date < CURDATE()"
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+riskFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("risk.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+riskSelectCols+riskFromClause+" "+where+" ORDER BY r.created_at DESC LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("risk.Search query: %w", err)
	}
	defer rows.Close()

	var risks []domain.Risk
	for rows.Next() {
		risk, err := scanRisk(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("risk.Search scan: %w", err)
		}
		risks = append(risks, *risk)
	}
	return risks, total, rows.Err()
}

func (r *riskRepo) GetRiskByID(ctx context.Context, id int) (*domain.Risk, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT"+riskSelectCols+riskFromClause+" WHERE r.id = ?", id)
	risk, err := scanRisk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk.GetByID(%d): %w", id, err)
	}
	return risk, nil
}

// CreateRisk generates a unique risk code using the risk_register_sequence table
// (atomically incremented per team) and inserts the new risk row.
func (r *riskRepo) CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (*domain.Risk, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk.Create begin: %w", err)
	}
	defer tx.Rollback()

	// ── 1. Reserve the next sequence number ───────────────────────────────────
	// Keyed on the SOURCE REGISTER, not the assignment team: a risk's code
	// belongs to the register it was raised in, and the two are frequently
	// different. INSERT IGNORE then SELECT ... FOR UPDATE holds the row for the
	// rest of the transaction, so concurrent creates cannot take the same
	// number. The counter never resets — it runs across years and quarters.
	if _, err = tx.ExecContext(ctx,
		"INSERT IGNORE INTO risk_register_sequence (risk_team_id, last_sequence_number) VALUES (?, 0)",
		req.SourceRegisterID); err != nil {
		return nil, fmt.Errorf("risk.Create ensure sequence: %w", err)
	}

	var lastSeq int
	if err = tx.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_register_sequence WHERE risk_team_id = ? FOR UPDATE",
		req.SourceRegisterID).Scan(&lastSeq); err != nil {
		return nil, fmt.Errorf("risk.Create lock sequence: %w", err)
	}
	seqNum := lastSeq + 1

	if _, err = tx.ExecContext(ctx,
		"UPDATE risk_register_sequence SET last_sequence_number = ? WHERE risk_team_id = ?",
		seqNum, req.SourceRegisterID); err != nil {
		return nil, fmt.Errorf("risk.Create bump sequence: %w", err)
	}

	// ── 2. Build the risk code from the source register's code ────────────────
	// Deliberately not COALESCE(code, name): a register with no code is a data
	// problem, and silently substituting its name would mint a risk code in a
	// different format that nothing can parse back.
	var teamCode string
	if err = tx.QueryRowContext(ctx,
		"SELECT code FROM risk_team WHERE id = ?",
		req.SourceRegisterID).Scan(&teamCode); err != nil {
		return nil, fmt.Errorf("risk.Create team code: %w", err)
	}
	riskCode := fmt.Sprintf("%d-%s-%s-%04d", req.RiskYear, teamCode, req.RiskQuarter, seqNum)

	// ── 3. Resolve the gross score from likelihood/impact ─────────────────────
	var grossScoreID int
	if err = tx.QueryRowContext(ctx,
		"SELECT id FROM risk_score WHERE likelihood = ? AND impact = ?",
		req.Likelihood, req.Impact).Scan(&grossScoreID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &apierror.ValidationError{
				Msg: fmt.Sprintf("no risk score for likelihood %d and impact %d", req.Likelihood, req.Impact),
			}
		}
		return nil, fmt.Errorf("risk.Create resolve gross score: %w", err)
	}

	// ── 4. Insert the risk ────────────────────────────────────────────────────
	res, err := tx.ExecContext(ctx,
		`INSERT INTO risk (
			risk_code, risk_year, risk_quarter, risk_title, risk_description,
			source_register_id, assignment_team_id, assigner_id, owner_id,
			treatment_strategy, gross_score_id, progress,
			implementation_date, reassessment_date,
			impact_description, risk_identified_date,
			identified_by_type, identified_by_name,
			git_issue_url, email_subject, remarks,
			workflow_status, created_by, updated_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING_RISK_OWNER_APPROVAL', ?, ?)`,
		riskCode, req.RiskYear, req.RiskQuarter, req.RiskTitle, req.RiskDescription,
		req.SourceRegisterID, req.AssignmentTeamID, req.AssignerID, req.OwnerID,
		req.TreatmentStrategy, grossScoreID, req.Progress,
		req.ImplementationDate, req.ReassessmentDate,
		req.ImpactDescription, req.RiskIdentifiedDate,
		req.IdentifiedByType, req.IdentifiedByName,
		req.GitIssueURL, req.EmailSubject, req.Remarks,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk.Create insert: %w", err)
	}
	riskID64, err := res.LastInsertId()
	if err != nil || riskID64 == 0 {
		if err == nil {
			err = fmt.Errorf("driver returned zero last-insert-id")
		}
		return nil, fmt.Errorf("risk.Create inserted id: %w", err)
	}
	riskID := int(riskID64)

	// ── 5. Action plan, its steps, and the compliance links ───────────────────
	// All inside this transaction. A risk visible in the register without its
	// action plan is not a state the product can represent.
	planRes, err := tx.ExecContext(ctx,
		`INSERT INTO risk_action_plan (risk_id, action_owner_id, description, status, plan_type, created_by, updated_by)
		 VALUES (?, ?, ?, 'PENDING', 'STANDARD', ?, ?)`,
		riskID, nullableInt(req.ActionOwnerID), req.ActionPlanDescription, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk.Create action plan: %w", err)
	}
	planID64, err := planRes.LastInsertId()
	if err != nil || planID64 == 0 {
		if err == nil {
			err = fmt.Errorf("driver returned zero last-insert-id")
		}
		return nil, fmt.Errorf("risk.Create action plan id: %w", err)
	}
	planID := int(planID64)

	if _, err = tx.ExecContext(ctx,
		"UPDATE risk SET action_plan_id = ? WHERE id = ?", planID, riskID); err != nil {
		return nil, fmt.Errorf("risk.Create link action plan: %w", err)
	}

	// Step numbers come from slice order, starting at 1.
	for i, step := range req.ActionSteps {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO risk_action_step (plan_id, step_no, description, status, created_by, updated_by)
			 VALUES (?, ?, ?, 'PENDING', ?, ?)`,
			planID, i+1, step.Description, req.CreatedBy, req.CreatedBy); err != nil {
			return nil, fmt.Errorf("risk.Create action step %d: %w", i+1, err)
		}
	}

	for _, refID := range req.ComplianceReferenceIDs {
		if _, err = tx.ExecContext(ctx,
			"INSERT INTO risk_compliance_reference (risk_id, reference_id) VALUES (?, ?)",
			riskID, refID); err != nil {
			return nil, fmt.Errorf("risk.Create compliance reference %d: %w", refID, err)
		}
	}

	// ── 6. Record the creation in the change log ──────────────────────────────
	if _, err = tx.ExecContext(ctx,
		"INSERT INTO risk_change_log (risk_id, created_by, action) VALUES (?, ?, 'CREATE')",
		riskID, req.CreatedBy); err != nil {
		return nil, fmt.Errorf("risk.Create change log: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk.Create commit: %w", err)
	}
	return r.GetRiskByID(ctx, riskID)
}

func (r *riskRepo) UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (*domain.Risk, error) {
	sets := []string{}
	args := []any{}

	if req.RiskTitle != nil {
		sets = append(sets, "risk_title = ?")
		args = append(args, *req.RiskTitle)
	}
	if req.RiskDescription != nil {
		sets = append(sets, "risk_description = ?")
		args = append(args, *req.RiskDescription)
	}
	if req.WorkflowStatus != nil {
		sets = append(sets, "workflow_status = ?")
		args = append(args, *req.WorkflowStatus)
	}
	if req.TreatmentStrategy != nil {
		sets = append(sets, "treatment_strategy = ?")
		args = append(args, *req.TreatmentStrategy)
	}
	if req.GrossScoreID != nil {
		sets = append(sets, "gross_score_id = ?")
		args = append(args, *req.GrossScoreID)
	}
	if req.ImplementationDate != nil {
		sets = append(sets, "implementation_date = ?")
		args = append(args, *req.ImplementationDate)
	}
	if req.ReassessmentDate != nil {
		sets = append(sets, "reassessment_date = ?")
		args = append(args, *req.ReassessmentDate)
	}
	if req.Progress != nil {
		sets = append(sets, "progress = ?")
		args = append(args, *req.Progress)
	}
	if req.EmailSubject != nil {
		sets = append(sets, "email_subject = ?")
		args = append(args, *req.EmailSubject)
	}
	if req.RiskType != nil {
		sets = append(sets, "risk_type = ?")
		args = append(args, *req.RiskType)
	}
	if req.OwnerFirstApprovedAt != nil {
		sets = append(sets, "owner_first_approved_at = ?")
		args = append(args, *req.OwnerFirstApprovedAt)
	}
	if req.ImpactDescription != nil {
		sets = append(sets, "impact_description = ?")
		args = append(args, *req.ImpactDescription)
	}
	if req.RiskIdentifiedDate != nil {
		sets = append(sets, "risk_identified_date = ?")
		args = append(args, *req.RiskIdentifiedDate)
	}
	if req.IdentifiedByType != nil {
		sets = append(sets, "identified_by_type = ?")
		args = append(args, *req.IdentifiedByType)
	}
	if req.IdentifiedByName != nil {
		sets = append(sets, "identified_by_name = ?")
		args = append(args, *req.IdentifiedByName)
	}
	if req.AssignerID != nil {
		sets = append(sets, "assigner_id = ?")
		args = append(args, *req.AssignerID)
	}
	if req.ClearRejection {
		sets = append(sets, "rejection_comment = NULL", "rejection_stage = NULL")
	}
	if req.RejectionComment != nil {
		sets = append(sets, "rejection_comment = ?")
		args = append(args, *req.RejectionComment)
	}
	if req.RejectionStage != nil {
		sets = append(sets, "rejection_stage = ?")
		args = append(args, *req.RejectionStage)
	}
	if req.ComplianceApprovalBy != nil {
		sets = append(sets, "compliance_approval_by = ?")
		args = append(args, *req.ComplianceApprovalBy)
	}
	if req.ComplianceApprovalDate != nil {
		sets = append(sets, "compliance_approval_date = ?")
		args = append(args, *req.ComplianceApprovalDate)
	}
	if req.AssignmentTeamID != nil {
		sets = append(sets, "assignment_team_id = ?")
		args = append(args, *req.AssignmentTeamID)
	}
	if req.OwnerID != nil {
		sets = append(sets, "owner_id = ?")
		args = append(args, *req.OwnerID)
	}
	if req.ActionPlanID != nil {
		sets = append(sets, "action_plan_id = ?")
		args = append(args, *req.ActionPlanID)
	}
	if req.GitIssueURL != nil {
		sets = append(sets, "git_issue_url = ?")
		args = append(args, *req.GitIssueURL)
	}
	if req.Remarks != nil {
		sets = append(sets, "remarks = ?")
		args = append(args, *req.Remarks)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)

	sets = append(sets, "updated_at = NOW()")

	// Everything below is one transaction: the risk row, its compliance-reference
	// links, its action plan and steps, and the change-log entries either all
	// land or none do. Over HTTP the caller cannot retry a half-applied edit,
	// so a partial update would leave a risk the user cannot repair.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk.Update begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var query string
	if req.ExpectedStatus != "" {
		args = append(args, id, req.ExpectedStatus)
		query = "UPDATE risk SET " + strings.Join(sets, ", ") + " WHERE id = ? AND workflow_status = ?" // #nosec G202
	} else {
		args = append(args, id)
		query = "UPDATE risk SET " + strings.Join(sets, ", ") + " WHERE id = ?" // #nosec G202
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("risk.Update(%d): %w", id, err)
	}
	if req.ExpectedStatus != "" {
		if n, _ := result.RowsAffected(); n == 0 {
			// Zero rows means either the status moved under us, or MySQL
			// reported a no-op because nothing actually differed. Distinguish
			// them by reading the row inside this transaction.
			var current string
			if err := tx.QueryRowContext(ctx,
				"SELECT workflow_status FROM risk WHERE id = ?", id).Scan(&current); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", id)}
				}
				return nil, fmt.Errorf("risk.Update(%d) recheck: %w", id, err)
			}
			if current != req.ExpectedStatus || (req.WorkflowStatus != nil && *req.WorkflowStatus != req.ExpectedStatus) {
				return nil, &apierror.ConflictError{Msg: "risk was modified concurrently, please retry"}
			}
		}
	}

	// Compliance references: nil means "not touching them"; a non-nil slice is
	// the complete desired set, so replace wholesale.
	if req.ComplianceReferenceIDs != nil {
		if _, err = tx.ExecContext(ctx,
			"DELETE FROM risk_compliance_reference WHERE risk_id = ?", id); err != nil {
			return nil, fmt.Errorf("risk.Update clear compliance refs: %w", err)
		}
		for _, refID := range req.ComplianceReferenceIDs {
			if _, err = tx.ExecContext(ctx,
				"INSERT INTO risk_compliance_reference (risk_id, reference_id) VALUES (?, ?)",
				id, refID); err != nil {
				return nil, fmt.Errorf("risk.Update compliance reference %d: %w", refID, err)
			}
		}
	}

	if req.ActionPlan != nil {
		if _, err = tx.ExecContext(ctx, `
			UPDATE risk_action_plan SET
				description = COALESCE(?, description),
				action_owner_id = COALESCE(?, action_owner_id),
				updated_by = ?, updated_at = NOW()
			WHERE risk_id = ? AND plan_type = 'STANDARD'`,
			req.ActionPlan.Description, nullableInt(req.ActionPlan.ActionOwnerID),
			req.UpdatedBy, id); err != nil {
			return nil, fmt.Errorf("risk.Update action plan: %w", err)
		}
	}

	if req.ActionSteps != nil {
		if err = r.applyActionSteps(ctx, tx, id, req); err != nil {
			return nil, err
		}
	}

	for _, e := range req.ChangeLog {
		action := e.Action
		if action == "" {
			action = "UPDATE"
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO risk_change_log (risk_id, created_by, action, field_changed, old_value, new_value)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, req.UpdatedBy, action, e.FieldChanged, e.OldValue, e.NewValue); err != nil {
			return nil, fmt.Errorf("risk.Update change log: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk.Update commit: %w", err)
	}
	return r.GetRiskByID(ctx, id)
}

// applyActionSteps reconciles the plan's steps with the desired list. Steps are
// matched by ID rather than replaced wholesale so that an untouched step keeps
// its status and completed_date — a user editing the wording of step 3 must not
// silently reopen step 1. An ID that is not on this plan (stale, or belonging
// to another plan) is treated as a new step rather than trusted.
func (r *riskRepo) applyActionSteps(ctx context.Context, tx *sql.Tx, riskID int, req domain.UpdateRiskRequest) error {
	var planID int
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM risk_action_plan WHERE risk_id = ? AND plan_type = 'STANDARD' LIMIT 1",
		riskID).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d has no standard action plan", riskID)}
	}
	if err != nil {
		return fmt.Errorf("risk.Update find action plan: %w", err)
	}

	rows, err := tx.QueryContext(ctx, "SELECT id FROM risk_action_step WHERE plan_id = ?", planID)
	if err != nil {
		return fmt.Errorf("risk.Update load step ids: %w", err)
	}
	existing := make(map[int]bool)
	for rows.Next() {
		var stepID int
		if err := rows.Scan(&stepID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("risk.Update scan step id: %w", err)
		}
		existing[stepID] = true
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("risk.Update iterate step ids: %w", err)
	}

	kept := make(map[int]bool)
	for i, step := range req.ActionSteps {
		if step.ID != nil && existing[*step.ID] {
			if _, err := tx.ExecContext(ctx, `
				UPDATE risk_action_step SET step_no = ?, description = ?, updated_by = ?, updated_at = NOW()
				WHERE id = ? AND plan_id = ?`,
				i+1, step.Description, req.UpdatedBy, *step.ID, planID); err != nil {
				return fmt.Errorf("risk.Update step %d: %w", i+1, err)
			}
			kept[*step.ID] = true
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO risk_action_step (plan_id, step_no, description, status, created_by, updated_by)
			VALUES (?, ?, ?, 'PENDING', ?, ?)`,
			planID, i+1, step.Description, req.UpdatedBy, req.UpdatedBy); err != nil {
			return fmt.Errorf("risk.Update new step %d: %w", i+1, err)
		}
	}

	for stepID := range existing {
		if !kept[stepID] {
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM risk_action_step WHERE id = ? AND plan_id = ?",
				stepID, planID); err != nil {
				return fmt.Errorf("risk.Update delete step %d: %w", stepID, err)
			}
		}
	}
	return nil
}

func scanRisk(s scanner) (*domain.Risk, error) {
	return scanRiskWithExtras(s)
}

// scanRiskWithExtras scans the riskSelectCols projection into a domain.Risk,
// then any extra destinations the caller appended to the SELECT. Detail reads
// add the compliance approver's name and the two score joins this way, so the
// base column list stays defined in exactly one place.
func scanRiskWithExtras(s scanner, extras ...any) (*domain.Risk, error) {
	var r domain.Risk
	var desc, treatment, grossLevel, implDate, reassDate sql.NullString
	var identifiedDate, identifiedByType, identifiedByName sql.NullString
	var impactDesc, progress, complianceApprovalDate sql.NullString
	var gitIssueURL, emailSubject, remarks sql.NullString
	var rejectionComment, rejectionStage, ownerFirstApprovedAt sql.NullString
	var effLevel, effColour sql.NullString
	var grossScoreID, actionPlanID, complianceApprovalBy sql.NullInt64

	dest := []any{
		&r.ID, &r.RiskCode, &r.RiskYear, &r.RiskQuarter, &r.RiskTitle, &desc,
		&r.SourceRegisterID, &r.SourceRegisterName,
		&r.AssignmentTeamID, &r.AssignmentTeamName,
		&r.AssignerID, &r.AssignerName,
		&r.OwnerID, &r.OwnerName,
		&r.WorkflowStatus, &treatment,
		&grossScoreID, &grossLevel,
		&implDate, &reassDate,
		&r.CreatedOn, &r.UpdatedOn,
		&identifiedDate, &identifiedByType, &identifiedByName,
		&impactDesc, &actionPlanID, &progress,
		&complianceApprovalBy, &complianceApprovalDate,
		&gitIssueURL, &emailSubject, &remarks,
		&r.RiskType, &rejectionComment, &rejectionStage,
		&ownerFirstApprovedAt,
		&r.CreatedBy, &r.UpdatedBy,
		&effLevel, &effColour,
	}
	if err := s.Scan(append(dest, extras...)...); err != nil {
		return nil, err
	}

	nullStr := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}
	nullInt := func(ni sql.NullInt64) *int {
		if ni.Valid {
			v := int(ni.Int64)
			return &v
		}
		return nil
	}
	r.RiskDescription = nullStr(desc)
	r.TreatmentStrategy = nullStr(treatment)
	r.GrossRiskLevel = nullStr(grossLevel)
	r.ImplementationDate = nullStr(implDate)
	r.ReassessmentDate = nullStr(reassDate)
	r.RiskIdentifiedDate = nullStr(identifiedDate)
	r.IdentifiedByType = nullStr(identifiedByType)
	r.IdentifiedByName = nullStr(identifiedByName)
	r.ImpactDescription = nullStr(impactDesc)
	r.Progress = nullStr(progress)
	r.ComplianceApprovalDate = nullStr(complianceApprovalDate)
	r.GitIssueURL = nullStr(gitIssueURL)
	r.EmailSubject = nullStr(emailSubject)
	r.Remarks = nullStr(remarks)
	r.RejectionComment = nullStr(rejectionComment)
	r.RejectionStage = nullStr(rejectionStage)
	r.OwnerFirstApprovedAt = nullStr(ownerFirstApprovedAt)
	r.GrossScoreID = nullInt(grossScoreID)
	r.ActionPlanID = nullInt(actionPlanID)
	r.ComplianceApprovalBy = nullInt(complianceApprovalBy)
	r.EffectiveRiskLevel = nullStr(effLevel)
	r.EffectiveColorCode = nullStr(effColour)
	return &r, nil
}

// NextSequenceNumber previews the sequence number the next risk created for
// sourceRegisterID would receive, without consuming it. CreateRisk increments
// the counter inside its transaction; this is a read used to show the risk code
// on a form before the risk exists.
//
// A register with no row in risk_register_sequence has had no risks created
// yet, so its next number is 1 — but only if the register itself exists,
// otherwise a typo in the id would silently preview a valid-looking code.
func (r *riskRepo) NextSequenceNumber(ctx context.Context, sourceRegisterID int) (int, error) {
	var lastSeq int
	err := r.db.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_register_sequence WHERE risk_team_id = ?",
		sourceRegisterID).Scan(&lastSeq)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if err := r.db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM risk_team WHERE id = ?)",
			sourceRegisterID).Scan(&exists); err != nil {
			return 0, fmt.Errorf("risk.NextSequenceNumber validate register: %w", err)
		}
		if !exists {
			return 0, &apierror.NotFoundError{Msg: fmt.Sprintf("source register %d not found", sourceRegisterID)}
		}
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("risk.NextSequenceNumber: %w", err)
	}
	return lastSeq + 1, nil
}

// effectiveScoreJoin resolves a risk's current residual standing: the score of
// its most recent assessment when it has one, otherwise its gross score. It is
// a LEFT JOIN because a risk with neither must still be readable — dropping it
// from a detail page would be worse than showing it without a score.
const effectiveScoreJoin = `
LEFT JOIN risk_score eff ON eff.id = COALESCE(
	(SELECT ra.score_id FROM risk_assessment ra
	  WHERE ra.risk_id = r.id
	  ORDER BY ra.created_at DESC, ra.id DESC LIMIT 1),
	r.gross_score_id)`

// GetRiskDetail assembles the whole risk page in one call: the risk, its
// resolved names, both scores, and its references, action plan, steps and
// assessments.
func (r *riskRepo) GetRiskDetail(ctx context.Context, id int) (*domain.RiskDetail, error) {
	var d domain.RiskDetail

	var approverName sql.NullString
	var grossID, grossLikelihood, grossImpact, grossRating sql.NullInt64
	var grossLevel, grossColor sql.NullString
	var effID, effLikelihood, effImpact, effRating sql.NullInt64
	var effLevel, effColor sql.NullString

	row := r.db.QueryRowContext(ctx, `
		SELECT `+riskSelectCols+`,
		       ca.display_name,
		       rs.id, rs.likelihood, rs.impact, rs.risk_rating, rs.risk_level, rs.color_code,
		       eff.id, eff.likelihood, eff.impact, eff.risk_rating, eff.risk_level, eff.color_code
		`+riskFromClause+`
		LEFT JOIN `+"`user`"+` ca ON ca.id = r.compliance_approval_by
		WHERE r.id = ?`, id)

	risk, err := scanRiskWithExtras(row,
		&approverName,
		&grossID, &grossLikelihood, &grossImpact, &grossRating, &grossLevel, &grossColor,
		&effID, &effLikelihood, &effImpact, &effRating, &effLevel, &effColor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk.GetDetail(%d): %w", id, err)
	}
	d.Risk = *risk
	if approverName.Valid {
		d.ComplianceApproverName = &approverName.String
	}
	d.GrossScore = buildScore(grossID, grossLikelihood, grossImpact, grossRating, grossLevel, grossColor)
	d.EffectiveScore = buildScore(effID, effLikelihood, effImpact, effRating, effLevel, effColor)

	if d.ComplianceReferences, err = r.detailReferences(ctx, id); err != nil {
		return nil, err
	}
	if d.ActionPlan, err = r.detailActionPlan(ctx, id); err != nil {
		return nil, err
	}
	if d.Assessments, err = r.detailAssessments(ctx, id); err != nil {
		return nil, err
	}
	return &d, nil
}

// buildScore returns nil when the joined score row was absent, so callers can
// tell "no score" from "a score of zero".
func buildScore(id, likelihood, impact, rating sql.NullInt64, level, colour sql.NullString) *domain.RiskScore {
	if !id.Valid {
		return nil
	}
	return &domain.RiskScore{
		ID:         int(id.Int64),
		Likelihood: int(likelihood.Int64),
		Impact:     int(impact.Int64),
		RiskRating: int(rating.Int64),
		RiskLevel:  level.String,
		ColorCode:  colour.String,
	}
}

func (r *riskRepo) detailReferences(ctx context.Context, id int) ([]domain.RiskComplianceReference, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT scr.id, scr.name, scr.description, scr.created_at, scr.updated_at
		FROM risk_compliance_reference rcr
		JOIN risk_security_compliance_reference scr ON scr.id = rcr.reference_id
		-- Ordered by reference id, which is the order MySQL returns for the
		-- equivalent unordered query in the GRC backend. Ordering by name
		-- instead would reorder the list a caller renders.
		WHERE rcr.risk_id = ? ORDER BY rcr.reference_id`, id)
	if err != nil {
		return nil, fmt.Errorf("risk.GetDetail references: %w", err)
	}
	defer rows.Close()

	refs := []domain.RiskComplianceReference{}
	for rows.Next() {
		var ref domain.RiskComplianceReference
		if err := rows.Scan(&ref.ID, &ref.Name, &ref.Description, &ref.CreatedOn, &ref.UpdatedOn); err != nil {
			return nil, fmt.Errorf("risk.GetDetail scan reference: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (r *riskRepo) detailActionPlan(ctx context.Context, id int) (*domain.RiskActionPlanDetail, error) {
	var ap domain.RiskActionPlanDetail
	err := r.db.QueryRowContext(ctx,
		`SELECT id, risk_id, action_owner_id, description, status, plan_type
		 FROM risk_action_plan WHERE risk_id = ? AND plan_type = 'STANDARD' LIMIT 1`, id).
		Scan(&ap.ID, &ap.RiskID, &ap.ActionOwnerID, &ap.Description, &ap.Status, &ap.PlanType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // a risk without a standard plan is legitimate, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("risk.GetDetail action plan: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, plan_id, step_no, description, status,
		        DATE_FORMAT(completed_date,'%Y-%m-%d'), created_at, updated_at
		 FROM risk_action_step WHERE plan_id = ? ORDER BY step_no`, ap.ID)
	if err != nil {
		return nil, fmt.Errorf("risk.GetDetail action steps: %w", err)
	}
	defer rows.Close()

	ap.Steps = []domain.RiskActionStep{}
	for rows.Next() {
		var st domain.RiskActionStep
		if err := rows.Scan(&st.ID, &st.PlanID, &st.StepNo, &st.Description, &st.Status,
			&st.CompletedDate, &st.CreatedOn, &st.UpdatedOn); err != nil {
			return nil, fmt.Errorf("risk.GetDetail scan step: %w", err)
		}
		ap.Steps = append(ap.Steps, st)
	}
	return &ap, rows.Err()
}

func (r *riskRepo) detailAssessments(ctx context.Context, id int) ([]domain.RiskAssessment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+assessmentColumns+`
		 FROM risk_assessment ra JOIN risk_score rs ON rs.id = ra.score_id
		 WHERE ra.risk_id = ? ORDER BY ra.created_at DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("risk.GetDetail assessments: %w", err)
	}
	defer rows.Close()

	out := []domain.RiskAssessment{}
	for rows.Next() {
		a, err := scanAssessment(rows)
		if err != nil {
			return nil, fmt.Errorf("risk.GetDetail scan assessment: %w", err)
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
