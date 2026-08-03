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
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// validStatuses is the set of allowed control status transitions the API accepts
// directly. It mirrors the audit_control.status ENUM in audit_schema.sql exactly
// (12 statuses) — keep in sync with the schema.
var validStatuses = map[string]bool{
	// OE — population phase
	"POPULATION_PENDING":            true,
	"POPULATION_INTERNAL_REVIEW":    true,
	"POPULATION_UNDER_VALIDATION":   true,
	"POPULATION_NEED_CLARIFICATION": true,
	"POPULATION_COMPLETE":           true,
	// OE — sample phase
	"AWAITING_SAMPLE":  true,
	"SUBMITTED_SAMPLE": true,
	// Evidence phase
	"EVIDENCE_PENDING":            true,
	"EVIDENCE_INTERNAL_REVIEW":    true,
	"EVIDENCE_UNDER_VALIDATION":   true,
	"EVIDENCE_NEED_CLARIFICATION": true,
	// Terminal
	"COMPLETE": true,
}

// ControlService defines business operations for audit controls.
type ControlService interface {
	List(ctx context.Context, auditID int) ([]*model.AuditControl, error)
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
	Add(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error)
	BulkAdd(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error)
	Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error
	UpdateStatus(ctx context.Context, auditID, controlID int, req model.UpdateStatusRequest, updatedBy string) error
	// UpdateStatusWithSample is UpdateStatus plus setting the sample note
	// atomically — used when the auditor submits the sample.
	UpdateStatusWithSample(ctx context.Context, auditID, controlID int, status, sampleReference, updatedBy string) error
	Delete(ctx context.Context, auditID, controlID int, deletedBy string) error
	GetAssignedForEvidence(ctx context.Context, userEmail string) ([]*model.AssignedControlForEvidence, error)
	// AssignedAuditID returns the audit id for controlID when userEmail's team is
	// assigned and the control is actionable; found=false means not assigned.
	AssignedAuditID(ctx context.Context, userEmail string, controlID int) (auditID int, found bool, err error)
	// ActivePopulationID returns the active population round id for an OE control;
	// found=false means no active population (e.g. a DESIGN control).
	ActivePopulationID(ctx context.Context, controlID int) (populationID int, found bool, err error)
}

type controlService struct {
	repo repository.ControlRepository
	// population is used only to edit an OE control's population details
	// (requirement text, due date, comments, owner, team) from the same form
	// used to create them.
	population repository.PopulationRepository
	// trail records lifecycle events (created, status transitions) to the
	// append-only audit trail. Best-effort; may be nil (recording is then skipped).
	trail TrailService
}

func NewControlService(repo repository.ControlRepository, population repository.PopulationRepository, trail TrailService) ControlService {
	return &controlService{repo: repo, population: population, trail: trail}
}

// recordTrail appends a best-effort lifecycle entry. A failure here must never
// fail the operation that triggered it — the trail is advisory history, not part
// of the write it describes.
func (s *controlService) recordTrail(ctx context.Context, auditID, controlID int, action, actor string, details map[string]any) {
	if s.trail == nil {
		return
	}
	if err := s.trail.RecordControlAction(ctx, auditID, controlID, action, actor, details); err != nil {
		slog.WarnContext(ctx, "record control trail failed", "controlId", controlID, "action", action, "err", err)
	}
}

// statusChangeAction maps a target status to the coarse trail action used for the
// event's icon/colour. The precise transition is always carried in details
// {from,to}; this only decides advance (APPROVED) vs send-back (REJECTED).
func statusChangeAction(to string) string {
	switch {
	case strings.HasSuffix(to, "_NEED_CLARIFICATION"), strings.HasSuffix(to, "_PENDING"):
		return "REJECTED"
	default:
		return "APPROVED"
	}
}

func (s *controlService) List(ctx context.Context, auditID int) ([]*model.AuditControl, error) {
	return s.repo.List(ctx, auditID)
}

func (s *controlService) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	return c, nil
}

// sanitizedControlNumbers returns a lowercased-sanitized-number → original-number
// index of every control currently in the audit. Blob folders key on
// sanitizeSegment(ControlNumber) (see evidence.go resolveNames), so raw values
// like "CA.01" and "CA:01" both collapse to "CA-01" and would otherwise share
// the same evidence/population/sample folder — this index lets callers reject
// a new or renamed control whose sanitized number collides with an existing
// one, mirroring checkNameAvailable's post-sanitization check for audit names.
func (s *controlService) sanitizedControlNumbers(ctx context.Context, auditID int) (map[string]string, error) {
	existing, err := s.repo.List(ctx, auditID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]string, len(existing))
	for _, c := range existing {
		index[strings.ToLower(sanitizeSegment(c.ControlNumber))] = c.ControlNumber
	}
	return index, nil
}

func (s *controlService) Add(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error) {
	if err := validateAddRequest(req); err != nil {
		return nil, err
	}
	index, err := s.sanitizedControlNumbers(ctx, auditID)
	if err != nil {
		return nil, err
	}
	if orig, ok := index[strings.ToLower(sanitizeSegment(req.ControlNumber))]; ok {
		return nil, &apierror.Error{StatusCode: http.StatusConflict, Body: "a control numbered \"" + orig + "\" already exists in this audit"}
	}
	c, err := s.repo.Create(ctx, auditID, req, createdBy)
	if err != nil {
		return nil, err
	}
	s.recordTrail(ctx, auditID, c.ID, "CREATED", createdBy, map[string]any{
		"controlNumber": c.ControlNumber,
	})
	return c, nil
}

func (s *controlService) BulkAdd(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error) {
	if len(reqs) == 0 {
		return []*model.AuditControl{}, nil
	}
	for _, req := range reqs {
		if err := validateAddRequest(req); err != nil {
			return nil, err
		}
	}
	index, err := s.sanitizedControlNumbers(ctx, auditID)
	if err != nil {
		return nil, err
	}
	for _, req := range reqs {
		key := strings.ToLower(sanitizeSegment(req.ControlNumber))
		if orig, ok := index[key]; ok {
			return nil, &apierror.Error{StatusCode: http.StatusConflict, Body: "a control numbered \"" + orig + "\" already exists in this audit (conflicts with \"" + req.ControlNumber + "\")"}
		}
		index[key] = req.ControlNumber
	}
	return s.repo.BulkCreate(ctx, auditID, reqs, createdBy)
}

func (s *controlService) Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error {
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if c == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	if req.ControlNumber != nil && strings.TrimSpace(*req.ControlNumber) != "" {
		index, err := s.sanitizedControlNumbers(ctx, auditID)
		if err != nil {
			return err
		}
		delete(index, strings.ToLower(sanitizeSegment(c.ControlNumber))) // exclude this control's current number
		if orig, ok := index[strings.ToLower(sanitizeSegment(*req.ControlNumber))]; ok {
			return &apierror.Error{StatusCode: http.StatusConflict, Body: "a control numbered \"" + orig + "\" already exists in this audit"}
		}
	}
	// DueDate is optional here (nil means "leave unchanged"), but a caller that
	// does send the field may not clear a control's due date to empty.
	if req.DueDate != nil && strings.TrimSpace(*req.DueDate) == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "dueDate cannot be cleared"}
	}
	// Population is only meaningful for OE controls — silently ignored for
	// DESIGN controls rather than erroring, since the form simply never sends
	// it for them.
	if req.Population != nil && c.RequirementType == "OE" {
		if strings.TrimSpace(req.Population.Description) == "" {
			return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "population.description is required"}
		}
		if req.Population.DueDate == nil || strings.TrimSpace(*req.Population.DueDate) == "" {
			return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "population.dueDate is required"}
		}
	}
	if err := s.repo.Update(ctx, auditID, controlID, req, updatedBy); err != nil {
		return err
	}
	if req.Population != nil && c.RequirementType == "OE" {
		// Deliberately not ActivePopulationID here: that only resolves a round
		// still in PENDING/COMPLIANCE_REJECTED/AUDITOR_REJECTED, so it returns
		// not-found (silently skipping the edit) for any control whose
		// population has already advanced past that — approved, in sample
		// phase, or complete. Editing details should work regardless of phase,
		// so take the control's most recent round instead.
		rounds, err := s.population.ListByControl(ctx, auditID, controlID)
		if err != nil {
			return err
		}
		if len(rounds) > 0 {
			latest := rounds[len(rounds)-1]
			if err := s.population.UpdateDetails(ctx, latest.ID, *req.Population, updatedBy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *controlService) UpdateStatus(ctx context.Context, auditID, controlID int, req model.UpdateStatusRequest, updatedBy string) error {
	if !validStatuses[req.Status] {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "invalid status value"}
	}
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if c == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	// TODO(status-workflow): enforce the control status TRANSITION rules here.
	// Above only checks that req.Status is a valid enum value — a caller can still
	// jump straight to any status (e.g. EVIDENCE_PENDING -> COMPLETE) and skip
	// internal review + auditor validation. The current status is already loaded in
	// `c.Status`, so add: if the move c.Status -> req.Status is not allowed, return
	// 422 "invalid status transition". Reuse the same transition map implemented in
	// compliance-entity/internal/service/audit_control_service.go (allowedControlTransitions
	// / isValidControlTransition) so both layers agree. (This is the live enforcement
	// point until the backend is migrated to call the compliance entity.)
	if err := s.repo.UpdateStatus(ctx, auditID, controlID, req.Status, req.Comment, updatedBy); err != nil {
		return err
	}

	// Record the transition for the History tab. Skip no-op updates and moves into
	// *_INTERNAL_REVIEW: a submission is already logged as an UPLOADED event, so
	// re-recording it as a status change would double up the timeline.
	if req.Status != c.Status && !strings.HasSuffix(req.Status, "_INTERNAL_REVIEW") {
		details := map[string]any{"from": c.Status, "to": req.Status}
		if req.Comment != nil && *req.Comment != "" {
			details["comment"] = *req.Comment
		}
		s.recordTrail(ctx, auditID, controlID, statusChangeAction(req.Status), updatedBy, details)
	}
	return nil
}

func (s *controlService) UpdateStatusWithSample(ctx context.Context, auditID, controlID int, status, sampleReference, updatedBy string) error {
	if !validStatuses[status] {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "invalid status value"}
	}
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if c == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	if err := s.repo.UpdateStatusWithSample(ctx, auditID, controlID, status, sampleReference, updatedBy); err != nil {
		return err
	}
	if status != c.Status {
		s.recordTrail(ctx, auditID, controlID, statusChangeAction(status), updatedBy, map[string]any{
			"from": c.Status,
			"to":   status,
		})
	}
	return nil
}

func (s *controlService) Delete(ctx context.Context, auditID, controlID int, deletedBy string) error {
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if c == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	// Record before deleting: audit_trail.control_id is a foreign key to
	// audit_control.id, so writing this row after the delete would fail (there's
	// no longer a matching control row) and the DELETED entry would silently
	// never appear — recordTrail swallows its own errors by design.
	s.recordTrail(ctx, auditID, controlID, "DELETED", deletedBy, map[string]any{
		"controlNumber": c.ControlNumber,
	})
	return s.repo.Delete(ctx, auditID, controlID)
}

func (s *controlService) GetAssignedForEvidence(ctx context.Context, userEmail string) ([]*model.AssignedControlForEvidence, error) {
	return s.repo.ListAssignedForEvidence(ctx, userEmail)
}

func (s *controlService) AssignedAuditID(ctx context.Context, userEmail string, controlID int) (int, bool, error) {
	return s.repo.AssignedAuditID(ctx, userEmail, controlID)
}

func (s *controlService) ActivePopulationID(ctx context.Context, controlID int) (int, bool, error) {
	return s.repo.ActivePopulationID(ctx, controlID)
}

func validateAddRequest(req model.AddControlRequest) error {
	if req.ControlNumber == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "controlNumber is required"}
	}
	if req.Description == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "description is required"}
	}
	if req.EvidenceRequirement == nil || strings.TrimSpace(*req.EvidenceRequirement) == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "evidenceRequirement is required"}
	}
	if req.DueDate == nil || strings.TrimSpace(*req.DueDate) == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "dueDate is required"}
	}
	if req.RequirementType != "DESIGN" && req.RequirementType != "OE" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "requirementType must be DESIGN or OE"}
	}
	if req.ControlType != "CONFIG" && req.ControlType != "NON_CONFIG" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "controlType must be CONFIG or NON_CONFIG"}
	}
	if req.Scope != "COMMON" && req.Scope != "PRODUCT_SPECIFIC" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "scope must be COMMON or PRODUCT_SPECIFIC"}
	}
	// The inline Population block on OE control creation was previously
	// forwarded unvalidated — the entity only enforced a due date via the
	// separate population-create endpoint, which nothing in the webapp calls.
	if req.RequirementType == "OE" {
		if req.Population == nil || req.Population.DueDate == nil || strings.TrimSpace(*req.Population.DueDate) == "" {
			return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "population.dueDate is required for OE controls"}
		}
	}
	return nil
}
