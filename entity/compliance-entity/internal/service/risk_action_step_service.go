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

package service

import (
	"context"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskActionStepService struct {
	repo     repository.RiskActionStepRepository
	planRepo repository.RiskActionPlanRepository
	riskSvc  RiskService
}

// NewRiskActionStepService constructs a RiskActionStepService. planRepo and
// riskSvc back UpdateRiskActionStep's check that the parent risk is actively
// being remediated before its steps can be touched.
func NewRiskActionStepService(repo repository.RiskActionStepRepository, planRepo repository.RiskActionPlanRepository, riskSvc RiskService) RiskActionStepService {
	return &riskActionStepService{repo: repo, planRepo: planRepo, riskSvc: riskSvc}
}

var validStepStatuses = map[string]bool{
	"PENDING": true, "IN_PROGRESS": true, "COMPLETED": true,
}

// assertStepsModifiable rejects any step create/update/delete unless the parent
// risk is actively being remediated — enforced uniformly so a caller can't
// sidestep the rule by choosing a different verb.
func (s *riskActionStepService) assertStepsModifiable(ctx context.Context, planID int) error {
	plan, err := s.planRepo.GetRiskActionPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	risk, err := s.riskSvc.GetRiskByID(ctx, plan.RiskID)
	if err != nil {
		return err
	}
	if risk.WorkflowStatus != "IN_REMEDIATION" && risk.WorkflowStatus != "ESCALATED" {
		return &apierror.ValidationError{Msg: "action steps can only be modified while the risk is IN_REMEDIATION or ESCALATED"}
	}
	return nil
}

func (s *riskActionStepService) CreateRiskActionStep(ctx context.Context, planID int, req domain.CreateRiskActionStepRequest) (domain.RiskActionStep, error) {
	if planID <= 0 {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if req.StepNo <= 0 {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "stepNo must be a positive integer"}
	}
	if req.CreatedBy == "" {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	if err := s.assertStepsModifiable(ctx, planID); err != nil {
		return domain.RiskActionStep{}, err
	}
	step, err := s.repo.CreateRiskActionStep(ctx, planID, req)
	if err != nil {
		return domain.RiskActionStep{}, err
	}
	return *step, nil
}

func (s *riskActionStepService) GetRiskActionStepByID(ctx context.Context, planID, stepID int) (domain.RiskActionStep, error) {
	if planID <= 0 {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if stepID <= 0 {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "stepId must be a positive integer"}
	}
	step, err := s.repo.GetRiskActionStepByID(ctx, planID, stepID)
	if err != nil {
		return domain.RiskActionStep{}, err
	}
	return *step, nil
}

func (s *riskActionStepService) UpdateRiskActionStep(ctx context.Context, planID, stepID int, req domain.UpdateRiskActionStepRequest) (domain.RiskActionStep, error) {
	if planID <= 0 {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if stepID <= 0 {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "stepId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil && !validStepStatuses[strings.ToUpper(*req.Status)] {
		return domain.RiskActionStep{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
	}

	// The IN_REMEDIATION/ESCALATED guard is enforced atomically inside
	// repo.UpdateRiskActionStep (it locks the risk row in the same transaction
	// as the write), so it isn't repeated here — a separate service-level check
	// would be a TOCTOU that a concurrent transition could race past.
	step, err := s.repo.UpdateRiskActionStep(ctx, planID, stepID, req)
	if err != nil {
		return domain.RiskActionStep{}, err
	}
	return *step, nil
}

func (s *riskActionStepService) DeleteRiskActionStep(ctx context.Context, planID, stepID int) error {
	if planID <= 0 {
		return &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if stepID <= 0 {
		return &apierror.ValidationError{Msg: "stepId must be a positive integer"}
	}
	if err := s.assertStepsModifiable(ctx, planID); err != nil {
		return err
	}
	return s.repo.DeleteRiskActionStep(ctx, planID, stepID)
}

func (s *riskActionStepService) ListRiskActionSteps(ctx context.Context, planID int) (domain.ListRiskActionStepsResponse, error) {
	if planID <= 0 {
		return domain.ListRiskActionStepsResponse{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	steps, err := s.repo.ListRiskActionSteps(ctx, planID)
	if err != nil {
		return domain.ListRiskActionStepsResponse{}, err
	}
	if steps == nil {
		steps = []domain.RiskActionStep{}
	}
	return domain.ListRiskActionStepsResponse{Steps: steps}, nil
}
