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
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskActionPlanService struct {
	repo            repository.RiskActionPlanRepository
	stepRepo        repository.RiskActionStepRepository
	escalationSvc   RiskEscalationService
	riskSvc         RiskService
	notificationSvc RiskNotificationService
	userSvc         UserService
}

// NewRiskActionPlanService constructs a RiskActionPlanService. The extra
// dependencies beyond repo back CompleteRiskActionPlan's cascade: verifying
// every step is done, resolving the linked escalation, notifying the Risk
// Assigner and (for a MANAGEMENT plan) the plan's creator, and reverting the
// risk's workflow_status.
func NewRiskActionPlanService(
	repo repository.RiskActionPlanRepository,
	stepRepo repository.RiskActionStepRepository,
	escalationSvc RiskEscalationService,
	riskSvc RiskService,
	notificationSvc RiskNotificationService,
	userSvc UserService,
) RiskActionPlanService {
	return &riskActionPlanService{
		repo:            repo,
		stepRepo:        stepRepo,
		escalationSvc:   escalationSvc,
		riskSvc:         riskSvc,
		notificationSvc: notificationSvc,
		userSvc:         userSvc,
	}
}

// validPlanTypes / validActionPlanStatuses mirror the risk_action_plan.plan_type
// and risk_action_plan.status ENUMs in risk_schema.sql.
// STANDARD is the only plan type. MANAGEMENT plans were how an escalation used
// to be answered and are retired; a comment does that job now.
var validPlanTypes = map[string]bool{"STANDARD": true}
var validActionPlanStatuses = map[string]bool{"PENDING": true, "IN_PROGRESS": true, "COMPLETED": true}

func (s *riskActionPlanService) CreateRiskActionPlan(ctx context.Context, riskID int, req domain.CreateRiskActionPlanRequest) (domain.RiskActionPlan, error) {
	if riskID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	req.PlanType = strings.ToUpper(req.PlanType)
	if !validPlanTypes[req.PlanType] {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planType must be STANDARD"}
	}
	if req.CreatedBy == "" {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	p, err := s.repo.CreateRiskActionPlan(ctx, riskID, req)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	return *p, nil
}

func (s *riskActionPlanService) GetRiskActionPlanByID(ctx context.Context, planID int) (domain.RiskActionPlan, error) {
	if planID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	p, err := s.repo.GetRiskActionPlanByID(ctx, planID)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	return *p, nil
}

func (s *riskActionPlanService) UpdateRiskActionPlan(ctx context.Context, planID int, req domain.UpdateRiskActionPlanRequest) (domain.RiskActionPlan, error) {
	if planID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		if !validActionPlanStatuses[up] {
			return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "status must be PENDING, IN_PROGRESS, or COMPLETED"}
		}
		req.Status = &up
	}
	p, err := s.repo.UpdateRiskActionPlan(ctx, planID, req)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	return *p, nil
}

func (s *riskActionPlanService) ListRiskActionPlans(ctx context.Context, riskID int) (domain.ListRiskActionPlansResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskActionPlansResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	resp, err := s.repo.ListRiskActionPlans(ctx, riskID)
	if err != nil {
		return domain.ListRiskActionPlansResponse{}, err
	}
	if resp.Plans == nil {
		resp.Plans = []domain.RiskActionPlan{}
	}
	return *resp, nil
}

// CompleteRiskActionPlan marks planID COMPLETED once every one of its steps
// is COMPLETED, notifies the Risk Assigner to reassess and resubmit, and —
// only for a MANAGEMENT plan — resolves the linked escalation and reverts the
// risk to IN_REMEDIATION.
//
// Safely retryable: if a MANAGEMENT plan is already COMPLETED but a previous
// call failed partway through the escalation resolution, calling this again
// re-enters resolveEscalation while the escalation is still OPEN and converges
// — reverting the risk (idempotently) and marking the escalation RESOLVED.
// Only once that whole cascade has succeeded does the escalation become
// RESOLVED, after which a further retry safely no-ops.
func (s *riskActionPlanService) CompleteRiskActionPlan(ctx context.Context, planID int, req domain.CompleteRiskActionPlanRequest) (domain.RiskActionPlan, error) {
	if planID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}

	plan, err := s.repo.GetRiskActionPlanByID(ctx, planID)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}

	risk, err := s.riskSvc.GetRiskByID(ctx, plan.RiskID)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	if risk.WorkflowStatus != "IN_REMEDIATION" && risk.WorkflowStatus != "ESCALATED" {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "action plans can only be completed while the risk is IN_REMEDIATION or ESCALATED"}
	}

	if plan.Status != "COMPLETED" {
		steps, err := s.stepRepo.ListRiskActionSteps(ctx, planID)
		if err != nil {
			return domain.RiskActionPlan{}, err
		}
		if len(steps) == 0 {
			return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "action plan has no steps"}
		}
		for _, st := range steps {
			if st.Status != "COMPLETED" {
				return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "all action steps must be COMPLETED before completing the plan"}
			}
		}

		completedDate := time.Now().Format("2006-01-02")
		completedStatus := "COMPLETED"
		updated, err := s.repo.UpdateRiskActionPlan(ctx, planID, domain.UpdateRiskActionPlanRequest{
			Status:        &completedStatus,
			CompletedDate: &completedDate,
			UpdatedBy:     req.UpdatedBy,
		})
		if err != nil {
			return domain.RiskActionPlan{}, err
		}
		plan = updated

		// Best-effort: the plan is already COMPLETED at this point, so a failed
		// notification must not fail the whole request — a retry would skip this
		// block entirely (plan.Status is now COMPLETED) and never send it, making
		// a hard failure here permanently silence the assigner. Swallow rather
		// than return, matching resolveEscalation's notification below.
		_, _ = s.notificationSvc.CreateRiskNotification(ctx, domain.CreateRiskNotificationRequest{
			RecipientID: risk.AssignerID,
			RiskID:      &plan.RiskID,
			Type:        "REASSESSMENT",
			Message:     fmt.Sprintf("The action plan for risk %s is complete — please reassess and resubmit for approval.", risk.RiskCode),
			CreatedBy:   req.UpdatedBy,
		})
	}

	// Completing a plan no longer resolves an escalation or reverts the risk.
	// An escalation is now answered by a comment (the GRC backend's escalation
	// comment endpoint), which is what returns the risk to its assigner; the
	// escalation itself stays OPEN until the assigner submits for completion
	// approval, so the risk remains in the Overdue tab throughout.
	return *plan, nil
}
