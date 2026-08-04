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
	"errors"
	"fmt"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// errNotImplemented is returned by service stubs that are not yet implemented.
var errNotImplemented = errors.New("not implemented")

// ActionPlanService defines business operations for risk action plans and steps.
type ActionPlanService interface {
	List(ctx context.Context, riskID int) ([]*model.ActionPlan, error)
	GetByID(ctx context.Context, riskID, planID int) (*model.ActionPlan, error)
	// Create adds a further STANDARD plan to a risk that already has one. The
	// first plan is still created inline as part of risk registration, a
	// separate path this deliberately doesn't touch (see
	// repository/entity/stubs.go's note on the two paths).
	//
	// It used to create MANAGEMENT plans, which were how an escalation was
	// answered. Escalations are now answered with a comment, so the plan type
	// no longer carries meaning and everything this creates is STANDARD.
	Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error)
	ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error)
	// UpdateStep and Complete are ownership-gated in addition to the
	// CompleteActionSteps privilege the handler already checks: callerEmail
	// must resolve to the plan's action_owner_id.
	// Both take canOverride so a compliance admin can act in the action
	// owner's place; the handler derives it from the caller's privileges.
	UpdateStep(ctx context.Context, riskID, planID, stepID int, req model.UpdateActionPlanStepRequest, callerEmail string, canOverride bool) error
	Complete(ctx context.Context, riskID, planID int, callerEmail string, canOverride bool) (*model.ActionPlan, error)
}

type actionPlanService struct {
	repo     repository.ActionPlanRepository
	userRepo user.Repository
}

// NewActionPlanService constructs an ActionPlanService. userRepo resolves the
// caller's email (from the JWT) to their user.id for the ownership check on
// UpdateStep/Complete.
func NewActionPlanService(repo repository.ActionPlanRepository, userRepo user.Repository) ActionPlanService {
	return &actionPlanService{repo: repo, userRepo: userRepo}
}

func badRequest(msg string) error {
	return &apierror.Error{StatusCode: http.StatusBadRequest, Body: msg}
}

func (s *actionPlanService) List(ctx context.Context, riskID int) ([]*model.ActionPlan, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	return s.repo.List(ctx, riskID)
}

// getForRisk fetches a plan and verifies it belongs to riskID, so a caller
// can't reach another risk's plan just by guessing its planID.
func (s *actionPlanService) getForRisk(ctx context.Context, riskID, planID int) (*model.ActionPlan, error) {
	if planID <= 0 {
		return nil, badRequest("planId must be a positive integer")
	}
	plan, err := s.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan == nil || plan.RiskID != riskID {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: fmt.Sprintf("action plan %d not found for risk %d", planID, riskID)}
	}
	return plan, nil
}

func (s *actionPlanService) GetByID(ctx context.Context, riskID, planID int) (*model.ActionPlan, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	return s.getForRisk(ctx, riskID, planID)
}

func (s *actionPlanService) Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	// Forced rather than validated: callers no longer choose a plan type, and
	// silently accepting "MANAGEMENT" from an old client would recreate the
	// very concept this removed.
	req.PlanType = "STANDARD"
	if createdBy == "" {
		return nil, badRequest("createdBy is required")
	}
	return s.repo.Create(ctx, riskID, req, createdBy)
}

func (s *actionPlanService) ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error) {
	if planID <= 0 {
		return nil, badRequest("planId must be a positive integer")
	}
	return s.repo.ListSteps(ctx, planID)
}

// requireOwner reports whether callerEmail resolves to plan's action_owner_id.
// canOverride short-circuits it for compliance admins, matching the identity
// gates on the approval transitions.
func (s *actionPlanService) requireOwner(ctx context.Context, plan *model.ActionPlan, callerEmail string, canOverride bool) error {
	if canOverride {
		return nil
	}
	caller, err := s.userRepo.GetByEmail(ctx, callerEmail)
	if err != nil {
		return err
	}
	if caller == nil || plan.ActionOwnerID == nil || caller.ID != *plan.ActionOwnerID {
		return &apierror.Error{StatusCode: http.StatusForbidden, Body: "you are not the action owner of this plan"}
	}
	return nil
}

func (s *actionPlanService) UpdateStep(ctx context.Context, riskID, planID, stepID int, req model.UpdateActionPlanStepRequest, callerEmail string, canOverride bool) error {
	if stepID <= 0 {
		return badRequest("stepId must be a positive integer")
	}
	if callerEmail == "" {
		return badRequest("caller email is required")
	}
	plan, err := s.getForRisk(ctx, riskID, planID)
	if err != nil {
		return err
	}
	if err := s.requireOwner(ctx, plan, callerEmail, canOverride); err != nil {
		return err
	}
	return s.repo.UpdateStep(ctx, planID, stepID, req, callerEmail)
}

func (s *actionPlanService) Complete(ctx context.Context, riskID, planID int, callerEmail string, canOverride bool) (*model.ActionPlan, error) {
	if callerEmail == "" {
		return nil, badRequest("caller email is required")
	}
	plan, err := s.getForRisk(ctx, riskID, planID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwner(ctx, plan, callerEmail, canOverride); err != nil {
		return nil, err
	}
	return s.repo.Complete(ctx, planID, callerEmail)
}
