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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// EscalationService defines business operations for risk escalations.
type EscalationService interface {
	List(ctx context.Context, riskID int) ([]*model.Escalation, error)
	Escalate(ctx context.Context, riskID int, createdBy string) (*model.Escalation, error)
}

type escalationService struct {
	repo repository.EscalationRepository
}

func NewEscalationService(repo repository.EscalationRepository) EscalationService {
	return &escalationService{repo: repo}
}

func (s *escalationService) List(ctx context.Context, riskID int) ([]*model.Escalation, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	return s.repo.List(ctx, riskID)
}

func (s *escalationService) Escalate(ctx context.Context, riskID int, createdBy string) (*model.Escalation, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	if createdBy == "" {
		return nil, badRequest("createdBy is required")
	}
	return s.repo.Escalate(ctx, riskID, createdBy)
}
