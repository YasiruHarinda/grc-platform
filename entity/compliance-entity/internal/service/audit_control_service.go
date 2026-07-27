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
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type controlService struct{ repo repository.ControlRepository }

// NewControlService constructs a ControlService.
func NewControlService(repo repository.ControlRepository) ControlService {
	return &controlService{repo: repo}
}

// validControlStatuses mirrors the audit_control.status ENUM in audit_schema.sql
// exactly (12 statuses). Keep in sync with the schema — any drift causes valid
// filters to 400 or invalid ones to reach the DB.
var validControlStatuses = map[string]bool{
	// OE — population phase
	"POPULATION_PENDING":            true,
	"POPULATION_INTERNAL_REVIEW":    true,
	"POPULATION_UNDER_VALIDATION":   true,
	"POPULATION_NEED_CLARIFICATION": true,
	"POPULATION_COMPLETE":           true,
	// OE — sample phase (between population approval and evidence)
	"AWAITING_SAMPLE":  true,
	"SUBMITTED_SAMPLE": true,
	// Evidence phase (Design default; OE after sample)
	"EVIDENCE_PENDING":            true,
	"EVIDENCE_INTERNAL_REVIEW":    true,
	"EVIDENCE_UNDER_VALIDATION":   true,
	"EVIDENCE_NEED_CLARIFICATION": true,
	// Terminal
	"COMPLETE": true,
}

var validRequirementTypes = map[string]bool{"DESIGN": true, "OE": true}

// validControlTypes / validScopes mirror the audit_control.control_type and
// audit_control.scope ENUMs in audit_schema.sql.
var validControlTypes = map[string]bool{"CONFIG": true, "NON_CONFIG": true}
var validScopes = map[string]bool{"COMMON": true, "PRODUCT_SPECIFIC": true}

// allowedControlTransitions defines the legal next statuses for each audit_control
// status, mirroring the audit (Design) and OE (population→sample→evidence) workflow.
// A status update whose target is not in the current status's allowed set is
// rejected, so the workflow order cannot be skipped (e.g. EVIDENCE_PENDING ->
// COMPLETE bypassing internal review and auditor validation).
var allowedControlTransitions = map[string][]string{
	// OE — population phase
	"POPULATION_PENDING":            {"POPULATION_INTERNAL_REVIEW"},
	"POPULATION_INTERNAL_REVIEW":    {"POPULATION_UNDER_VALIDATION", "POPULATION_PENDING"},
	"POPULATION_UNDER_VALIDATION":   {"POPULATION_COMPLETE", "POPULATION_NEED_CLARIFICATION"},
	"POPULATION_NEED_CLARIFICATION": {"POPULATION_INTERNAL_REVIEW"},
	// OE — sample handoff (auditor submits the sample or asks for more time)
	"POPULATION_COMPLETE": {"AWAITING_SAMPLE", "SUBMITTED_SAMPLE", "EVIDENCE_PENDING"},
	"AWAITING_SAMPLE":     {"SUBMITTED_SAMPLE", "EVIDENCE_PENDING"},
	"SUBMITTED_SAMPLE":    {"EVIDENCE_PENDING", "EVIDENCE_INTERNAL_REVIEW"},
	// Evidence phase (Design default; OE after sample)
	"EVIDENCE_PENDING":            {"EVIDENCE_INTERNAL_REVIEW"},
	"EVIDENCE_INTERNAL_REVIEW":    {"EVIDENCE_UNDER_VALIDATION", "EVIDENCE_PENDING"},
	"EVIDENCE_UNDER_VALIDATION":   {"COMPLETE", "EVIDENCE_NEED_CLARIFICATION"},
	"EVIDENCE_NEED_CLARIFICATION": {"EVIDENCE_INTERNAL_REVIEW"},
	// Terminal
	"COMPLETE": {},
}

// isValidControlTransition reports whether moving from -> to is a legal workflow
// step. A no-op (from == to) is always allowed. An empty current status (newly
// created / not yet set) is allowed to move to any valid status.
func isValidControlTransition(from, to string) bool {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to || from == "" {
		return true
	}
	for _, next := range allowedControlTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

func (s *controlService) SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) (domain.SearchControlsResponse, error) {
	if auditID <= 0 {
		return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	for _, sk := range req.StatusKeys {
		if !validControlStatuses[strings.ToUpper(sk)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: " + sk}
		}
	}
	for _, rt := range req.RequirementTypes {
		if !validRequirementTypes[strings.ToUpper(rt)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid requirementType: " + rt + " (must be DESIGN or OE)"}
		}
	}
	normalizePagination(&req.Pagination)
	controls, total, err := s.repo.SearchControls(ctx, auditID, req)
	if err != nil {
		return domain.SearchControlsResponse{}, err
	}
	if controls == nil {
		controls = []domain.AuditControl{}
	}
	return domain.SearchControlsResponse{Controls: controls, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *controlService) SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) (domain.SearchControlsResponse, error) {
	for _, sk := range req.StatusKeys {
		if !validControlStatuses[strings.ToUpper(sk)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: " + sk}
		}
	}
	for _, rt := range req.RequirementTypes {
		if !validRequirementTypes[strings.ToUpper(rt)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid requirementType: " + rt + " (must be DESIGN or OE)"}
		}
	}
	normalizePagination(&req.Pagination)
	controls, total, err := s.repo.SearchControlsGlobal(ctx, req)
	if err != nil {
		return domain.SearchControlsResponse{}, err
	}
	if controls == nil {
		controls = []domain.AuditControl{}
	}
	return domain.SearchControlsResponse{Controls: controls, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *controlService) BulkCreateControls(ctx context.Context, auditID int, req domain.BulkCreateControlsRequest) (domain.BulkCreateControlsResponse, error) {
	if auditID <= 0 {
		return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if len(req.Controls) == 0 {
		return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: "controls must not be empty"}
	}
	for i, c := range req.Controls {
		if c.ControlNumber == "" && c.FrameworkControlID == nil {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: controlNumber is required", i)}
		}
		if c.Description == "" && c.FrameworkControlID == nil {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: description is required", i)}
		}
		if !validRequirementTypes[strings.ToUpper(c.RequirementType)] {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: invalid requirementType %q", i, c.RequirementType)}
		}
		if !validControlTypes[strings.ToUpper(c.ControlType)] {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: invalid controlType %q (must be CONFIG or NON_CONFIG)", i, c.ControlType)}
		}
		if !validScopes[strings.ToUpper(c.Scope)] {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: invalid scope %q (must be COMMON or PRODUCT_SPECIFIC)", i, c.Scope)}
		}
		if c.CreatedBy == "" {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: createdBy is required", i)}
		}
		req.Controls[i].RequirementType = strings.ToUpper(c.RequirementType)
		req.Controls[i].ControlType = strings.ToUpper(c.ControlType)
		req.Controls[i].Scope = strings.ToUpper(c.Scope)
	}
	controls, err := s.repo.BulkCreateControls(ctx, auditID, req.Controls)
	if err != nil {
		return domain.BulkCreateControlsResponse{}, err
	}
	return domain.BulkCreateControlsResponse{Controls: controls, Created: len(controls)}, nil
}

func (s *controlService) DeleteControl(ctx context.Context, auditID, controlID int) error {
	if auditID <= 0 {
		return &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	return s.repo.DeleteControl(ctx, auditID, controlID)
}

func (s *controlService) GetControlByID(ctx context.Context, auditID, controlID int) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	c, err := s.repo.GetControlByID(ctx, auditID, controlID)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}

func (s *controlService) CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if req.ControlNumber == "" && req.FrameworkControlID == nil {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlNumber is required"}
	}
	if req.Description == "" && req.FrameworkControlID == nil {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "description is required"}
	}
	if !validRequirementTypes[strings.ToUpper(req.RequirementType)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "requirementType must be DESIGN or OE"}
	}
	if !validControlTypes[strings.ToUpper(req.ControlType)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlType must be CONFIG or NON_CONFIG"}
	}
	if !validScopes[strings.ToUpper(req.Scope)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "scope must be COMMON or PRODUCT_SPECIFIC"}
	}
	if req.CreatedBy == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	// dueDate is required for every control (framework-linked controls resolve it
	// from the template instead, same carve-out as controlNumber/description
	// above). OE controls additionally need a population due date — this was
	// previously only enforced by the standalone population-create endpoint,
	// which the webapp never calls; the inline Population block on control
	// creation went completely unvalidated, letting an OE control end up with no
	// due date anywhere.
	if req.DueDate == nil || strings.TrimSpace(*req.DueDate) == "" {
		if req.FrameworkControlID == nil {
			return domain.AuditControl{}, &apierror.ValidationError{Msg: "dueDate is required"}
		}
	}
	if strings.EqualFold(req.RequirementType, "OE") {
		if req.Population == nil || req.Population.DueDate == nil || strings.TrimSpace(*req.Population.DueDate) == "" {
			return domain.AuditControl{}, &apierror.ValidationError{Msg: "population.dueDate is required for OE controls"}
		}
	}
	req.RequirementType = strings.ToUpper(req.RequirementType)
	req.ControlType = strings.ToUpper(req.ControlType)
	req.Scope = strings.ToUpper(req.Scope)
	c, err := s.repo.CreateControl(ctx, auditID, req)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}

func (s *controlService) ListAssignedForEvidence(ctx context.Context, userEmail string) (domain.ListAssignedControlsResponse, error) {
	if userEmail == "" {
		return domain.ListAssignedControlsResponse{}, &apierror.ValidationError{Msg: "email is required"}
	}
	controls, err := s.repo.ListAssignedForEvidence(ctx, userEmail)
	if err != nil {
		return domain.ListAssignedControlsResponse{}, err
	}
	if controls == nil {
		controls = []domain.AssignedControlForEvidence{}
	}
	return domain.ListAssignedControlsResponse{Controls: controls}, nil
}

func (s *controlService) GetEvidenceAssignment(ctx context.Context, userEmail string, controlID int) (domain.EvidenceAssignmentResponse, error) {
	if userEmail == "" {
		return domain.EvidenceAssignmentResponse{}, &apierror.ValidationError{Msg: "email is required"}
	}
	if controlID <= 0 {
		return domain.EvidenceAssignmentResponse{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	auditID, err := s.repo.GetEvidenceAssignment(ctx, userEmail, controlID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EvidenceAssignmentResponse{}, &apierror.NotFoundError{Msg: "not assigned to this control"}
	}
	if err != nil {
		return domain.EvidenceAssignmentResponse{}, err
	}
	return domain.EvidenceAssignmentResponse{AuditID: auditID}, nil
}

func (s *controlService) FindActivePopulation(ctx context.Context, controlID int) (domain.ActivePopulationResponse, error) {
	if controlID <= 0 {
		return domain.ActivePopulationResponse{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	populationID, err := s.repo.FindActivePopulation(ctx, controlID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ActivePopulationResponse{}, &apierror.NotFoundError{Msg: "no active population for this control"}
	}
	if err != nil {
		return domain.ActivePopulationResponse{}, err
	}
	return domain.ActivePopulationResponse{PopulationID: populationID}, nil
}

func (s *controlService) UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.ControlType != nil {
		upper := strings.ToUpper(*req.ControlType)
		if !validControlTypes[upper] {
			return domain.AuditControl{}, &apierror.ValidationError{Msg: "invalid controlType: " + *req.ControlType}
		}
		req.ControlType = &upper
	}
	if req.Scope != nil {
		upper := strings.ToUpper(*req.Scope)
		if !validScopes[upper] {
			return domain.AuditControl{}, &apierror.ValidationError{Msg: "invalid scope: " + *req.Scope}
		}
		req.Scope = &upper
	}
	// DueDate is optional on update (nil means "leave unchanged" — e.g. a
	// status-transition PATCH never sends it), but a caller that does send the
	// field may not clear a control's due date to empty.
	if req.DueDate != nil && strings.TrimSpace(*req.DueDate) == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "dueDate cannot be cleared"}
	}
	if req.Status != nil {
		if !validControlStatuses[strings.ToUpper(*req.Status)] {
			return domain.AuditControl{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
		}
		// Enforce workflow order: the target status must be reachable from the
		// control's current status. Prevents skipping review/validation stages.
		current, err := s.repo.GetControlByID(ctx, auditID, controlID)
		if err != nil {
			return domain.AuditControl{}, err
		}
		if !isValidControlTransition(current.Status, *req.Status) {
			return domain.AuditControl{}, &apierror.ValidationError{
				Msg: fmt.Sprintf("invalid status transition: %s -> %s", current.Status, *req.Status),
			}
		}
		// Pass current status to the repo so the UPDATE enforces it atomically,
		// preventing TOCTOU races between the read above and the write below.
		req.ExpectedStatus = current.Status
	}
	c, err := s.repo.UpdateControl(ctx, auditID, controlID, req)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}
