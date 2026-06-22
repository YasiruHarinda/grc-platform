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

	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/repository"
)

// ControlService defines business operations for audit controls.
type ControlService interface {
	List(ctx context.Context, auditID int) ([]*model.AuditControl, error)
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
	Add(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error)
	Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error
}

type controlService struct {
	repo repository.ControlRepository
}

func NewControlService(repo repository.ControlRepository) ControlService {
	return &controlService{repo: repo}
}

func (s *controlService) List(ctx context.Context, auditID int) ([]*model.AuditControl, error) {
	// TODO: delegate to repo
	return nil, nil
}

func (s *controlService) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	// TODO: delegate to repo; verify control belongs to auditID
	return nil, nil
}

func (s *controlService) Add(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error) {
	// TODO: validate control_number uniqueness within audit, delegate to repo
	return nil, nil
}

func (s *controlService) Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error {
	// TODO: verify control belongs to auditID, delegate to repo
	return nil
}
