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
	"net/http"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// RiskAssessmentService defines business operations for residual risk assessments.
type RiskAssessmentService interface {
	Create(ctx context.Context, riskID int, req model.CreateAssessmentRequest, assessedBy string) (*model.RiskAssessment, error)
	// ListByRiskID returns a risk's reassessments newest-first. Used by the
	// history writer to render a reassessment as "HIGH → MEDIUM" rather than
	// just its new level; the repository has always exposed it.
	ListByRiskID(ctx context.Context, riskID int) ([]model.RiskAssessment, error)
}

type riskAssessmentService struct {
	repo repository.RiskAssessmentRepository
}

// NewRiskAssessmentService creates a RiskAssessmentService backed by the given repository.
func NewRiskAssessmentService(repo repository.RiskAssessmentRepository) RiskAssessmentService {
	return &riskAssessmentService{repo: repo}
}

func (s *riskAssessmentService) Create(ctx context.Context, riskID int, req model.CreateAssessmentRequest, assessedBy string) (*model.RiskAssessment, error) {
	if req.Likelihood < 1 || req.Likelihood > 3 {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "likelihood must be 1, 2, or 3"}
	}
	if req.Impact < 1 || req.Impact > 3 {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "impact must be 1, 2, or 3"}
	}
	if req.Progress == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "progress is required"}
	}
	if req.ReassessmentDate == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "reassessment_date is required"}
	}
	if _, err := time.Parse("2006-01-02", req.ReassessmentDate); err != nil {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "reassessment_date must be in YYYY-MM-DD format"}
	}
	return s.repo.Create(ctx, riskID, req, assessedBy)
}

func (s *riskAssessmentService) ListByRiskID(ctx context.Context, riskID int) ([]model.RiskAssessment, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	return s.repo.ListByRiskID(ctx, riskID)
}
