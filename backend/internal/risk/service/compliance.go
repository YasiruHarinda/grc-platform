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

// ComplianceReferenceService defines business operations for compliance references.
type ComplianceReferenceService interface {
	List(ctx context.Context) ([]*model.ComplianceReference, error)
	Create(ctx context.Context, req model.CreateComplianceRefRequest, createdBy string) (*model.ComplianceReference, error)
}

type complianceReferenceService struct {
	repo repository.ComplianceReferenceRepository
}

func NewComplianceReferenceService(repo repository.ComplianceReferenceRepository) ComplianceReferenceService {
	return &complianceReferenceService{repo: repo}
}

func (s *complianceReferenceService) List(ctx context.Context) ([]*model.ComplianceReference, error) {
	return s.repo.List(ctx)
}

func (s *complianceReferenceService) Create(ctx context.Context, req model.CreateComplianceRefRequest, createdBy string) (*model.ComplianceReference, error) {
	// TODO: delegate to repo
	return nil, nil
}
