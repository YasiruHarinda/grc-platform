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

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

// RiskService defines the business operations for risk lifecycle management.
type RiskService interface {
	List(ctx context.Context, filter model.ListRisksFilter) ([]*model.Risk, error)
	GetByID(ctx context.Context, id int) (*model.Risk, error)
	Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error)
	NextSequenceID(ctx context.Context, sourceRegisterID, year int, quarter string) (int, error)
	Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error

	// Workflow transitions — each validates the current status before advancing.
	Submit(ctx context.Context, id int, byUserID string) error
	Approve(ctx context.Context, id int, byUserID string) error
	Reject(ctx context.Context, id int, req model.RejectRiskRequest, byUserID string) error
	Complete(ctx context.Context, id int, byUserID string) error
	OwnerApprove(ctx context.Context, id int, byUserID string) error
	Close(ctx context.Context, id int, byUserID string) error
	Escalate(ctx context.Context, id int, req model.EscalateRiskRequest, byUserID string) error
	Assess(ctx context.Context, id int, req model.AssessRiskRequest, byUserID string) error
}

type riskService struct {
	repo repository.RiskRepository
}

// NewRiskService creates a RiskService backed by the given repository.
func NewRiskService(repo repository.RiskRepository) RiskService {
	return &riskService{repo: repo}
}

func (s *riskService) List(ctx context.Context, filter model.ListRisksFilter) ([]*model.Risk, error) {
	// TODO: delegate to repo.List
	return nil, nil
}

func (s *riskService) GetByID(ctx context.Context, id int) (*model.Risk, error) {
	// TODO: delegate to repo.GetByID; wrap missing row as apierror.Error{StatusCode: 404}
	return nil, nil
}

func (s *riskService) Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error) {
	return s.repo.Create(ctx, req, createdBy)
}

func (s *riskService) NextSequenceID(ctx context.Context, sourceRegisterID, year int, quarter string) (int, error) {
	return s.repo.NextSequenceID(ctx, sourceRegisterID, year, quarter)
}

func (s *riskService) Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error {
	// TODO: fetch risk, check status allows edits, call repo.Update, write changelog row
	return nil
}

func (s *riskService) Submit(ctx context.Context, id int, byUserID string) error {
	// TODO: fetch risk, guard status == DRAFT, set PENDING_COMPLIANCE_REVIEW, write changelog
	return nil
}

func (s *riskService) Approve(ctx context.Context, id int, byUserID string) error {
	// TODO: guard status == PENDING_COMPLIANCE_REVIEW, set IN_REMEDIATION,
	//       record compliance_approval_by + compliance_approval_date, write changelog
	return nil
}

func (s *riskService) Reject(ctx context.Context, id int, req model.RejectRiskRequest, byUserID string) error {
	// TODO: guard status == PENDING_COMPLIANCE_REVIEW, set DRAFT,
	//       store rejection_comment, write changelog
	return nil
}

func (s *riskService) Complete(ctx context.Context, id int, byUserID string) error {
	// TODO: guard status == IN_REMEDIATION, set PENDING_RISK_OWNER_APPROVAL, write changelog
	return nil
}

func (s *riskService) OwnerApprove(ctx context.Context, id int, byUserID string) error {
	// TODO: guard status == PENDING_RISK_OWNER_APPROVAL, set PENDING_COMPLIANCE_CLOSURE, write changelog
	return nil
}

func (s *riskService) Close(ctx context.Context, id int, byUserID string) error {
	// TODO: guard status == PENDING_COMPLIANCE_CLOSURE, set CLOSED, write changelog
	return nil
}

func (s *riskService) Escalate(ctx context.Context, id int, req model.EscalateRiskRequest, byUserID string) error {
	// TODO: guard status allows escalation, set ESCALATED,
	//       insert risk_escalation row, send notification to management user
	return nil
}

func (s *riskService) Assess(ctx context.Context, id int, req model.AssessRiskRequest, byUserID string) error {
	// TODO: fetch escalation for risk, record management decision + new_treatment_strategy
	return nil
}
