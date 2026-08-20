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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskReferenceService struct {
	repo repository.RiskReferenceRepository
}

// NewRiskReferenceService constructs a RiskReferenceService.
func NewRiskReferenceService(repo repository.RiskReferenceRepository) RiskReferenceService {
	return &riskReferenceService{repo: repo}
}

func (s *riskReferenceService) SearchRiskReferences(ctx context.Context, req domain.SearchRiskReferencesRequest) (domain.SearchRiskReferencesResponse, error) {
	normalizePagination(&req.Pagination)
	refs, total, err := s.repo.SearchRiskReferences(ctx, req)
	if err != nil {
		return domain.SearchRiskReferencesResponse{}, err
	}
	if refs == nil {
		refs = []domain.RiskComplianceReference{}
	}
	return domain.SearchRiskReferencesResponse{References: refs, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *riskReferenceService) GetRiskReferenceByID(ctx context.Context, id int) (domain.RiskComplianceReference, error) {
	if id <= 0 {
		return domain.RiskComplianceReference{}, &apierror.ValidationError{Msg: "reference id must be a positive integer"}
	}
	r, err := s.repo.GetRiskReferenceByID(ctx, id)
	if err != nil {
		return domain.RiskComplianceReference{}, err
	}
	return *r, nil
}

func (s *riskReferenceService) CreateRiskReference(ctx context.Context, req domain.CreateRiskReferenceRequest) (domain.RiskComplianceReference, error) {
	if req.Name == "" {
		return domain.RiskComplianceReference{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if req.CreatedBy == "" {
		return domain.RiskComplianceReference{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	r, err := s.repo.CreateRiskReference(ctx, req)
	if err != nil {
		return domain.RiskComplianceReference{}, err
	}
	return *r, nil
}

func (s *riskReferenceService) UpdateRiskReference(ctx context.Context, id int, req domain.UpdateRiskReferenceRequest) (domain.RiskComplianceReference, error) {
	if id <= 0 {
		return domain.RiskComplianceReference{}, &apierror.ValidationError{Msg: "reference id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskComplianceReference{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	r, err := s.repo.UpdateRiskReference(ctx, id, req)
	if err != nil {
		return domain.RiskComplianceReference{}, err
	}
	return *r, nil
}

func (s *riskReferenceService) DeleteRiskReference(ctx context.Context, id int) error {
	if id <= 0 {
		return &apierror.ValidationError{Msg: "reference id must be a positive integer"}
	}
	return s.repo.DeleteRiskReference(ctx, id)
}
