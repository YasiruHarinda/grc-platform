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

type controlService struct {
	repo          repository.ControlRepository
	frameworkRepo repository.FrameworkControlRepository
	auditRepo     repository.AuditRepository
}

// NewControlService constructs a ControlService. frameworkRepo/auditRepo back
// the optional "push to framework" side effect on control creation (see
// applyFrameworkPushBack) — resolving the audit's framework id and writing
// into the catalog are the only things that ever touch audit_framework_control
// from this service.
func NewControlService(repo repository.ControlRepository, frameworkRepo repository.FrameworkControlRepository, auditRepo repository.AuditRepository) ControlService {
	return &controlService{repo: repo, frameworkRepo: frameworkRepo, auditRepo: auditRepo}
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

// controlStatusRank orders every control status for the backward status
// override: a single ordered list rather than a second transition map, so
// legality can never drift out of sync with itself. An override is legal iff
// rank(target) < rank(current).
var controlStatusRank = map[string]int{
	"POPULATION_PENDING":            0,
	"POPULATION_INTERNAL_REVIEW":    1,
	"POPULATION_NEED_CLARIFICATION": 1,
	"POPULATION_UNDER_VALIDATION":   2,
	"POPULATION_COMPLETE":           3,
	"AWAITING_SAMPLE":               4,
	"SUBMITTED_SAMPLE":              5,
	"EVIDENCE_PENDING":              6,
	"EVIDENCE_NEED_CLARIFICATION":   6,
	"EVIDENCE_INTERNAL_REVIEW":      7,
	"EVIDENCE_UNDER_VALIDATION":     8,
	"COMPLETE":                      9,
}

// designControlFloorRank is the lowest rank a DESIGN control may be overridden
// to. DESIGN controls have no audit_population row, so rewinding one into the
// population phase would deadlock it: FindActivePopulation would find nothing
// and the team could never submit.
const designControlFloorRank = 6

// isValidOverrideTransition reports whether a backward status override from
// -> to is legal for a control of the given requirement type: strictly
// backward by rank, floored at designControlFloorRank for DESIGN controls.
func isValidOverrideTransition(requirementType, from, to string) bool {
	fromRank, fromOK := controlStatusRank[strings.ToUpper(from)]
	toRank, toOK := controlStatusRank[strings.ToUpper(to)]
	if !fromOK || !toOK {
		return false
	}
	if strings.EqualFold(requirementType, "DESIGN") && toRank < designControlFloorRank {
		return false
	}
	return toRank < fromRank
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
		if c.ControlNumber == "" {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: controlNumber is required", i)}
		}
		if c.Description == "" {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: description is required", i)}
		}
		if c.DueDate == nil || strings.TrimSpace(*c.DueDate) == "" {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: dueDate is required", i)}
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
	if err := s.applyFrameworkPushBacks(ctx, auditID, req.Controls); err != nil {
		return domain.BulkCreateControlsResponse{}, err
	}
	return domain.BulkCreateControlsResponse{Controls: controls, Created: len(controls)}, nil
}

// applyFrameworkPushBacks performs the optional framework-catalog side
// effect for each control in a bulk create that requested it
// (PushToFramework=true). Runs after the audit_control rows are committed:
// pushControlToFramework writes directly to the catalog (its own commit,
// outside the control-insert transaction), so running it first risked
// stranding a committed catalog row if the control insert then failed (e.g.
// duplicate control number within the audit). A push-back failure here still
// leaves the audit_control rows in place unlinked from the catalog, which is
// a valid state on its own (PushToFramework is opt-in). Resolves the audit's
// framework id once and reuses it for every row. No-op if nothing in the
// batch opted in.
func (s *controlService) applyFrameworkPushBacks(ctx context.Context, auditID int, reqs []domain.CreateControlRequest) error {
	needsFramework := false
	for _, r := range reqs {
		if r.PushToFramework {
			needsFramework = true
			break
		}
	}
	if !needsFramework {
		return nil
	}
	audit, err := s.auditRepo.GetAuditByID(ctx, auditID)
	if err != nil {
		return err
	}
	for _, r := range reqs {
		if !r.PushToFramework {
			continue
		}
		if err := s.pushControlToFramework(ctx, audit.FrameworkID, r); err != nil {
			return err
		}
	}
	return nil
}

// pushControlToFramework writes one control into the framework catalog: a new
// version of an existing catalog control (SourceFrameworkControlID set), or a
// first version of a brand-new one — rejected if that control number already
// exists in the framework's current catalog under a different id.
func (s *controlService) pushControlToFramework(ctx context.Context, frameworkID int, r domain.CreateControlRequest) error {
	if r.SourceFrameworkControlID != nil {
		_, err := s.frameworkRepo.NewVersion(ctx, *r.SourceFrameworkControlID, domain.UpdateFrameworkControlRequest{
			Description:         &r.Description,
			EvidenceRequirement: r.EvidenceRequirement,
			RequirementType:     &r.RequirementType,
			ControlType:         &r.ControlType,
			Scope:               &r.Scope,
			UpdatedBy:           r.CreatedBy,
		})
		return err
	}
	existing, err := s.frameworkRepo.GetCurrentByNumber(ctx, frameworkID, r.ControlNumber)
	if err == nil {
		return &apierror.ValidationError{Msg: fmt.Sprintf(
			"control %s already exists in this framework (id %d), uncheck 'add to library' or pick it from the existing list instead",
			r.ControlNumber, existing.ID)}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.frameworkRepo.Create(ctx, frameworkID, domain.CreateFrameworkControlRequest{
		ControlNumber:       r.ControlNumber,
		Description:         r.Description,
		EvidenceRequirement: r.EvidenceRequirement,
		RequirementType:     r.RequirementType,
		ControlType:         r.ControlType,
		Scope:               r.Scope,
		CreatedBy:           r.CreatedBy,
	})
	return err
}

func (s *controlService) DeleteControl(ctx context.Context, auditID, controlID int) error {
	if auditID <= 0 {
		return &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	// audit_evidence and audit_population both cascade-delete with the control at
	// the DB level (see audit_schema.sql), so once work has started on a control
	// deleting it silently destroys that work. Block it here instead.
	evidenceCount, activePopulationCount, err := s.repo.CountDeletionBlockers(ctx, controlID)
	if err != nil {
		return err
	}
	if evidenceCount > 0 || activePopulationCount > 0 {
		var reasons []string
		if evidenceCount > 0 {
			reasons = append(reasons, fmt.Sprintf("%d evidence submission(s)", evidenceCount))
		}
		if activePopulationCount > 0 {
			reasons = append(reasons, fmt.Sprintf("%d population(s) in progress", activePopulationCount))
		}
		return &apierror.ConflictError{
			Msg: fmt.Sprintf("cannot delete control: %s exist for this control", strings.Join(reasons, " and ")),
		}
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
	if req.ControlNumber == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlNumber is required"}
	}
	if req.Description == "" {
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
	// dueDate is required for every control. OE controls additionally need a
	// population due date — this was previously only enforced by the standalone
	// population-create endpoint, which the webapp never calls; the inline
	// Population block on control creation went completely unvalidated, letting
	// an OE control end up with no due date anywhere.
	if req.DueDate == nil || strings.TrimSpace(*req.DueDate) == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "dueDate is required"}
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
	if req.PushToFramework {
		audit, err := s.auditRepo.GetAuditByID(ctx, auditID)
		if err != nil {
			return domain.AuditControl{}, err
		}
		// Runs after the control insert commits (see applyFrameworkPushBacks) so a
		// rejected push-back can't strand an already-committed catalog row.
		if err := s.pushControlToFramework(ctx, audit.FrameworkID, req); err != nil {
			return domain.AuditControl{}, err
		}
	}
	return *c, nil
}

func (s *controlService) GetEvidenceAssignment(ctx context.Context, userID int, controlID int) (domain.EvidenceAssignmentResponse, error) {
	if userID <= 0 {
		return domain.EvidenceAssignmentResponse{}, &apierror.ValidationError{Msg: "userId is required"}
	}
	if controlID <= 0 {
		return domain.EvidenceAssignmentResponse{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	auditID, err := s.repo.GetEvidenceAssignment(ctx, userID, controlID)
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

// OverrideControlStatus backward-overrides a control's status: legality is
// rank-based (isValidOverrideTransition) rather than allowedControlTransitions,
// so it can reach statuses ordinary UpdateControl never allows moving back to
// (e.g. COMPLETE -> EVIDENCE_PENDING). The repo cascades dependent
// audit_population/audit_evidence rows and stamps the override marker in the
// same transaction as the status write.
func (s *controlService) OverrideControlStatus(ctx context.Context, auditID, controlID int, req domain.OverrideControlStatusRequest) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	target := strings.ToUpper(req.Status)
	if !validControlStatuses[target] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "invalid status: " + req.Status}
	}

	current, err := s.repo.GetControlByID(ctx, auditID, controlID)
	if err != nil {
		return domain.AuditControl{}, err
	}

	audit, err := s.auditRepo.GetAuditByID(ctx, auditID)
	if err != nil {
		return domain.AuditControl{}, err
	}
	if audit.Status == "REMOVED" {
		return domain.AuditControl{}, &apierror.ConflictError{Msg: "cannot override status: audit is removed"}
	}

	if !isValidOverrideTransition(current.RequirementType, current.Status, target) {
		return domain.AuditControl{}, &apierror.ValidationError{
			Msg: fmt.Sprintf("invalid status override: %s -> %s", current.Status, target),
		}
	}

	req.Status = target
	// Pass current status to the repo so the UPDATE enforces it atomically,
	// preventing TOCTOU races between the read above and the write below.
	req.ExpectedStatus = current.Status
	c, err := s.repo.OverrideControlStatus(ctx, auditID, controlID, req)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}
