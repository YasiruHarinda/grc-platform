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

package service

import (
	"context"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

// ActionPlanService defines business operations for risk action plans and steps.
type ActionPlanService interface {
	List(ctx context.Context, riskID int) ([]*model.ActionPlan, error)
	GetByID(ctx context.Context, riskID, planID int) (*model.ActionPlan, error)
	Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error)
	Update(ctx context.Context, riskID, planID int, req model.UpdateActionPlanRequest, updatedBy string) error
	ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error)
	AddStep(ctx context.Context, planID int, req model.AddActionPlanStepRequest, createdBy string) (*model.ActionPlanStep, error)
	UpdateStep(ctx context.Context, planID, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error
}

type actionPlanService struct {
	repo repository.ActionPlanRepository
}

func NewActionPlanService(repo repository.ActionPlanRepository) ActionPlanService {
	return &actionPlanService{repo: repo}
}

func (s *actionPlanService) List(ctx context.Context, riskID int) ([]*model.ActionPlan, error) {
	// TODO: delegate to repo
	return nil, nil
}

func (s *actionPlanService) GetByID(ctx context.Context, riskID, planID int) (*model.ActionPlan, error) {
	// TODO: delegate to repo; verify plan belongs to riskID
	return nil, nil
}

func (s *actionPlanService) Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error) {
	// TODO: verify risk exists and is in a status that allows plan creation, delegate to repo
	return nil, nil
}

func (s *actionPlanService) Update(ctx context.Context, riskID, planID int, req model.UpdateActionPlanRequest, updatedBy string) error {
	// TODO: verify plan belongs to riskID, delegate to repo
	return nil
}

func (s *actionPlanService) ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error) {
	// TODO: delegate to repo, order by step_no
	return nil, nil
}

func (s *actionPlanService) AddStep(ctx context.Context, planID int, req model.AddActionPlanStepRequest, createdBy string) (*model.ActionPlanStep, error) {
	// TODO: determine next step_no, delegate to repo with stepNo
	return nil, nil
}

func (s *actionPlanService) UpdateStep(ctx context.Context, planID, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error {
	// TODO: verify step belongs to planID, delegate to repo
	return nil
}
