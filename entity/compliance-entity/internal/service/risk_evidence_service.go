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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskEvidenceService struct {
	repo           repository.RiskEvidenceRepository
	actionPlanRepo repository.RiskActionPlanRepository
}

// NewRiskEvidenceService constructs a RiskEvidenceService.
func NewRiskEvidenceService(repo repository.RiskEvidenceRepository, actionPlanRepo repository.RiskActionPlanRepository) RiskEvidenceService {
	return &riskEvidenceService{repo: repo, actionPlanRepo: actionPlanRepo}
}

// validRiskEvidenceTypes mirrors the risk_evidence.evidence_type ENUM in risk_schema.sql.
var validRiskEvidenceTypes = map[string]bool{
	"ACTION_PLAN_ATTACHMENT":    true,
	"FINAL_APPROVAL_ATTACHMENT": true,
}

func (s *riskEvidenceService) CreateRiskEvidence(ctx context.Context, riskID int, req domain.CreateRiskEvidenceRequest) (domain.RiskEvidenceFile, error) {
	if riskID <= 0 {
		return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.FileName == "" {
		return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "fileName is required"}
	}
	if req.FilePath == "" {
		return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "filePath is required"}
	}
	req.EvidenceType = strings.ToUpper(req.EvidenceType)
	if !validRiskEvidenceTypes[req.EvidenceType] {
		return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "evidenceType must be ACTION_PLAN_ATTACHMENT or FINAL_APPROVAL_ATTACHMENT"}
	}
	// FINAL_APPROVAL_ATTACHMENT ("Risk Action Plan Completion Attachment") is
	// always evidence for one specific plan — a risk can have more than one
	// STANDARD action plan, so without this link there'd be no way to tell
	// which plan a completion file supports, or to gate that plan's own
	// completion on its own evidence. ACTION_PLAN_ATTACHMENT ("Risk Evidence
	// Attachment") is risk-level, uploaded before any plan necessarily exists,
	// so it stays unlinked.
	if req.EvidenceType == "FINAL_APPROVAL_ATTACHMENT" {
		if req.ActionPlanID == nil || *req.ActionPlanID <= 0 {
			return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "actionPlanId is required for FINAL_APPROVAL_ATTACHMENT"}
		}
		// The caller (normally the GRC Backend) is expected to have already
		// checked this, but this service has no auth of its own — see
		// routes.go: "Authentication is handled upstream by the Choreo API
		// Gateway" — so it can't assume every caller re-implements that
		// check correctly. Re-validating here keeps a cross-risk
		// (riskId, actionPlanId) pair from ever reaching the DB, rather than
		// only guarding against it at read time (e.g. HasCompletionEvidence).
		plan, err := s.actionPlanRepo.GetRiskActionPlanByID(ctx, *req.ActionPlanID)
		if err != nil {
			return domain.RiskEvidenceFile{}, err
		}
		if plan.RiskID != riskID {
			return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "actionPlanId does not belong to this risk"}
		}
	} else {
		req.ActionPlanID = nil
	}
	if req.CreatedBy == "" {
		return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	f, err := s.repo.CreateRiskEvidence(ctx, riskID, req)
	if err != nil {
		return domain.RiskEvidenceFile{}, err
	}
	return *f, nil
}

func (s *riskEvidenceService) GetRiskEvidenceByID(ctx context.Context, fileID int) (domain.RiskEvidenceFile, error) {
	if fileID <= 0 {
		return domain.RiskEvidenceFile{}, &apierror.ValidationError{Msg: "fileId must be a positive integer"}
	}
	f, err := s.repo.GetRiskEvidenceByID(ctx, fileID)
	if err != nil {
		return domain.RiskEvidenceFile{}, err
	}
	return *f, nil
}

func (s *riskEvidenceService) ListRiskEvidence(ctx context.Context, riskID int) (domain.ListRiskEvidenceResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskEvidenceResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	resp, err := s.repo.ListRiskEvidence(ctx, riskID)
	if err != nil {
		return domain.ListRiskEvidenceResponse{}, err
	}
	if resp.Evidence == nil {
		resp.Evidence = []domain.RiskEvidenceFile{}
	}
	return *resp, nil
}

func (s *riskEvidenceService) DeleteRiskEvidence(ctx context.Context, riskID, fileID int) error {
	if riskID <= 0 {
		return &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if fileID <= 0 {
		return &apierror.ValidationError{Msg: "fileId must be a positive integer"}
	}
	return s.repo.DeleteRiskEvidence(ctx, riskID, fileID)
}
