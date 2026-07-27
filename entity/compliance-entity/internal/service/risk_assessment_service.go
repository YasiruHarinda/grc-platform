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

type riskAssessmentService struct {
	repo repository.RiskAssessmentRepository
}

// NewRiskAssessmentService constructs a RiskAssessmentService.
func NewRiskAssessmentService(repo repository.RiskAssessmentRepository) RiskAssessmentService {
	return &riskAssessmentService{repo: repo}
}

func (s *riskAssessmentService) CreateRiskAssessment(ctx context.Context, riskID int, req domain.CreateRiskAssessmentRequest) (domain.RiskAssessment, error) {
	if riskID <= 0 {
		return domain.RiskAssessment{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.Likelihood < 1 || req.Likelihood > 3 {
		return domain.RiskAssessment{}, &apierror.ValidationError{Msg: "likelihood must be 1, 2, or 3"}
	}
	if req.Impact < 1 || req.Impact > 3 {
		return domain.RiskAssessment{}, &apierror.ValidationError{Msg: "impact must be 1, 2, or 3"}
	}
	if req.Progress == "" {
		return domain.RiskAssessment{}, &apierror.ValidationError{Msg: "progress is required"}
	}
	if req.AssessedBy == "" {
		return domain.RiskAssessment{}, &apierror.ValidationError{Msg: "assessedBy is required"}
	}
	if req.CreatedBy == "" {
		return domain.RiskAssessment{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	a, err := s.repo.CreateRiskAssessment(ctx, riskID, req)
	if err != nil {
		return domain.RiskAssessment{}, err
	}
	return *a, nil
}

func (s *riskAssessmentService) ListRiskAssessments(ctx context.Context, riskID int) (domain.ListRiskAssessmentsResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskAssessmentsResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	resp, err := s.repo.ListRiskAssessments(ctx, riskID)
	if err != nil {
		return domain.ListRiskAssessmentsResponse{}, err
	}
	if resp.Assessments == nil {
		resp.Assessments = []domain.RiskAssessment{}
	}
	return *resp, nil
}
