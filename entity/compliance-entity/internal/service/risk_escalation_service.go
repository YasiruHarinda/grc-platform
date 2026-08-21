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
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskEscalationService struct {
	repo    repository.RiskEscalationRepository
	riskSvc RiskService
}

// NewRiskEscalationService constructs a RiskEscalationService. riskSvc backs
// EscalateRisk's IN_REMEDIATION+overdue check and the workflow_status flip.
func NewRiskEscalationService(repo repository.RiskEscalationRepository, riskSvc RiskService) RiskEscalationService {
	return &riskEscalationService{repo: repo, riskSvc: riskSvc}
}

var validEscalationStatuses = map[string]bool{"OPEN": true, "RESOLVED": true}

func (s *riskEscalationService) CreateRiskEscalation(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.NewTreatmentStrategy != nil {
		up := strings.ToUpper(*req.NewTreatmentStrategy)
		if !validTreatmentStrategies[up] {
			return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "newTreatmentStrategy must be REMEDIATE, ACCEPT, TRANSFER, or VOID"}
		}
		req.NewTreatmentStrategy = &up
	}
	if req.CreatedBy == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	e, err := s.repo.CreateRiskEscalation(ctx, riskID, req)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) GetRiskEscalationByID(ctx context.Context, riskID, escalationID int) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if escalationID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "escalationId must be a positive integer"}
	}
	e, err := s.repo.GetRiskEscalationByID(ctx, riskID, escalationID)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) UpdateRiskEscalation(ctx context.Context, riskID, escalationID int, req domain.UpdateRiskEscalationRequest) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if escalationID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "escalationId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		if !validEscalationStatuses[up] {
			return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
		}
		req.Status = &up
	}
	if req.NewTreatmentStrategy != nil {
		up := strings.ToUpper(*req.NewTreatmentStrategy)
		if !validTreatmentStrategies[up] {
			return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "newTreatmentStrategy must be REMEDIATE, ACCEPT, TRANSFER, or VOID"}
		}
		req.NewTreatmentStrategy = &up
	}
	e, err := s.repo.UpdateRiskEscalation(ctx, riskID, escalationID, req)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) CommentEscalation(ctx context.Context, riskID, escalationID int, req domain.CommentEscalationRequest) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if escalationID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "escalationId must be a positive integer"}
	}
	if strings.TrimSpace(req.Decision) == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "decision is required"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	e, err := s.repo.CommentEscalation(ctx, riskID, escalationID, strings.TrimSpace(req.Decision), req.UpdatedBy)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) ListRiskEscalations(ctx context.Context, riskID int) (domain.ListRiskEscalationsResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskEscalationsResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	escalations, err := s.repo.ListRiskEscalations(ctx, riskID)
	if err != nil {
		return domain.ListRiskEscalationsResponse{}, err
	}
	if escalations == nil {
		escalations = []domain.RiskEscalation{}
	}
	return domain.ListRiskEscalationsResponse{Escalations: escalations}, nil
}

func (s *riskEscalationService) EscalateRisk(ctx context.Context, riskID int, req domain.EscalateRiskRequest) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.CreatedBy == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}

	risk, err := s.riskSvc.GetRiskByID(ctx, riskID)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	if risk.WorkflowStatus != "IN_REMEDIATION" {
		return domain.RiskEscalation{}, &apierror.ValidationError{
			Msg: "risk must be IN_REMEDIATION to escalate, currently: " + risk.WorkflowStatus,
		}
	}
	if risk.ImplementationDate == nil || !isPastDate(*risk.ImplementationDate) {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "risk is not overdue"}
	}

	// Flip the status and insert the escalation atomically: a failed insert
	// rolls the flip back, so the risk can't be left ESCALATED with no
	// escalation row (an unrecoverable stuck state). The CAS inside also makes
	// a concurrent escalate (daily job vs. manual click) fail rather than
	// create a duplicate escalation.
	e, err := s.repo.Escalate(ctx, riskID, domain.CreateRiskEscalationRequest{
		CreatedBy:           req.CreatedBy,
		AssignerLeadUUID:    req.AssignerLeadUUID,
		ActionOwnerLeadUUID: req.ActionOwnerLeadUUID,
	})
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

// isPastDate reports whether a YYYY-MM-DD date string is strictly before
// today — lexicographic comparison is valid for this format, and matches the
// daily job's own `implementation_date < CURDATE()` SQL predicate exactly.
func isPastDate(yyyymmdd string) bool {
	return yyyymmdd < time.Now().Format("2006-01-02")
}
