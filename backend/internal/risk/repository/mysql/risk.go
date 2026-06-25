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
	"fmt"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
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
func (r *riskRepository) NextSequenceID(ctx context.Context, sourceRegisterID, year int, quarter string) (int, error) {
	var lastSeq int
	err := r.db.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_register_sequence WHERE risk_team_id = ?",
		sourceRegisterID,
	).Scan(&lastSeq)
	if err == sql.ErrNoRows {
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
//  4. Inserts risk with workflow_status = PENDING_COMPLIANCE_REVIEW
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
			'PENDING_COMPLIANCE_REVIEW', ?, ?
		)`,
		req.Year, req.SourceRegisterID, req.Quarter, riskCode,
		req.RiskTitle, req.RiskDescription, nullableString(req.RiskIdentifiedDate),
		req.IdentifiedByType, req.IdentifiedByUserID, req.IdentifiedByName,
		req.AssignerID, req.OwnerID, nullableString(req.ImpactDescription), grossScoreID,
		req.TreatmentStrategy, req.AssignmentTeamID, nullableString(req.Progress),
		nullableString(req.ImplementationDate), nullableString(req.ReassessmentDate),
		nullableString(req.GitIssueURL), nullableString(req.EmailSubject), nullableString(req.Remarks),
		createdBy, createdBy,
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

func (r *riskRepository) List(ctx context.Context, filter model.ListRisksFilter) ([]*model.Risk, error) {
	// TODO: SELECT with dynamic WHERE clauses based on filter
	return nil, nil
}

func (r *riskRepository) GetByID(ctx context.Context, id int) (*model.Risk, error) {
	// TODO: SELECT * FROM risk WHERE id = ?
	return nil, nil
}

func (r *riskRepository) Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error {
	// TODO: UPDATE risk SET ... WHERE id = ?
	return nil
}

func (r *riskRepository) UpdateStatus(ctx context.Context, id int, status, updatedBy string) error {
	// TODO: UPDATE risk SET workflow_status = ?, updated_by = ? WHERE id = ?
	return nil
}

// nullableString converts an empty string to nil so the DB stores NULL
// rather than an empty string for optional text columns.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
