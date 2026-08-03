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

// RiskEscalationRepository defines persistence for risk_escalation.
type RiskEscalationRepository interface {
	CreateRiskEscalation(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (*domain.RiskEscalation, error)
	// Escalate flips the risk IN_REMEDIATION -> ESCALATED and inserts its OPEN
	// escalation row atomically (one transaction), so a failed insert can't
	// leave the risk stranded in ESCALATED with no escalation row.
	Escalate(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (*domain.RiskEscalation, error)
	GetRiskEscalationByID(ctx context.Context, riskID, escalationID int) (*domain.RiskEscalation, error)
	UpdateRiskEscalation(ctx context.Context, riskID, escalationID int, req domain.UpdateRiskEscalationRequest) (*domain.RiskEscalation, error)
	ListRiskEscalations(ctx context.Context, riskID int) ([]domain.RiskEscalation, error)
	// GetOpenByActionPlanID finds the still-OPEN escalation linked to a
	// MANAGEMENT action plan (see CreateRiskActionPlan's linking step). Used
	// by the plan-completion cascade to resolve it. Returns NotFoundError if
	// no OPEN escalation is linked — including when it was already resolved,
	// which lets the cascade be safely retried.
	GetOpenByActionPlanID(ctx context.Context, planID int) (*domain.RiskEscalation, error)
}

type riskEscalationRepo struct{ db *sql.DB }

// NewRiskEscalationRepository constructs a RiskEscalationRepository.
func NewRiskEscalationRepository(db *sql.DB) RiskEscalationRepository {
	return &riskEscalationRepo{db: db}
}

func (r *riskEscalationRepo) CreateRiskEscalation(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (*domain.RiskEscalation, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_escalation
		 (risk_id, new_treatment_strategy, action_plan_id, status, created_by, updated_by)
		 VALUES (?, ?, ?, 'OPEN', ?, ?)`,
		riskID,
		nullableString(req.NewTreatmentStrategy),
		nullableInt(req.ActionPlanID),
		req.CreatedBy, req.CreatedBy,
	)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", riskID)}
		}
		return nil, fmt.Errorf("risk_escalation.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskEscalationByID(ctx, riskID, int(id))
}

// Escalate performs the whole escalation as one transaction: a CAS flip of the
// risk from IN_REMEDIATION to ESCALATED, then the OPEN escalation insert. If the
// insert fails, the flip rolls back — so the risk is never left ESCALATED with
// no escalation row (a stuck state: the daily job skips non-IN_REMEDIATION
// risks, manual escalate requires IN_REMEDIATION, and a MANAGEMENT plan needs an
// open escalation, leaving no recovery path). The CAS also makes a concurrent
// escalate (daily job vs. manual click) fail here rather than insert a duplicate.
func (r *riskEscalationRepo) Escalate(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (*domain.RiskEscalation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk_escalation.Escalate begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`UPDATE risk SET workflow_status = 'ESCALATED', updated_by = ?, updated_at = NOW()
		 WHERE id = ? AND workflow_status = 'IN_REMEDIATION'`,
		req.CreatedBy, riskID)
	if err != nil {
		return nil, fmt.Errorf("risk_escalation.Escalate flip: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Zero rows: the risk doesn't exist, or it's no longer IN_REMEDIATION
		// (lost a concurrent escalate race, or moved on). Distinguish the two,
		// mirroring risk.UpdateRisk's CAS handling.
		var current string
		if err := tx.QueryRowContext(ctx,
			"SELECT workflow_status FROM risk WHERE id = ?", riskID).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", riskID)}
			}
			return nil, fmt.Errorf("risk_escalation.Escalate recheck: %w", err)
		}
		return nil, &apierror.ConflictError{Msg: "risk was modified concurrently, please retry"}
	}

	ins, err := tx.ExecContext(ctx,
		`INSERT INTO risk_escalation
		 (risk_id, new_treatment_strategy, action_plan_id,
		  assigner_lead_email, action_owner_lead_email, status, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, 'OPEN', ?, ?)`,
		riskID,
		nullableString(req.NewTreatmentStrategy),
		nullableInt(req.ActionPlanID),
		nullableString(req.AssignerLeadEmail),
		nullableString(req.ActionOwnerLeadEmail),
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk_escalation.Escalate insert: %w", err)
	}
	id, _ := ins.LastInsertId()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk_escalation.Escalate commit: %w", err)
	}
	return r.GetRiskEscalationByID(ctx, riskID, int(id))
}

func (r *riskEscalationRepo) GetRiskEscalationByID(ctx context.Context, riskID, escalationID int) (*domain.RiskEscalation, error) {
	e, err := scanRiskEscalation(r.db.QueryRowContext(ctx,
		`SELECT id, risk_id, new_treatment_strategy,
		        action_plan_id, decision, assigner_lead_email, action_owner_lead_email,
		        status, created_by, updated_by, created_at, updated_at
		 FROM risk_escalation WHERE id = ? AND risk_id = ?`, escalationID, riskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("escalation %d not found", escalationID)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_escalation.GetByID(%d): %w", escalationID, err)
	}
	return e, nil
}

func (r *riskEscalationRepo) UpdateRiskEscalation(ctx context.Context, riskID, escalationID int, req domain.UpdateRiskEscalationRequest) (*domain.RiskEscalation, error) {
	sets := []string{}
	args := []any{}

	if req.Decision != nil {
		sets = append(sets, "decision = ?")
		args = append(args, *req.Decision)
	}
	if req.NewTreatmentStrategy != nil {
		sets = append(sets, "new_treatment_strategy = ?")
		args = append(args, *req.NewTreatmentStrategy)
	}
	if req.ActionPlanID != nil {
		sets = append(sets, "action_plan_id = ?")
		args = append(args, *req.ActionPlanID)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, escalationID, riskID)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE risk_escalation SET "+strings.Join(sets, ", ")+" WHERE id = ? AND risk_id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("risk_escalation.Update(%d): %w", escalationID, err)
	}
	return r.GetRiskEscalationByID(ctx, riskID, escalationID)
}

func (r *riskEscalationRepo) GetOpenByActionPlanID(ctx context.Context, planID int) (*domain.RiskEscalation, error) {
	e, err := scanRiskEscalation(r.db.QueryRowContext(ctx,
		`SELECT id, risk_id, new_treatment_strategy,
		        action_plan_id, decision, assigner_lead_email, action_owner_lead_email,
		        status, created_by, updated_by, created_at, updated_at
		 FROM risk_escalation WHERE action_plan_id = ? AND status = 'OPEN'`, planID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("no open escalation linked to action plan %d", planID)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_escalation.GetOpenByActionPlanID(%d): %w", planID, err)
	}
	return e, nil
}

func (r *riskEscalationRepo) ListRiskEscalations(ctx context.Context, riskID int) ([]domain.RiskEscalation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, risk_id, new_treatment_strategy,
		        action_plan_id, decision, assigner_lead_email, action_owner_lead_email,
		        status, created_by, updated_by, created_at, updated_at
		 FROM risk_escalation WHERE risk_id = ? ORDER BY created_at DESC`, riskID)
	if err != nil {
		return nil, fmt.Errorf("risk_escalation.List: %w", err)
	}
	defer rows.Close()

	var escalations []domain.RiskEscalation
	for rows.Next() {
		e, err := scanRiskEscalation(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_escalation.List scan: %w", err)
		}
		escalations = append(escalations, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_escalation.List rows: %w", err)
	}
	return escalations, nil
}

func scanRiskEscalation(s scanner) (*domain.RiskEscalation, error) {
	var e domain.RiskEscalation
	var strategy, decision, assignerLead, actionOwnerLead, createdBy, updatedBy sql.NullString
	var actionPlanID sql.NullInt64
	err := s.Scan(
		&e.ID, &e.RiskID,
		&strategy, &actionPlanID,
		&decision, &assignerLead, &actionOwnerLead,
		&e.Status,
		&createdBy, &updatedBy,
		&e.CreatedOn, &e.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	if strategy.Valid {
		e.NewTreatmentStrategy = &strategy.String
	}
	if actionPlanID.Valid {
		v := int(actionPlanID.Int64)
		e.ActionPlanID = &v
	}
	if decision.Valid {
		e.Decision = &decision.String
	}
	if assignerLead.Valid {
		e.AssignerLeadEmail = &assignerLead.String
	}
	if actionOwnerLead.Valid {
		e.ActionOwnerLeadEmail = &actionOwnerLead.String
	}
	if createdBy.Valid {
		e.CreatedBy = &createdBy.String
	}
	if updatedBy.Valid {
		e.UpdatedBy = &updatedBy.String
	}
	return &e, nil
}
