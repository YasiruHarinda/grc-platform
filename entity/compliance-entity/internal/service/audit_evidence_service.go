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
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type evidenceService struct{ repo repository.EvidenceRepository }

// NewEvidenceService constructs an EvidenceService.
func NewEvidenceService(repo repository.EvidenceRepository) EvidenceService {
	return &evidenceService{repo: repo}
}

// validEvidenceStatuses mirrors the audit_evidence.status ENUM in audit_schema.sql.
var validEvidenceStatuses = map[string]bool{
	"SUBMITTED":           true,
	"COMPLIANCE_APPROVED": true,
	"COMPLIANCE_REJECTED": true,
	"APPROVED":            true,
	"AUDITOR_REJECTED":    true,
}

// allowedEvidenceTransitions defines the legal next statuses for an evidence
// record: submit → compliance review → auditor validation, with rejections
// looping back to a resubmit. Prevents skipping straight to APPROVED.
var allowedEvidenceTransitions = map[string][]string{
	"SUBMITTED":           {"COMPLIANCE_APPROVED", "COMPLIANCE_REJECTED"},
	"COMPLIANCE_REJECTED": {"SUBMITTED"},
	"COMPLIANCE_APPROVED": {"APPROVED", "AUDITOR_REJECTED"},
	"AUDITOR_REJECTED":    {"SUBMITTED"},
	"APPROVED":            {},
}

// isValidEvidenceTransition reports whether moving from -> to is a legal step.
// A no-op (from == to) and an empty current status are always allowed.
func isValidEvidenceTransition(from, to string) bool {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to || from == "" {
		return true
	}
	for _, next := range allowedEvidenceTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

func (s *evidenceService) CreateEvidence(ctx context.Context, controlID int, req domain.CreateEvidenceRequest) (domain.AuditEvidence, error) {
	if controlID <= 0 {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	if req.CreatedBy == "" {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	e, err := s.repo.CreateEvidence(ctx, controlID, req)
	if err != nil {
		return domain.AuditEvidence{}, err
	}
	return *e, nil
}

func (s *evidenceService) GetEvidenceByID(ctx context.Context, evidenceID int) (domain.AuditEvidence, error) {
	if evidenceID <= 0 {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	e, err := s.repo.GetEvidenceByID(ctx, evidenceID)
	if err != nil {
		return domain.AuditEvidence{}, err
	}
	return *e, nil
}

func (s *evidenceService) ListEvidenceByControl(ctx context.Context, auditID, controlID int) (domain.ListEvidenceResponse, error) {
	if auditID <= 0 {
		return domain.ListEvidenceResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.ListEvidenceResponse{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	evidence, err := s.repo.ListEvidenceByControl(ctx, auditID, controlID)
	if err != nil {
		return domain.ListEvidenceResponse{}, err
	}
	if evidence == nil {
		evidence = []domain.AuditEvidence{}
	}
	return domain.ListEvidenceResponse{Evidence: evidence}, nil
}

func (s *evidenceService) UpdateEvidence(ctx context.Context, evidenceID int, req domain.UpdateEvidenceRequest) (domain.AuditEvidence, error) {
	if evidenceID <= 0 {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	if req.Status == "" {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "status is required"}
	}
	if !validEvidenceStatuses[strings.ToUpper(req.Status)] {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "invalid status: " + req.Status}
	}
	if req.UpdatedBy == "" {
		return domain.AuditEvidence{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	// Enforce workflow order: the target status must be reachable from the
	// evidence record's current status.
	current, err := s.repo.GetEvidenceByID(ctx, evidenceID)
	if err != nil {
		return domain.AuditEvidence{}, err
	}
	if !isValidEvidenceTransition(current.Status, req.Status) {
		return domain.AuditEvidence{}, &apierror.ValidationError{
			Msg: fmt.Sprintf("invalid status transition: %s -> %s", current.Status, req.Status),
		}
	}
	// Pass current status to the repo so the UPDATE enforces it atomically,
	// preventing TOCTOU races between the read above and the write below.
	req.ExpectedStatus = current.Status
	e, err := s.repo.UpdateEvidence(ctx, evidenceID, req)
	if err != nil {
		return domain.AuditEvidence{}, err
	}
	return *e, nil
}

func (s *evidenceService) AddEvidenceFile(ctx context.Context, evidenceID int, req domain.CreateEvidenceFileRequest) (domain.AuditEvidenceFile, error) {
	if evidenceID <= 0 {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	if req.FileName == "" {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "fileName is required"}
	}
	if req.FilePath == "" {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "filePath is required"}
	}
	if req.CreatedBy == "" {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	f, err := s.repo.AddEvidenceFile(ctx, evidenceID, req)
	if err != nil {
		return domain.AuditEvidenceFile{}, err
	}
	return *f, nil
}

func (s *evidenceService) ListEvidenceFiles(ctx context.Context, evidenceID int) (domain.ListEvidenceFilesResponse, error) {
	if evidenceID <= 0 {
		return domain.ListEvidenceFilesResponse{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	resp, err := s.repo.ListEvidenceFiles(ctx, evidenceID)
	if err != nil {
		return domain.ListEvidenceFilesResponse{}, err
	}
	if resp.Files == nil {
		resp.Files = []domain.AuditEvidenceFile{}
	}
	return *resp, nil
}

func (s *evidenceService) GetEvidenceFileByID(ctx context.Context, fileID int) (domain.AuditEvidenceFile, error) {
	if fileID <= 0 {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "fileId must be a positive integer"}
	}
	f, err := s.repo.GetEvidenceFileByID(ctx, fileID)
	if err != nil {
		return domain.AuditEvidenceFile{}, err
	}
	return *f, nil
}

func (s *evidenceService) DeleteEvidenceFile(ctx context.Context, fileID int) error {
	if fileID <= 0 {
		return &apierror.ValidationError{Msg: "fileId must be a positive integer"}
	}
	return s.repo.DeleteEvidenceFile(ctx, fileID)
}

func (s *evidenceService) DeleteEvidence(ctx context.Context, evidenceID int) error {
	if evidenceID <= 0 {
		return &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	return s.repo.DeleteEvidence(ctx, evidenceID)
}
