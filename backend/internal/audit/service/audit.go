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

// Package service implements the business logic for the Audit Hub module.
// Handlers call service methods; services call repository methods.
// Workflow transitions, validation rules, and trail writes live here.
package service

import (
	"context"

	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/repository"
)

// AuditService defines business operations for audit lifecycle management.
type AuditService interface {
	List(ctx context.Context) ([]*model.Audit, error)
	GetByID(ctx context.Context, id int) (*model.Audit, error)
	Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error)
	Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error

	// Workflow transitions.
	MoveToFieldwork(ctx context.Context, id int, byUserID string) error
	SubmitForReview(ctx context.Context, id int, byUserID string) error
	Complete(ctx context.Context, id int, byUserID string) error
}

type auditService struct {
	repo repository.AuditRepository
}

func NewAuditService(repo repository.AuditRepository) AuditService {
	return &auditService{repo: repo}
}

func (s *auditService) List(ctx context.Context) ([]*model.Audit, error) {
	// TODO: delegate to repo
	return nil, nil
}

func (s *auditService) GetByID(ctx context.Context, id int) (*model.Audit, error) {
	// TODO: delegate to repo; wrap missing row as apierror.Error{StatusCode: 404}
	return nil, nil
}

func (s *auditService) Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error) {
	// TODO: validate framework + product exist, delegate to repo, write trail entry
	return nil, nil
}

func (s *auditService) Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error {
	// TODO: fetch audit, validate editable in current status, delegate to repo
	return nil
}

func (s *auditService) MoveToFieldwork(ctx context.Context, id int, byUserID string) error {
	// TODO: guard status == ACTIVE, set FIELDWORK (or equivalent), write trail entry
	return nil
}

func (s *auditService) SubmitForReview(ctx context.Context, id int, byUserID string) error {
	// TODO: guard all controls are in a reviewable state, write trail entry
	return nil
}

func (s *auditService) Complete(ctx context.Context, id int, byUserID string) error {
	// TODO: guard status, set COMPLETED, write trail entry
	return nil
}
