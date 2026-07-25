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

// RiskActionPlanRepository defines persistence for risk_action_plan.
type RiskActionPlanRepository interface {
	CreateRiskActionPlan(ctx context.Context, riskID int, req domain.CreateRiskActionPlanRequest) (*domain.RiskActionPlan, error)
	GetRiskActionPlanByID(ctx context.Context, planID int) (*domain.RiskActionPlan, error)
	UpdateRiskActionPlan(ctx context.Context, planID int, req domain.UpdateRiskActionPlanRequest) (*domain.RiskActionPlan, error)
	ListRiskActionPlans(ctx context.Context, riskID int) (*domain.ListRiskActionPlansResponse, error)
}

type riskActionPlanRepo struct{ db *sql.DB }

// NewRiskActionPlanRepository constructs a RiskActionPlanRepository.
func NewRiskActionPlanRepository(db *sql.DB) RiskActionPlanRepository {
	return &riskActionPlanRepo{db: db}
}

func (r *riskActionPlanRepo) CreateRiskActionPlan(ctx context.Context, riskID int, req domain.CreateRiskActionPlanRequest) (*domain.RiskActionPlan, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk_action_plan.Create: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`INSERT INTO risk_action_plan (risk_id, description, action_owner_id, plan_type, status, created_by, updated_by)
		 VALUES (?, ?, ?, ?, 'PENDING', ?, ?)`,
		riskID,
		nullableString(req.Description),
		nullableInt(req.ActionOwnerID),
		req.PlanType,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", riskID)}
		}
		return nil, fmt.Errorf("risk_action_plan.Create: %w", err)
	}
	id, _ := res.LastInsertId()

	// Insert the plan's steps in the same transaction, so a plan is never
	// persisted without the steps it was created with (the caller sends both
	// together). step_no is 1-based by position.
	for i, desc := range req.Steps {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO risk_action_step (plan_id, step_no, description, status, created_by, updated_by)
			 VALUES (?, ?, ?, 'PENDING', ?, ?)`,
			id, i+1, desc, req.CreatedBy, req.CreatedBy); err != nil {
			return nil, fmt.Errorf("risk_action_plan.Create: insert step %d: %w", i+1, err)
		}
	}

	// A MANAGEMENT plan is always the response to an escalation — link it to
	// the risk's currently OPEN escalation so the plan-completion cascade can
	// find its way back to resolve it. See risk_action_plan_service.go.
	//
	// action_plan_id IS NULL guards against a second MANAGEMENT plan stealing
	// an already-linked escalation: without it the re-link would silently
	// orphan the first plan's connection, and completing that first plan would
	// then find no OPEN escalation to resolve, leaving the risk stuck ESCALATED.
	if req.PlanType == "MANAGEMENT" {
		linkRes, err := tx.ExecContext(ctx,
			`UPDATE risk_escalation SET action_plan_id = ? WHERE risk_id = ? AND status = 'OPEN' AND action_plan_id IS NULL`,
			id, riskID)
		if err != nil {
			return nil, fmt.Errorf("risk_action_plan.Create: link escalation: %w", err)
		}
		if n, _ := linkRes.RowsAffected(); n == 0 {
			return nil, &apierror.ValidationError{Msg: fmt.Sprintf("risk %d has no unlinked open escalation to attach a MANAGEMENT plan to", riskID)}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk_action_plan.Create: commit: %w", err)
	}
	return r.GetRiskActionPlanByID(ctx, int(id))
}

func (r *riskActionPlanRepo) GetRiskActionPlanByID(ctx context.Context, planID int) (*domain.RiskActionPlan, error) {
	plan, err := scanRiskActionPlan(r.db.QueryRowContext(ctx,
		`SELECT id, risk_id, action_owner_id, description, status, DATE_FORMAT(completed_date,'%Y-%m-%d'), plan_type, created_by, created_at, updated_at
		 FROM risk_action_plan WHERE id = ?`, planID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("action plan %d not found", planID)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_action_plan.GetByID(%d): %w", planID, err)
	}
	return plan, nil
}

func (r *riskActionPlanRepo) UpdateRiskActionPlan(ctx context.Context, planID int, req domain.UpdateRiskActionPlanRequest) (*domain.RiskActionPlan, error) {
	sets := []string{}
	args := []any{}

	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.ActionOwnerID != nil {
		sets = append(sets, "action_owner_id = ?")
		args = append(args, *req.ActionOwnerID)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	if req.CompletedDate != nil {
		sets = append(sets, "completed_date = ?")
		args = append(args, *req.CompletedDate)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, planID)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE risk_action_plan SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("risk_action_plan.Update(%d): %w", planID, err)
	}
	return r.GetRiskActionPlanByID(ctx, planID)
}

func (r *riskActionPlanRepo) ListRiskActionPlans(ctx context.Context, riskID int) (*domain.ListRiskActionPlansResponse, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, risk_id, action_owner_id, description, status, DATE_FORMAT(completed_date,'%Y-%m-%d'), plan_type, created_by, created_at, updated_at
		 FROM risk_action_plan WHERE risk_id = ? ORDER BY created_at DESC`,
		riskID)
	if err != nil {
		return nil, fmt.Errorf("risk_action_plan.List: %w", err)
	}
	defer rows.Close()

	var plans []domain.RiskActionPlan
	for rows.Next() {
		plan, err := scanRiskActionPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_action_plan.List scan: %w", err)
		}
		plans = append(plans, *plan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_action_plan.List rows: %w", err)
	}
	return &domain.ListRiskActionPlansResponse{Plans: plans}, nil
}

func scanRiskActionPlan(s scanner) (*domain.RiskActionPlan, error) {
	var p domain.RiskActionPlan
	var ownerID sql.NullInt64
	var desc, completedDate, createdBy sql.NullString
	err := s.Scan(
		&p.ID, &p.RiskID,
		&ownerID, &desc,
		&p.Status, &completedDate,
		&p.PlanType, &createdBy,
		&p.CreatedOn, &p.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	if ownerID.Valid {
		v := int(ownerID.Int64)
		p.ActionOwnerID = &v
	}
	if desc.Valid {
		p.Description = &desc.String
	}
	if completedDate.Valid {
		p.CompletedDate = &completedDate.String
	}
	if createdBy.Valid {
		p.CreatedBy = &createdBy.String
	}
	return &p, nil
}
