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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type actionPlanRepository struct{ c *entityclient.Client }

// NewActionPlanRepository creates a Compliance Entity-backed repository.ActionPlanRepository.
//
// Used for additional plans added after registration (POST /risks/{id}/action-plans
// once a risk is ESCALATED) and for completing plans of either type — STANDARD
// plans are still created inline as part of risk creation (repository/entity/risk.go),
// a separate path per the note this file used to carry in stubs.go.
func NewActionPlanRepository(c *entityclient.Client) repository.ActionPlanRepository {
	return &actionPlanRepository{c: c}
}

// entActionPlan is the entity's camelCase action plan.
type entActionPlan struct {
	ID            int     `json:"id"`
	RiskID        int     `json:"riskId"`
	ActionOwnerID *int    `json:"actionOwnerId"`
	Description   *string `json:"description"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completedDate"`
	PlanType      string  `json:"planType"`
	CreatedBy     *string `json:"createdBy"`
}

func (e entActionPlan) toModel() *model.ActionPlan {
	return &model.ActionPlan{
		ID:            e.ID,
		RiskID:        e.RiskID,
		ActionOwnerID: e.ActionOwnerID,
		Description:   e.Description,
		Status:        e.Status,
		CompletedDate: e.CompletedDate,
		PlanType:      e.PlanType,
		CreatedBy:     e.CreatedBy,
	}
}

func (r *actionPlanRepository) List(ctx context.Context, riskID int) ([]*model.ActionPlan, error) {
	var resp struct {
		Plans []entActionPlan `json:"plans"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/%d/action-plans", riskID), &resp); err != nil {
		return nil, fmt.Errorf("list action plans for risk %d: %w", riskID, err)
	}
	plans := make([]*model.ActionPlan, 0, len(resp.Plans))
	for _, p := range resp.Plans {
		plans = append(plans, p.toModel())
	}
	return plans, nil
}

func (r *actionPlanRepository) GetByID(ctx context.Context, planID int) (*model.ActionPlan, error) {
	var e entActionPlan
	if err := r.c.Get(ctx, fmt.Sprintf("/action-plans/%d", planID), &e); err != nil {
		return nil, fmt.Errorf("get action plan %d: %w", planID, err)
	}
	return e.toModel(), nil
}

func (r *actionPlanRepository) Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error) {
	body := map[string]any{
		"description":   nullableString(req.Description),
		"actionOwnerId": req.ActionOwnerID,
		"planType":      req.PlanType,
		"createdBy":     createdBy,
		"steps":         req.Steps,
	}
	var e entActionPlan
	if err := r.c.Post(ctx, fmt.Sprintf("/risks/%d/action-plans", riskID), body, &e); err != nil {
		return nil, fmt.Errorf("create action plan for risk %d: %w", riskID, err)
	}
	return e.toModel(), nil
}

// entActionStep is the entity's camelCase action step.
type entActionStep struct {
	ID            int     `json:"id"`
	PlanID        int     `json:"planId"`
	StepNo        int     `json:"stepNo"`
	Description   *string `json:"description"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completedDate"`
}

func (e entActionStep) toModel() *model.ActionPlanStep {
	return &model.ActionPlanStep{
		ID:            e.ID,
		PlanID:        e.PlanID,
		StepNo:        e.StepNo,
		Description:   e.Description,
		Status:        e.Status,
		CompletedDate: e.CompletedDate,
	}
}

func (r *actionPlanRepository) ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error) {
	var resp struct {
		Steps []entActionStep `json:"steps"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/action-plans/%d/steps", planID), &resp); err != nil {
		return nil, fmt.Errorf("list steps for plan %d: %w", planID, err)
	}
	steps := make([]*model.ActionPlanStep, 0, len(resp.Steps))
	for _, s := range resp.Steps {
		steps = append(steps, s.toModel())
	}
	return steps, nil
}

func (r *actionPlanRepository) UpdateStep(ctx context.Context, planID, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error {
	body := map[string]any{
		"description":   nullableString(req.Description),
		"status":        nullableString(req.Status),
		"completedDate": req.CompletedDate,
		"updatedBy":     updatedBy,
	}
	if err := r.c.Patch(ctx, fmt.Sprintf("/action-plans/%d/steps/%d", planID, stepID), body, nil); err != nil {
		return fmt.Errorf("update step %d on plan %d: %w", stepID, planID, err)
	}
	return nil
}

func (r *actionPlanRepository) Complete(ctx context.Context, planID int, updatedBy string) (*model.ActionPlan, error) {
	body := map[string]any{"updatedBy": updatedBy}
	var e entActionPlan
	if err := r.c.Post(ctx, fmt.Sprintf("/action-plans/%d/complete", planID), body, &e); err != nil {
		return nil, fmt.Errorf("complete action plan %d: %w", planID, err)
	}
	return e.toModel(), nil
}
