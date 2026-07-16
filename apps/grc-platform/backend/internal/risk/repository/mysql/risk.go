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

package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

type riskRepository struct{ db *sql.DB }

// NewRiskRepository creates a MySQL-backed repository.RiskRepository.
func NewRiskRepository(db *sql.DB) repository.RiskRepository {
	return &riskRepository{db: db}
}

// NextSequenceID returns the next sequence number for a given source register
// without reserving it. This is a preview — the actual number is assigned
// atomically inside Create. Reads last_sequence_number from the sequence table;
// returns 1 if no row exists yet (first risk for this team).
// The counter never resets per year/quarter (it's unique per team across all
// time), so the lookup only needs sourceRegisterID.
func (r *riskRepository) NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error) {
	var lastSeq int
	err := r.db.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_register_sequence WHERE risk_team_id = ?",
		sourceRegisterID,
	).Scan(&lastSeq)
	if err == sql.ErrNoRows {
		var exists bool
		if existsErr := r.db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM risk_team WHERE id = ?)",
			sourceRegisterID,
		).Scan(&exists); existsErr != nil {
			return 0, fmt.Errorf("validate source register for preview: %w", existsErr)
		}
		if !exists {
			return 0, &apierror.Error{StatusCode: http.StatusNotFound, Body: fmt.Sprintf("source register %d not found", sourceRegisterID)}
		}
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read sequence for preview: %w", err)
	}
	return lastSeq + 1, nil
}

// Create inserts a new risk and all related records inside a single transaction:
//  1. Locks risk_register_sequence row and increments (counter never resets — globally unique per team)
//  2. Resolves the team code → generates YEAR-CODE-QUARTER-NNNN risk code
//  3. Resolves gross_score_id from (likelihood, impact)
//  4. Inserts risk with workflow_status = PENDING_RISK_OWNER_APPROVAL
//  5. Inserts risk_action_plan and links back to risk.action_plan_id
//  6. Inserts risk_action_step rows
//  7. Inserts risk_compliance_reference rows
//  8. Inserts a CREATE row into risk_change_log
func (r *riskRepository) Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// ── 1. Lock sequence row and determine next number ────────────────────────
	// INSERT IGNORE ensures the row exists for new teams; FOR UPDATE serialises
	// concurrent creates. The counter never resets — it increments across years
	// and quarters so every risk code for this team is globally unique.
	if _, err = tx.ExecContext(ctx,
		"INSERT IGNORE INTO risk_register_sequence (risk_team_id, last_sequence_number) VALUES (?, 0)",
		req.SourceRegisterID,
	); err != nil {
		return nil, fmt.Errorf("ensure sequence row: %w", err)
	}

	var lastSeq int
	err = tx.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_register_sequence WHERE risk_team_id = ? FOR UPDATE",
		req.SourceRegisterID,
	).Scan(&lastSeq)
	if err != nil {
		return nil, fmt.Errorf("lock sequence row: %w", err)
	}

	nextSeq := lastSeq + 1

	if _, err = tx.ExecContext(ctx,
		"UPDATE risk_register_sequence SET last_sequence_number = ? WHERE risk_team_id = ?",
		nextSeq, req.SourceRegisterID,
	); err != nil {
		return nil, fmt.Errorf("update sequence: %w", err)
	}

	// ── 2. Resolve team code for risk code generation ─────────────────────────
	var teamCode string
	err = tx.QueryRowContext(ctx,
		"SELECT code FROM risk_team WHERE id = ?",
		req.SourceRegisterID,
	).Scan(&teamCode)
	if err != nil {
		return nil, fmt.Errorf("resolve team code: %w", err)
	}

	riskCode := fmt.Sprintf("%d-%s-%s-%04d", req.Year, teamCode, req.Quarter, nextSeq)

	// ── 3. Resolve gross score ID ─────────────────────────────────────────────
	var grossScoreID int
	err = tx.QueryRowContext(ctx,
		"SELECT id FROM risk_score WHERE likelihood = ? AND impact = ?",
		req.Likelihood, req.Impact,
	).Scan(&grossScoreID)
	if err != nil {
		return nil, fmt.Errorf("resolve gross score: %w", err)
	}

	// ── 4. Insert risk row ───────────────────────────────────────────────────
	riskResult, err := tx.ExecContext(ctx, `
		INSERT INTO risk (
			risk_year, source_register_id, risk_quarter, risk_code,
			risk_title, risk_description, risk_identified_date,
			identified_by_type, identified_by_user_id, identified_by_name,
			assigner_id, owner_id, impact_description, gross_score_id,
			treatment_strategy, assignment_team_id, progress,
			implementation_date, reassessment_date,
			git_issue_url, email_subject, remarks,
			workflow_status, created_by, updated_by
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?,
			?, ?, ?,
			?, ?, ?
		)`,
		req.Year, req.SourceRegisterID, req.Quarter, riskCode,
		req.RiskTitle, req.RiskDescription, nullableString(req.RiskIdentifiedDate),
		req.IdentifiedByType, req.IdentifiedByUserID, req.IdentifiedByName,
		req.AssignerID, req.OwnerID, nullableString(req.ImpactDescription), grossScoreID,
		req.TreatmentStrategy, req.AssignmentTeamID, nullableString(req.Progress),
		nullableString(req.ImplementationDate), nullableString(req.ReassessmentDate),
		nullableString(req.GitIssueURL), nullableString(req.EmailSubject), nullableString(req.Remarks),
		model.StatusPendingOwnerApproval, createdBy, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("insert risk: %w", err)
	}
	riskIDInt64, err := riskResult.LastInsertId()
	if err != nil || riskIDInt64 == 0 {
		if err == nil {
			err = fmt.Errorf("driver returned zero last-insert-id")
		}
		return nil, fmt.Errorf("get inserted risk id: %w", err)
	}
	riskID := int(riskIDInt64)

	// ── 5. Insert action plan ─────────────────────────────────────────────────
	planResult, err := tx.ExecContext(ctx, `
		INSERT INTO risk_action_plan (risk_id, action_owner_id, description, status, plan_type, created_by, updated_by)
		VALUES (?, ?, ?, 'PENDING', 'STANDARD', ?, ?)`,
		riskID, req.ActionOwnerID, nullableString(req.ActionPlanDescription), createdBy, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("insert action plan: %w", err)
	}
	planIDInt64, err := planResult.LastInsertId()
	if err != nil || planIDInt64 == 0 {
		if err == nil {
			err = fmt.Errorf("driver returned zero last-insert-id")
		}
		return nil, fmt.Errorf("get inserted plan id: %w", err)
	}
	planID := int(planIDInt64)

	// Link action_plan_id back onto the risk row.
	if _, err = tx.ExecContext(ctx,
		"UPDATE risk SET action_plan_id = ? WHERE id = ?", planID, riskID,
	); err != nil {
		return nil, fmt.Errorf("link action plan to risk: %w", err)
	}

	// ── 7. Insert action steps ────────────────────────────────────────────────
	for i, step := range req.ActionSteps {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO risk_action_step (plan_id, step_no, description, status, created_by, updated_by)
			VALUES (?, ?, ?, 'PENDING', ?, ?)`,
			planID, i+1, step.Description, createdBy, createdBy,
		); err != nil {
			return nil, fmt.Errorf("insert action step %d: %w", i+1, err)
		}
	}

	// ── 8. Insert compliance reference links ──────────────────────────────────
	for _, refID := range req.ComplianceReferenceIDs {
		if _, err = tx.ExecContext(ctx,
			"INSERT INTO risk_compliance_reference (risk_id, reference_id) VALUES (?, ?)",
			riskID, refID,
		); err != nil {
			return nil, fmt.Errorf("insert compliance reference %d: %w", refID, err)
		}
	}

	// ── 9. Insert change log entry ────────────────────────────────────────────
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO risk_change_log (risk_id, created_by, action)
		VALUES (?, ?, 'CREATE')`,
		riskID, createdBy,
	); err != nil {
		return nil, fmt.Errorf("insert change log: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &model.CreateRiskResponse{ID: riskID, RiskCode: riskCode}, nil
}

// placeholders returns n comma-separated `?` marks for a SQL IN (...) clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// effectiveScoreLeftJoin resolves each risk's effective residual score (latest
// reassessment if one exists, else gross score) as `rs`, same convention as
// effectiveScoreJoin (dashboard.go). Unlike that one, this is a LEFT JOIN: a
// risk with neither a gross score nor any assessment must still appear in the
// registers table / detail drawer, whereas dropping it from an aggregate
// chart is acceptable.
const effectiveScoreLeftJoin = `
	LEFT JOIN risk_score rs ON rs.id = COALESCE(
		(SELECT ra.score_id
		   FROM risk_assessment ra
		  WHERE ra.risk_id = r.id
		  ORDER BY ra.created_at DESC, ra.id DESC
		  LIMIT 1),
		r.gross_score_id)`

func (r *riskRepository) List(ctx context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error) {
	query := `
		SELECT r.id, r.risk_code, r.risk_title,
		       st.name  AS source_register_name,
		       COALESCE(rs.risk_level, '')      AS risk_level,
		       COALESCE(rs.color_code, '')      AS risk_level_color,
		       COALESCE(owner.display_name, '') AS owner_name,
		       COALESCE(asgn.display_name, '')  AS assigner_name,
		       r.workflow_status,
		       r.risk_type,
		       r.implementation_date,
		       r.rejection_comment,
		       r.rejection_stage,
		       r.created_at,
		       COUNT(*) OVER() AS total_count
		FROM risk r
		LEFT JOIN risk_team st ON st.id = r.source_register_id
		LEFT JOIN ` + "`user`" + ` owner ON owner.id = r.owner_id
		LEFT JOIN ` + "`user`" + ` asgn  ON asgn.id  = r.assigner_id` + effectiveScoreLeftJoin

	var args []any
	var where []string

	if len(filter.Statuses) > 0 {
		where = append(where, "r.workflow_status IN ("+placeholders(len(filter.Statuses))+")")
		for _, s := range filter.Statuses {
			args = append(args, s)
		}
	}
	if len(filter.TeamIDs) > 0 {
		where = append(where, "r.source_register_id IN ("+placeholders(len(filter.TeamIDs))+")")
		for _, id := range filter.TeamIDs {
			args = append(args, id)
		}
	}
	if len(filter.Levels) > 0 {
		where = append(where, "rs.risk_level IN ("+placeholders(len(filter.Levels))+")")
		for _, l := range filter.Levels {
			args = append(args, l)
		}
	}
	if filter.Search != "" {
		where = append(where, "(r.risk_code LIKE ? OR r.risk_title LIKE ?)")
		like := "%" + filter.Search + "%"
		args = append(args, like, like)
	}
	if len(filter.RiskTypes) > 0 {
		where = append(where, "r.risk_type IN ("+placeholders(len(filter.RiskTypes))+")")
		for _, t := range filter.RiskTypes {
			args = append(args, t)
		}
	}
	if len(filter.OwnerIDs) > 0 {
		where = append(where, "r.owner_id IN ("+placeholders(len(filter.OwnerIDs))+")")
		for _, id := range filter.OwnerIDs {
			args = append(args, id)
		}
	}
	if filter.SubmittedFrom != "" {
		where = append(where, "r.created_at >= ?")
		args = append(args, filter.SubmittedFrom+" 00:00:00")
	}
	if filter.SubmittedTo != "" {
		where = append(where, "r.created_at <= ?")
		args = append(args, filter.SubmittedTo+" 23:59:59")
	}
	if filter.DueFrom != "" {
		where = append(where, "r.implementation_date >= ?")
		args = append(args, filter.DueFrom)
	}
	if filter.DueTo != "" {
		where = append(where, "r.implementation_date <= ?")
		args = append(args, filter.DueTo)
	}
	if filter.DueOverdueOnly {
		where = append(where, "r.implementation_date IS NOT NULL AND r.implementation_date < CURDATE()")
	}

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY r.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list risks: %w", err)
	}
	defer rows.Close()

	page := &model.RiskListPage{
		Items:  make([]*model.RiskListItem, 0),
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}
	for rows.Next() {
		var item model.RiskListItem
		var createdAt []byte
		if err := rows.Scan(
			&item.ID, &item.RiskCode, &item.RiskTitle,
			&item.SourceRegisterName, &item.RiskLevel, &item.RiskLevelColor,
			&item.OwnerName, &item.AssignerName,
			&item.WorkflowStatus, &item.RiskType, &item.ImplementationDate,
			&item.RejectionComment, &item.RejectionStage, &createdAt,
			&page.Total,
		); err != nil {
			return nil, fmt.Errorf("scan risk list item: %w", err)
		}
		item.CreatedAt = string(createdAt)
		page.Items = append(page.Items, &item)
	}
	return page, rows.Err()
}

func (r *riskRepository) GetByID(ctx context.Context, id int) (*model.RiskDetail, error) {
	var d model.RiskDetail
	var createdAt, updatedAt []byte
	var complianceApprovalDate, ownerFirstApprovedAt []byte
	var scoreID, scoreLikelihood, scoreImpact, scoreRating sql.NullInt64
	var scoreLevel, scoreColor sql.NullString
	var effScoreID, effScoreLikelihood, effScoreImpact, effScoreRating sql.NullInt64
	var effScoreLevel, effScoreColor sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT r.id, r.risk_code, r.risk_year, r.risk_quarter,
		       r.risk_title, r.risk_description, r.risk_identified_date,
		       r.identified_by_type, r.identified_by_user_id, r.identified_by_name,
		       r.assigner_id, r.owner_id,
		       r.impact_description, r.treatment_strategy,
		       r.assignment_team_id,
		       r.progress, r.implementation_date, r.reassessment_date,
		       r.git_issue_url, r.email_subject, r.remarks,
		       r.workflow_status, r.risk_type, r.rejection_comment, r.rejection_stage, r.owner_first_approved_at, r.compliance_approval_date,
		       r.created_at, r.updated_at,
		       COALESCE(st.name,'') AS source_register_name,
		       COALESCE(at.name,'') AS assignment_team_name,
		       COALESCE(owner.display_name,'') AS owner_name,
		       COALESCE(asgn.display_name,'')  AS assigner_name,
		       ibu.display_name                AS identified_by_user_name,
		       ca.display_name                 AS compliance_approver_name,
		       rs.id, rs.likelihood, rs.impact, rs.risk_rating, rs.risk_level, rs.color_code,
		       ers.id, ers.likelihood, ers.impact, ers.risk_rating, ers.risk_level, ers.color_code
		FROM risk r
		LEFT JOIN risk_team st ON st.id = r.source_register_id
		LEFT JOIN risk_team at ON at.id = r.assignment_team_id
		LEFT JOIN `+"`user`"+` owner ON owner.id = r.owner_id
		LEFT JOIN `+"`user`"+` asgn  ON asgn.id  = r.assigner_id
		LEFT JOIN `+"`user`"+` ibu   ON ibu.id   = r.identified_by_user_id
		LEFT JOIN `+"`user`"+` ca    ON ca.id    = r.compliance_approval_by
		LEFT JOIN risk_score rs ON rs.id = r.gross_score_id
		LEFT JOIN risk_score ers ON ers.id = COALESCE(
			(SELECT ra.score_id
			   FROM risk_assessment ra
			  WHERE ra.risk_id = r.id
			  ORDER BY ra.created_at DESC, ra.id DESC
			  LIMIT 1),
			r.gross_score_id)
		WHERE r.id = ?`, id,
	).Scan(
		&d.ID, &d.RiskCode, &d.RiskYear, &d.RiskQuarter,
		&d.RiskTitle, &d.RiskDescription, &d.RiskIdentifiedDate,
		&d.IdentifiedByType, &d.IdentifiedByUserID, &d.IdentifiedByName,
		&d.AssignerID, &d.OwnerID,
		&d.ImpactDescription, &d.TreatmentStrategy,
		&d.AssignmentTeamID,
		&d.Progress, &d.ImplementationDate, &d.ReassessmentDate,
		&d.GitIssueURL, &d.EmailSubject, &d.Remarks,
		&d.WorkflowStatus, &d.RiskType, &d.RejectionComment, &d.RejectionStage, &ownerFirstApprovedAt, &complianceApprovalDate,
		&createdAt, &updatedAt,
		&d.SourceRegisterName, &d.AssignmentTeamName,
		&d.OwnerName, &d.AssignerName,
		&d.IdentifiedByUserName, &d.ComplianceApproverName,
		&scoreID, &scoreLikelihood, &scoreImpact, &scoreRating, &scoreLevel, &scoreColor,
		&effScoreID, &effScoreLikelihood, &effScoreImpact, &effScoreRating, &effScoreLevel, &effScoreColor,
	)
	if err == sql.ErrNoRows {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: fmt.Sprintf("risk %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("get risk by id: %w", err)
	}

	d.CreatedAt = string(createdAt)
	d.UpdatedAt = string(updatedAt)
	if ownerFirstApprovedAt != nil {
		s := string(ownerFirstApprovedAt)
		d.OwnerFirstApprovedAt = &s
	}
	if complianceApprovalDate != nil {
		s := string(complianceApprovalDate)
		d.ComplianceApprovalDate = &s
	}

	if scoreID.Valid {
		d.GrossScore = &model.RiskScore{
			ID:         int(scoreID.Int64),
			Likelihood: int(scoreLikelihood.Int64),
			Impact:     int(scoreImpact.Int64),
			RiskRating: int(scoreRating.Int64),
			RiskLevel:  scoreLevel.String,
			ColorCode:  scoreColor.String,
		}
	}
	if effScoreID.Valid {
		d.EffectiveScore = &model.RiskScore{
			ID:         int(effScoreID.Int64),
			Likelihood: int(effScoreLikelihood.Int64),
			Impact:     int(effScoreImpact.Int64),
			RiskRating: int(effScoreRating.Int64),
			RiskLevel:  effScoreLevel.String,
			ColorCode:  effScoreColor.String,
		}
	}

	// Compliance references
	refRows, err := r.db.QueryContext(ctx, `
		SELECT scr.id, scr.name, scr.description
		FROM risk_compliance_reference rcr
		JOIN risk_security_compliance_reference scr ON scr.id = rcr.reference_id
		WHERE rcr.risk_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("fetch compliance refs: %w", err)
	}
	defer refRows.Close()
	for refRows.Next() {
		var ref model.ComplianceReference
		if err := refRows.Scan(&ref.ID, &ref.Name, &ref.Description); err != nil {
			return nil, fmt.Errorf("scan compliance ref: %w", err)
		}
		d.ComplianceReferences = append(d.ComplianceReferences, ref)
	}
	if d.ComplianceReferences == nil {
		d.ComplianceReferences = []model.ComplianceReference{}
	}

	// Action plan + steps
	var ap model.ActionPlanDetail
	apErr := r.db.QueryRowContext(ctx,
		"SELECT id, action_owner_id, description, status, plan_type FROM risk_action_plan WHERE risk_id = ? AND plan_type = 'STANDARD' LIMIT 1", id,
	).Scan(&ap.ID, &ap.ActionOwnerID, &ap.Description, &ap.Status, &ap.PlanType)
	if apErr != nil && apErr != sql.ErrNoRows {
		return nil, fmt.Errorf("fetch action plan: %w", apErr)
	}
	if apErr == nil {
		stepRows, err := r.db.QueryContext(ctx,
			"SELECT id, plan_id, step_no, description, status, completed_date FROM risk_action_step WHERE plan_id = ? ORDER BY step_no", ap.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch action steps: %w", err)
		}
		defer stepRows.Close()
		for stepRows.Next() {
			var step model.ActionPlanStep
			if err := stepRows.Scan(&step.ID, &step.PlanID, &step.StepNo, &step.Description, &step.Status, &step.CompletedDate); err != nil {
				return nil, fmt.Errorf("scan action step: %w", err)
			}
			ap.Steps = append(ap.Steps, step)
		}
		if ap.Steps == nil {
			ap.Steps = []model.ActionPlanStep{}
		}
		d.ActionPlan = &ap
	}

	// Assessments (most recent first)
	assRows, err := r.db.QueryContext(ctx, `
		SELECT ra.id, ra.risk_id, ra.score_id, ra.progress, ra.reassessment_date,
		       ra.assessed_by, ra.created_at,
		       rs.likelihood, rs.impact, rs.risk_rating, rs.risk_level, rs.color_code
		FROM risk_assessment ra
		JOIN risk_score rs ON rs.id = ra.score_id
		WHERE ra.risk_id = ?
		ORDER BY ra.created_at DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("fetch assessments: %w", err)
	}
	defer assRows.Close()
	for assRows.Next() {
		a, err := scanAssessment(assRows)
		if err != nil {
			return nil, err
		}
		d.Assessments = append(d.Assessments, *a)
	}
	if d.Assessments == nil {
		d.Assessments = []model.RiskAssessment{}
	}

	// Append the gross score as a synthetic, oldest entry so the log shows the
	// full lineage (gross -> reassessment -> reassessment ...), not just the
	// reassessments. Real assessments were appended above newest-first, so
	// this lands last.
	if d.GrossScore != nil {
		baselineDate := d.CreatedAt
		if d.RiskIdentifiedDate != nil && *d.RiskIdentifiedDate != "" {
			baselineDate = *d.RiskIdentifiedDate
		}
		d.Assessments = append(d.Assessments, model.RiskAssessment{
			RiskID:             d.ID,
			ScoreID:            d.GrossScore.ID,
			ReassessmentDate:   baselineDate,
			ResidualLikelihood: d.GrossScore.Likelihood,
			ResidualImpact:     d.GrossScore.Impact,
			ResidualRating:     d.GrossScore.RiskRating,
			ResidualLevel:      d.GrossScore.RiskLevel,
			ResidualColorCode:  d.GrossScore.ColorCode,
			IsInitial:          true,
		})
	}

	return &d, nil
}

// GetWorkflowStatus fetches only workflow_status, for callers that just need to
// guard a status transition without paying for GetByID's full join + related-entity queries.
func (r *riskRepository) GetWorkflowStatus(ctx context.Context, id int) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, "SELECT workflow_status FROM risk WHERE id = ?", id).Scan(&status)
	if err == sql.ErrNoRows {
		return "", &apierror.Error{StatusCode: http.StatusNotFound, Body: fmt.Sprintf("risk %d not found", id)}
	}
	if err != nil {
		return "", fmt.Errorf("get workflow status: %w", err)
	}
	return status, nil
}

func (r *riskRepository) Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error {
	// Only implementation_date, email_subject, and action_steps require re-approval when changed on an IN_REMEDIATION risk.
	var curImplDate, curEmailSubject, curStatus sql.NullString
	var ownerFirstApprovedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		"SELECT implementation_date, email_subject, owner_first_approved_at, workflow_status FROM risk WHERE id = ?", id,
	).Scan(&curImplDate, &curEmailSubject, &ownerFirstApprovedAt, &curStatus)
	if err != nil {
		return fmt.Errorf("fetch current risk for update: %w", err)
	}
	if curStatus.String == model.StatusClosed {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "risk is closed and can no longer be edited"}
	}

	// Gross score and reassessment date are full-edit-only: ignore them once a
	// risk owner has approved the risk at least once.
	if ownerFirstApprovedAt.Valid {
		req.GrossScoreID = nil
		req.ReassessmentDate = ""
	}

	restrictedChanged := false
	var changelogArgs []any

	checkAndLog := func(field, oldVal, newVal string) {
		if oldVal != newVal && newVal != "" {
			restrictedChanged = true
			oldJSON, _ := json.Marshal(oldVal)
			newJSON, _ := json.Marshal(newVal)
			changelogArgs = append(changelogArgs, id, updatedBy, field, string(oldJSON), string(newJSON))
		}
	}
	checkAndLog("implementation_date", curImplDate.String, req.ImplementationDate)
	checkAndLog("email_subject", curEmailSubject.String, req.EmailSubject)
	stepsChanged := false
	if len(req.ActionSteps) > 0 {
		var planID int
		planErr := r.db.QueryRowContext(ctx,
			"SELECT id FROM risk_action_plan WHERE risk_id = ? AND plan_type = 'STANDARD' LIMIT 1", id,
		).Scan(&planID)
		if planErr != nil {
			stepsChanged = true
		} else {
			stepRows, stepErr := r.db.QueryContext(ctx,
				"SELECT id, description FROM risk_action_step WHERE plan_id = ? ORDER BY step_no", planID)
			if stepErr != nil {
				stepsChanged = true
			} else {
				type curStep struct {
					id   int
					desc string
				}
				var curSteps []curStep
				for stepRows.Next() {
					var s curStep
					if stepRows.Scan(&s.id, &s.desc) == nil {
						curSteps = append(curSteps, s)
					}
				}
				stepRows.Close()
				if len(curSteps) != len(req.ActionSteps) {
					stepsChanged = true
				} else {
					for i, step := range req.ActionSteps {
						if step.ID == nil || *step.ID != curSteps[i].id || curSteps[i].desc != step.Description {
							stepsChanged = true
							break
						}
					}
				}
			}
		}
		if stepsChanged {
			restrictedChanged = true
		}
	}

	// Switching identified_by_type invalidates the counterpart field: an EMPLOYEE
	// risk keeps no free-text name, a TOOL/EXTERNAL_PERSON risk keeps no user id.
	clearIdentifiedByUser := req.IdentifiedByType == "EXTERNAL_PERSON" || req.IdentifiedByType == "TOOL"
	clearIdentifiedByName := req.IdentifiedByType == "EMPLOYEE"

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Update the risk row.
	if _, err = tx.ExecContext(ctx, `
		UPDATE risk SET
			risk_title = ?, risk_description = ?,
			risk_identified_date = COALESCE(NULLIF(?,''), risk_identified_date),
			identified_by_type = COALESCE(NULLIF(?,''), identified_by_type),
			identified_by_user_id = CASE WHEN ? THEN NULL ELSE COALESCE(?, identified_by_user_id) END,
			identified_by_name = CASE WHEN ? THEN NULL ELSE COALESCE(?, identified_by_name) END,
			assigner_id = COALESCE(?, assigner_id),
			owner_id = COALESCE(?, owner_id),
			impact_description = COALESCE(NULLIF(?,''), impact_description),
			progress = COALESCE(NULLIF(?,''), progress),
			git_issue_url = COALESCE(NULLIF(?,''), git_issue_url),
			email_subject = ?,
			remarks = COALESCE(NULLIF(?,''), remarks),
			implementation_date = COALESCE(NULLIF(?,''), implementation_date),
			reassessment_date = COALESCE(NULLIF(?,''), reassessment_date),
			treatment_strategy = COALESCE(NULLIF(?,''), treatment_strategy),
			assignment_team_id = COALESCE(?, assignment_team_id),
			gross_score_id = COALESCE(?, gross_score_id),
			updated_by = ?, updated_at = NOW()
		WHERE id = ?`,
		req.RiskTitle, req.RiskDescription,
		req.RiskIdentifiedDate, req.IdentifiedByType,
		clearIdentifiedByUser, req.IdentifiedByUserID,
		clearIdentifiedByName, req.IdentifiedByName,
		req.AssignerID, req.OwnerID,
		req.ImpactDescription,
		req.Progress, req.GitIssueURL, req.EmailSubject, req.Remarks,
		req.ImplementationDate, req.ReassessmentDate, req.TreatmentStrategy, req.AssignmentTeamID,
		req.GrossScoreID,
		updatedBy, id,
	); err != nil {
		return fmt.Errorf("update risk: %w", err)
	}

	// Update compliance references if provided.
	if req.ComplianceReferenceIDs != nil {
		if _, err = tx.ExecContext(ctx, "DELETE FROM risk_compliance_reference WHERE risk_id = ?", id); err != nil {
			return fmt.Errorf("clear compliance refs: %w", err)
		}
		for _, refID := range req.ComplianceReferenceIDs {
			if _, err = tx.ExecContext(ctx,
				"INSERT INTO risk_compliance_reference (risk_id, reference_id) VALUES (?, ?)", id, refID,
			); err != nil {
				return fmt.Errorf("insert compliance ref %d: %w", refID, err)
			}
		}
	}

	// Update action plan description + owner if provided.
	if req.ActionPlanDescription != "" || req.ActionOwnerID != nil {
		if _, err = tx.ExecContext(ctx, `
			UPDATE risk_action_plan SET
				description = COALESCE(NULLIF(?,''), description),
				action_owner_id = COALESCE(?, action_owner_id),
				updated_by = ?, updated_at = NOW()
			WHERE risk_id = ? AND plan_type = 'STANDARD'`,
			req.ActionPlanDescription, req.ActionOwnerID, updatedBy, id,
		); err != nil {
			return fmt.Errorf("update action plan: %w", err)
		}
	}

	// Diff action steps by ID so untouched steps keep their status and
	// completed_date: update existing steps in place, insert only new ones,
	// delete only steps removed from the payload.
	if stepsChanged {
		var planID int
		if err = tx.QueryRowContext(ctx,
			"SELECT id FROM risk_action_plan WHERE risk_id = ? AND plan_type = 'STANDARD' LIMIT 1", id,
		).Scan(&planID); err != nil {
			return fmt.Errorf("find action plan for step update: %w", err)
		}

		existing := make(map[int]bool)
		idRows, err := tx.QueryContext(ctx, "SELECT id FROM risk_action_step WHERE plan_id = ?", planID)
		if err != nil {
			return fmt.Errorf("load existing step ids: %w", err)
		}
		for idRows.Next() {
			var stepID int
			if err = idRows.Scan(&stepID); err != nil {
				idRows.Close()
				return fmt.Errorf("scan existing step id: %w", err)
			}
			existing[stepID] = true
		}
		idRows.Close()
		if err = idRows.Err(); err != nil {
			return fmt.Errorf("iterate existing step ids: %w", err)
		}

		kept := make(map[int]bool)
		for i, step := range req.ActionSteps {
			// Stale or foreign IDs (not in this plan) are treated as new steps.
			if step.ID != nil && existing[*step.ID] {
				if _, err = tx.ExecContext(ctx, `
					UPDATE risk_action_step SET step_no = ?, description = ?, updated_by = ?
					WHERE id = ? AND plan_id = ?`,
					i+1, step.Description, updatedBy, *step.ID, planID,
				); err != nil {
					return fmt.Errorf("update step %d: %w", i+1, err)
				}
				kept[*step.ID] = true
			} else {
				if _, err = tx.ExecContext(ctx, `
					INSERT INTO risk_action_step (plan_id, step_no, description, status, created_by, updated_by)
					VALUES (?, ?, ?, 'PENDING', ?, ?)`,
					planID, i+1, step.Description, updatedBy, updatedBy,
				); err != nil {
					return fmt.Errorf("insert new step %d: %w", i+1, err)
				}
			}
		}

		for stepID := range existing {
			if !kept[stepID] {
				if _, err = tx.ExecContext(ctx,
					"DELETE FROM risk_action_step WHERE id = ? AND plan_id = ?", stepID, planID,
				); err != nil {
					return fmt.Errorf("delete removed step %d: %w", stepID, err)
				}
			}
		}
	}

	// Write changelog entries for changed restricted fields.
	for i := 0; i < len(changelogArgs); i += 5 {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO risk_change_log (risk_id, created_by, action, field_changed, old_value, new_value)
			VALUES (?, ?, 'UPDATE', ?, ?, ?)`,
			changelogArgs[i], changelogArgs[i+1], changelogArgs[i+2],
			changelogArgs[i+3], changelogArgs[i+4],
		); err != nil {
			return fmt.Errorf("insert changelog: %w", err)
		}
	}
	if stepsChanged {
		if _, err = tx.ExecContext(ctx,
			"INSERT INTO risk_change_log (risk_id, created_by, action, field_changed) VALUES (?, ?, 'UPDATE', 'action_steps')",
			id, updatedBy,
		); err != nil {
			return fmt.Errorf("insert action_steps changelog: %w", err)
		}
	}

	// If a restricted field changed on an IN_REMEDIATION risk, atomically mark it
	// as UPDATED and move it to PENDING_AMENDMENT in the same transaction. The
	// status was read before the transaction began, so guard the write with it:
	// a concurrent transition (e.g. Complete) rolls the whole edit back with 409.
	if restrictedChanged && curStatus.String == model.StatusInRemediation {
		res, aerr := tx.ExecContext(ctx,
			"UPDATE risk SET risk_type = ?, workflow_status = ?, updated_by = ?, updated_at = NOW() WHERE id = ? AND workflow_status = ?",
			model.RiskTypeUpdated, model.StatusPendingAmendment, updatedBy, id, model.StatusInRemediation,
		)
		if aerr != nil {
			return fmt.Errorf("set amendment status: %w", aerr)
		}
		n, aerr := res.RowsAffected()
		if aerr != nil {
			return fmt.Errorf("set amendment status rows: %w", aerr)
		}
		if n == 0 {
			return &apierror.Error{StatusCode: http.StatusConflict, Body: "concurrent modification: workflow status changed"}
		}
	}

	return tx.Commit()
}

func (r *riskRepository) TransitionStatus(ctx context.Context, id int, fromStatus, toStatus, updatedBy string) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE risk SET workflow_status = ?, updated_by = ?, updated_at = NOW() WHERE id = ? AND workflow_status = ?",
		toStatus, updatedBy, id, fromStatus,
	)
	if err != nil {
		return fmt.Errorf("transition status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("transition status rows: %w", err)
	}
	if n == 0 {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "concurrent modification: workflow status changed"}
	}
	return nil
}

func (r *riskRepository) RejectTransition(ctx context.Context, id int, comment, stage, fromStatus, updatedBy string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE risk
		 SET rejection_comment = ?, rejection_stage = ?, workflow_status = ?,
		     updated_by = ?, updated_at = NOW()
		 WHERE id = ? AND workflow_status = ?`,
		comment, stage, model.StatusPendingRevision, updatedBy, id, fromStatus,
	)
	if err != nil {
		return fmt.Errorf("reject transition: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reject transition rows: %w", err)
	}
	if n == 0 {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "concurrent modification: workflow status changed"}
	}
	return nil
}

func (r *riskRepository) ResubmitTransition(ctx context.Context, id int, fromStatus, toStatus, updatedBy string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE risk
		 SET rejection_comment = NULL, rejection_stage = NULL, workflow_status = ?,
		     updated_by = ?, updated_at = NOW()
		 WHERE id = ? AND workflow_status = ?`,
		toStatus, updatedBy, id, fromStatus,
	)
	if err != nil {
		return fmt.Errorf("resubmit transition: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("resubmit transition rows: %w", err)
	}
	if n == 0 {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "concurrent modification: workflow status changed"}
	}
	return nil
}

func (r *riskRepository) SetRiskType(ctx context.Context, id int, riskType, updatedBy string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE risk SET risk_type = ?, updated_by = ?, updated_at = NOW() WHERE id = ?",
		riskType, updatedBy, id,
	)
	return err
}

func (r *riskRepository) SetOwnerFirstApprovedAt(ctx context.Context, id int, updatedBy string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE risk SET owner_first_approved_at = NOW(), updated_by = ?, updated_at = NOW() WHERE id = ? AND owner_first_approved_at IS NULL",
		updatedBy, id,
	)
	return err
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
