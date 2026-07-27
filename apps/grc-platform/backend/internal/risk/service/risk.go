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

// Package service implements the business logic for the Risk Hub module.
// Handlers call service methods; services call repository methods.
// Business rules (status transition guards, validations, changelog writes)
// live here — not in handlers, not in repositories.
package service

import (
	"context"
	"fmt"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// RiskService defines the business operations for risk lifecycle management.
type RiskService interface {
	List(ctx context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error)
	GetByID(ctx context.Context, id int) (*model.RiskDetail, error)
	Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error)
	NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error)
	Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error

	// Workflow transitions — each validates the current status before advancing.
	OwnerApprove(ctx context.Context, id int, byUserEmail string) error
	ManagementApprove(ctx context.Context, id int, byUserEmail string) error
	Approve(ctx context.Context, id int, byUserEmail string) error
	Reject(ctx context.Context, id int, req model.RejectRiskRequest, fromStatus, byUserEmail string) error
	Complete(ctx context.Context, id int, byUserEmail string) error
	Resubmit(ctx context.Context, id int, byUserEmail string) error
	Close(ctx context.Context, id int, byUserEmail string) error
	Cancel(ctx context.Context, id int, byUserEmail string) error
}

type riskService struct {
	repo           repository.RiskRepository
	actionPlanRepo repository.ActionPlanRepository
}

// NewRiskService creates a RiskService backed by the given repository.
// actionPlanRepo backs Complete's check that remediation work is actually
// finished before a risk can be submitted for completion approval.
func NewRiskService(repo repository.RiskRepository, actionPlanRepo repository.ActionPlanRepository) RiskService {
	return &riskService{repo: repo, actionPlanRepo: actionPlanRepo}
}

func (s *riskService) List(ctx context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error) {
	return s.repo.List(ctx, filter)
}

func (s *riskService) GetByID(ctx context.Context, id int) (*model.RiskDetail, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *riskService) Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error) {
	return s.repo.Create(ctx, req, createdBy)
}

func (s *riskService) NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error) {
	return s.repo.NextSequenceID(ctx, sourceRegisterID)
}

// Update saves risk field changes. If a restricted field changed while the risk is
// IN_REMEDIATION, the repository atomically marks risk_type = UPDATED and moves
// the risk to PENDING_AMENDMENT in the same transaction.
func (s *riskService) Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error {
	return s.repo.Update(ctx, id, req, updatedBy)
}

// OwnerApprove handles three situations:
//   - PENDING_RISK_OWNER_APPROVAL or PENDING_AMENDMENT (initial/amendment approval):
//     Accept+HIGH → PENDING_MANAGEMENT_APPROVAL; otherwise → PENDING_COMPLIANCE_REVIEW
//   - PENDING_OWNER_COMPLETION_APPROVAL (post-remediation): → PENDING_COMPLIANCE_CLOSURE
func (s *riskService) OwnerApprove(ctx context.Context, id int, byUserEmail string) error {
	detail, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	switch detail.WorkflowStatus {
	case model.StatusPendingOwnerApproval, model.StatusPendingAmendment:
		if detail.OwnerFirstApprovedAt == nil {
			if err = s.repo.SetOwnerFirstApprovedAt(ctx, id, byUserEmail); err != nil {
				return err
			}
		}
		next := model.StatusPendingComplianceReview
		if stringVal(detail.TreatmentStrategy) == "ACCEPT" && detail.GrossScore != nil && detail.GrossScore.RiskLevel == "HIGH" {
			next = model.StatusPendingManagementApproval
		}
		return s.repo.TransitionStatus(ctx, id, detail.WorkflowStatus, next, byUserEmail)

	case model.StatusPendingOwnerCompletion:
		return s.repo.TransitionStatus(ctx, id, model.StatusPendingOwnerCompletion, model.StatusPendingComplianceClosure, byUserEmail)

	default:
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be owner-approved from status: %s", detail.WorkflowStatus)}
	}
}

// ManagementApprove moves PENDING_MANAGEMENT_APPROVAL → PENDING_COMPLIANCE_REVIEW.
func (s *riskService) ManagementApprove(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != model.StatusPendingManagementApproval {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be management-approved from status: %s", status)}
	}
	return s.repo.TransitionStatus(ctx, id, model.StatusPendingManagementApproval, model.StatusPendingComplianceReview, byUserEmail)
}

// Approve is the compliance approval step: PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION.
func (s *riskService) Approve(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != model.StatusPendingComplianceReview {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be approved from status: %s", status)}
	}
	return s.repo.TransitionStatus(ctx, id, model.StatusPendingComplianceReview, model.StatusInRemediation, byUserEmail)
}

// Reject routes rejections from any pending-approval stage back to PENDING_REVISION
// and records where the rejection occurred. fromStatus is the status the caller
// was authorized against — the transition is CAS-keyed on it, so a concurrent
// status change fails with 409 instead of rejecting at a stage the caller never
// held the privilege for.
func (s *riskService) Reject(ctx context.Context, id int, req model.RejectRiskRequest, fromStatus, byUserEmail string) error {
	if req.RejectionComment == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "rejection_comment is required"}
	}

	var stage string
	switch fromStatus {
	case model.StatusPendingOwnerApproval, model.StatusPendingAmendment:
		stage = "OWNER"
	case model.StatusPendingManagementApproval:
		stage = "MANAGEMENT"
	case model.StatusPendingComplianceReview:
		stage = "COMPLIANCE"
	case model.StatusPendingOwnerCompletion:
		stage = "COMPLETION_OWNER"
	default:
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be rejected from status: %s", fromStatus)}
	}

	return s.repo.RejectTransition(ctx, id, req.RejectionComment, stage, fromStatus, byUserEmail)
}

// Complete moves IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL. At least
// one of the risk's action plans must be COMPLETED first — not necessarily
// all of them, since an escalation cycle can leave the original STANDARD
// plan permanently abandoned once a MANAGEMENT plan supersedes and completes
// it (that completion is exactly what reverts ESCALATED -> IN_REMEDIATION).
func (s *riskService) Complete(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != model.StatusInRemediation {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be completed from status: %s", status)}
	}

	plans, err := s.actionPlanRepo.List(ctx, id)
	if err != nil {
		return err
	}
	hasCompletedPlan := false
	for _, p := range plans {
		if p.Status == "COMPLETED" {
			hasCompletedPlan = true
			break
		}
	}
	if !hasCompletedPlan {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "cannot submit for approval: the action plan must be completed first"}
	}

	return s.repo.TransitionStatus(ctx, id, model.StatusInRemediation, model.StatusPendingOwnerCompletion, byUserEmail)
}

// Resubmit clears rejection info and moves PENDING_REVISION back to the appropriate approval stage:
// - COMPLETION_OWNER rejection → PENDING_OWNER_COMPLETION_APPROVAL
// - all other rejections → PENDING_RISK_OWNER_APPROVAL
func (s *riskService) Resubmit(ctx context.Context, id int, byUserEmail string) error {
	detail, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if detail.WorkflowStatus != model.StatusPendingRevision {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be resubmitted from status: %s", detail.WorkflowStatus)}
	}
	next := model.StatusPendingOwnerApproval
	if stringVal(detail.RejectionStage) == "COMPLETION_OWNER" {
		next = model.StatusPendingOwnerCompletion
	}
	return s.repo.ResubmitTransition(ctx, id, model.StatusPendingRevision, next, byUserEmail)
}

// Close moves PENDING_COMPLIANCE_CLOSURE → CLOSED.
func (s *riskService) Close(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != model.StatusPendingComplianceClosure {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be closed from status: %s", status)}
	}
	return s.repo.TransitionStatus(ctx, id, model.StatusPendingComplianceClosure, model.StatusClosed, byUserEmail)
}

// Cancel soft-deletes a risk by setting it to CANCELLED. Only valid from PENDING_RISK_OWNER_APPROVAL.
func (s *riskService) Cancel(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != model.StatusPendingOwnerApproval {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be cancelled from status: %s", status)}
	}
	return s.repo.TransitionStatus(ctx, id, model.StatusPendingOwnerApproval, model.StatusCancelled, byUserEmail)
}

func stringVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
