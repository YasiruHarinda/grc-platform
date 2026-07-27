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

type populationService struct {
	repo repository.PopulationRepository
}

// NewPopulationService constructs a PopulationService.
func NewPopulationService(repo repository.PopulationRepository) PopulationService {
	return &populationService{repo: repo}
}

// validPopulationStatuses mirrors the audit_population.status ENUM in audit_schema.sql.
var validPopulationStatuses = map[string]bool{
	"PENDING":             true,
	"SUBMITTED":           true,
	"COMPLIANCE_APPROVED": true,
	"COMPLIANCE_REJECTED": true,
	"APPROVED":            true,
	"AUDITOR_REJECTED":    true,
}

// allowedPopulationTransitions defines legal next statuses for an audit_population
// record, mirroring the review workflow: submit → compliance review → auditor
// validation, with rejections looping back to SUBMITTED for resubmission.
var allowedPopulationTransitions = map[string][]string{
	"PENDING":             {"SUBMITTED"},
	"SUBMITTED":           {"COMPLIANCE_APPROVED", "COMPLIANCE_REJECTED"},
	"COMPLIANCE_REJECTED": {"SUBMITTED"},
	"COMPLIANCE_APPROVED": {"APPROVED", "AUDITOR_REJECTED"},
	"AUDITOR_REJECTED":    {"SUBMITTED"},
	"APPROVED":            {},
}

// isValidPopulationTransition reports whether moving from → to is a legal step.
// A no-op (from == to) and an empty current status are always allowed.
func isValidPopulationTransition(from, to string) bool {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to || from == "" {
		return true
	}
	for _, next := range allowedPopulationTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// validPopulationFileKinds mirrors the audit_evidence_file.file_kind ENUM.
var validPopulationFileKinds = map[string]bool{"POPULATION": true, "SAMPLE": true}

func (s *populationService) CreatePopulation(ctx context.Context, auditID, controlID int, req domain.CreatePopulationRequest) (domain.AuditPopulation, error) {
	if auditID <= 0 {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	if req.DueDate == nil || *req.DueDate == "" {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "dueDate is required"}
	}
	if req.CreatedBy == "" {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	p, err := s.repo.CreatePopulation(ctx, auditID, controlID, req)
	if err != nil {
		return domain.AuditPopulation{}, err
	}
	return *p, nil
}

func (s *populationService) GetPopulationByID(ctx context.Context, populationID int) (domain.AuditPopulation, error) {
	if populationID <= 0 {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "populationId must be a positive integer"}
	}
	p, err := s.repo.GetPopulationByID(ctx, populationID)
	if err != nil {
		return domain.AuditPopulation{}, err
	}
	return *p, nil
}

func (s *populationService) ListPopulations(ctx context.Context, auditID, controlID int) ([]domain.AuditPopulation, error) {
	if auditID <= 0 {
		return nil, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return nil, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	pops, err := s.repo.ListPopulations(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if pops == nil {
		pops = []domain.AuditPopulation{}
	}
	return pops, nil
}

func (s *populationService) UpdatePopulation(ctx context.Context, populationID int, req domain.UpdatePopulationRequest) (domain.AuditPopulation, error) {
	if populationID <= 0 {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "populationId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		if !validPopulationStatuses[up] {
			return domain.AuditPopulation{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
		}
		req.Status = &up
		current, err := s.repo.GetPopulationByID(ctx, populationID)
		if err != nil {
			return domain.AuditPopulation{}, err
		}
		if !isValidPopulationTransition(current.Status, up) {
			return domain.AuditPopulation{}, &apierror.ValidationError{
				Msg: fmt.Sprintf("invalid status transition: %s -> %s", current.Status, up),
			}
		}
		req.ExpectedStatus = current.Status
	}
	p, err := s.repo.UpdatePopulation(ctx, populationID, req)
	if err != nil {
		return domain.AuditPopulation{}, err
	}
	return *p, nil
}

func (s *populationService) AddPopulationFile(ctx context.Context, populationID int, req domain.CreatePopulationFileRequest) (domain.AuditEvidenceFile, error) {
	if populationID <= 0 {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "populationId must be a positive integer"}
	}
	if req.FileName == "" {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "fileName is required"}
	}
	if req.FilePath == "" {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "filePath is required"}
	}
	if !validPopulationFileKinds[strings.ToUpper(req.FileKind)] {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "fileKind must be POPULATION or SAMPLE"}
	}
	if req.CreatedBy == "" {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	f, err := s.repo.AddPopulationFile(ctx, populationID, req)
	if err != nil {
		return domain.AuditEvidenceFile{}, err
	}
	return *f, nil
}

func (s *populationService) ListPopulationFiles(ctx context.Context, populationID int) ([]domain.AuditEvidenceFile, error) {
	if populationID <= 0 {
		return nil, &apierror.ValidationError{Msg: "populationId must be a positive integer"}
	}
	files, err := s.repo.ListPopulationFiles(ctx, populationID)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []domain.AuditEvidenceFile{}
	}
	return files, nil
}

func (s *populationService) GetPopulationFileByID(ctx context.Context, fileID int) (domain.AuditEvidenceFile, error) {
	if fileID <= 0 {
		return domain.AuditEvidenceFile{}, &apierror.ValidationError{Msg: "fileId must be a positive integer"}
	}
	f, err := s.repo.GetPopulationFileByID(ctx, fileID)
	if err != nil {
		return domain.AuditEvidenceFile{}, err
	}
	return *f, nil
}

func (s *populationService) DeletePopulationFile(ctx context.Context, fileID int) error {
	if fileID <= 0 {
		return &apierror.ValidationError{Msg: "fileId must be a positive integer"}
	}
	return s.repo.DeletePopulationFile(ctx, fileID)
}
