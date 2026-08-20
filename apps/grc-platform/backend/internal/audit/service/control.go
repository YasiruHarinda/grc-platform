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
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
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
	ListScoped(ctx context.Context, auditID int, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.AuditControl, error)
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
	InScope(ctx context.Context, auditID, controlID int, scope model.Scope, userID int, scopeTeamIDs []int) (bool, error)
	Add(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error)
	BulkAdd(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error)
	// Update edits a control (and optionally its population round) and
	// reports what changed via ControlUpdateResult, so the caller (handler,
	// not this service — see notify.go) can decide what to notify.
	Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) (ControlUpdateResult, error)
	UpdateStatus(ctx context.Context, auditID, controlID int, req model.UpdateStatusRequest, updatedBy string) error
	// OverrideStatus backward-overrides a control's status (ManageControls-gated
	// admin escape hatch) — distinct from UpdateStatus's ordinary forward workflow.
	OverrideStatus(ctx context.Context, auditID, controlID int, req model.OverrideStatusRequest, actor string) error
	// UpdateStatusWithSample is UpdateStatus plus setting the sample note
	// atomically — used when the auditor submits the sample.
	UpdateStatusWithSample(ctx context.Context, auditID, controlID int, status, sampleReference, updatedBy string) error
	Delete(ctx context.Context, auditID, controlID int, deletedBy string) error
	// AssignedAuditID returns the audit id for controlID when userID is the
	// control's owner and the control is actionable; found=false means not assigned.
	AssignedAuditID(ctx context.Context, userID int, controlID int) (auditID int, found bool, err error)
	// ActivePopulationID returns the active population round id for an OE control;
	// found=false means no active population (e.g. a DESIGN control).
	ActivePopulationID(ctx context.Context, controlID int) (populationID int, found bool, err error)
	// ListAllForReminders returns every control across every audit — for the
	// daily reminder job's sweep (internal/audit/job). See
	// repository.ControlRepository.ListAllForReminders.
	ListAllForReminders(ctx context.Context) ([]*model.AuditControl, error)
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
	// directory resolves OwnerUUID/AuditorUUID/PopulationOwnerUUID to display
	// names (see enrichNames) — the `user` table stores none itself. Nil is
	// tolerated (local dev without SCIM configured): names simply stay unset.
	directory *directory.Service
}

func NewControlService(repo repository.ControlRepository, population repository.PopulationRepository, trail TrailService, dir *directory.Service) ControlService {
	return &controlService{repo: repo, population: population, trail: trail, directory: dir}
}

// enrichNames batch-resolves OwnerUUID/AuditorUUID/PopulationOwnerUUID on
// every control to a display name via the identity directory — one batched
// lookup for the whole slice, not one per control. Best-effort: a uuid the
// directory doesn't know (or an absent owner/auditor) simply leaves the
// corresponding *Name field nil, the same way an unassigned control already
// carries a nil OwnerName.
func (s *controlService) enrichNames(ctx context.Context, controls []*model.AuditControl) {
	if s.directory == nil {
		return
	}
	uuidTypes := map[string]string{}
	addRef := func(uuid, userType *string) {
		if uuid == nil || *uuid == "" {
			return
		}
		ut := ""
		if userType != nil {
			ut = *userType
		}
		uuidTypes[*uuid] = ut
	}
	for _, c := range controls {
		addRef(c.OwnerUUID, c.OwnerUserType)
		addRef(c.AuditorUUID, c.AuditorUserType)
		addRef(c.PopulationOwnerUUID, c.PopulationOwnerUserType)
	}
	if len(uuidTypes) == 0 {
		return
	}
	people := s.directory.LookupAllTyped(ctx, uuidTypes)
	nameFor := func(uuid *string) *string {
		if uuid == nil || *uuid == "" {
			return nil
		}
		if p, ok := people[*uuid]; ok && p.DisplayName != "" {
			name := p.DisplayName
			return &name
		}
		return nil
	}
	for _, c := range controls {
		c.OwnerName = nameFor(c.OwnerUUID)
		c.AuditorName = nameFor(c.AuditorUUID)
		c.PopulationOwnerName = nameFor(c.PopulationOwnerUUID)
	}
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

// List is unscoped and used internally (e.g. sanitizedControlNumbers) — its
// result is never shown to a user, so it skips name enrichment.
func (s *controlService) List(ctx context.Context, auditID int) ([]*model.AuditControl, error) {
	return s.repo.List(ctx, auditID)
}

func (s *controlService) ListScoped(ctx context.Context, auditID int, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.AuditControl, error) {
	controls, err := s.repo.ListScoped(ctx, auditID, scope, userID, scopeTeamIDs)
	if err != nil {
		return nil, err
	}
	s.enrichNames(ctx, controls)
	return controls, nil
}

func (s *controlService) InScope(ctx context.Context, auditID, controlID int, scope model.Scope, userID int, scopeTeamIDs []int) (bool, error) {
	return s.repo.InScope(ctx, auditID, controlID, scope, userID, scopeTeamIDs)
}

func (s *controlService) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	s.enrichNames(ctx, []*model.AuditControl{c})
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
	s.enrichNames(ctx, []*model.AuditControl{c})
	return c, nil
}

// maxBulkControls bounds a single bulk-add request. Keep this in sync with
// maxItems on BulkAddControlsRequest.controls in openapi.yaml.
const maxBulkControls = 500

func (s *controlService) BulkAdd(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error) {
	if len(reqs) == 0 {
		return []*model.AuditControl{}, nil
	}
	if len(reqs) > maxBulkControls {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: fmt.Sprintf("controls must not exceed %d items per request", maxBulkControls)}
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
	created, err := s.repo.BulkCreate(ctx, auditID, reqs, createdBy)
	if err != nil {
		return nil, err
	}
	s.enrichNames(ctx, created)
	return created, nil
}

// ControlUpdateResult reports what Update changed, so the caller (the
// handler, not this service) can decide what to notify — see notify.go.
// "Changed" means the field was provided, non-nil, and differs from the
// value before this update; a field cleared to nil, or simply not sent, is
// never a change (no outgoing-owner notification — see the design note on
// notifyAuditEvent).
type ControlUpdateResult struct {
	ControlOwnerChanged    bool
	PopulationOwnerChanged bool
	AuditorChanged         bool
	NewControlOwnerID      *int
	NewPopulationOwnerID   *int
	NewAuditorID           *int
}

func (s *controlService) Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) (ControlUpdateResult, error) {
	var result ControlUpdateResult
	c, err := s.repo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return result, err
	}
	if c == nil {
		return result, &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	if req.ControlNumber != nil && strings.TrimSpace(*req.ControlNumber) != "" {
		index, err := s.sanitizedControlNumbers(ctx, auditID)
		if err != nil {
			return result, err
		}
		delete(index, strings.ToLower(sanitizeSegment(c.ControlNumber))) // exclude this control's current number
		if orig, ok := index[strings.ToLower(sanitizeSegment(*req.ControlNumber))]; ok {
			return result, &apierror.Error{StatusCode: http.StatusConflict, Body: "a control numbered \"" + orig + "\" already exists in this audit"}
		}
	}
	// DueDate is optional here (nil means "leave unchanged"), but a caller that
	// does send the field may not clear a control's due date to empty.
	if req.DueDate != nil && strings.TrimSpace(*req.DueDate) == "" {
		return result, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "dueDate cannot be cleared"}
	}
	// Population is only meaningful for OE controls — silently ignored for
	// DESIGN controls rather than erroring, since the form simply never sends
	// it for them.
	if req.Population != nil && c.RequirementType == "OE" {
		if strings.TrimSpace(req.Population.Description) == "" {
			return result, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "population.description is required"}
		}
		if req.Population.DueDate == nil || strings.TrimSpace(*req.Population.DueDate) == "" {
			return result, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "population.dueDate is required"}
		}
	}

	if req.OwnerID != nil && (c.OwnerID == nil || *req.OwnerID != *c.OwnerID) {
		result.ControlOwnerChanged = true
		result.NewControlOwnerID = req.OwnerID
	}
	if req.AuditorID != nil && (c.AuditorID == nil || *req.AuditorID != *c.AuditorID) {
		result.AuditorChanged = true
		result.NewAuditorID = req.AuditorID
	}

	if err := s.repo.Update(ctx, auditID, controlID, req, updatedBy); err != nil {
		return result, err
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
			return result, err
		}
		if len(rounds) > 0 {
			latest := rounds[len(rounds)-1]
			if req.Population.OwnerID != nil && (latest.OwnerID == nil || *req.Population.OwnerID != *latest.OwnerID) {
				result.PopulationOwnerChanged = true
				result.NewPopulationOwnerID = req.Population.OwnerID
			}
			if err := s.population.UpdateDetails(ctx, latest.ID, *req.Population, updatedBy); err != nil {
				return result, err
			}
		}
	}
	return result, nil
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
	// Above only checks that req.Status is a valid enum value — transition
	// legality (is c.Status -> req.Status an allowed workflow step) is enforced
	// by the compliance entity when this PATCH lands on it; the entity is the
	// sole enforcer, so this layer does not duplicate that check.
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

// OverrideStatus backward-overrides a control's status via the entity's
// rank-based status-override endpoint (see ControlRepository.OverrideStatus).
// Unlike UpdateStatus, transition legality is backward-only by rank rather
// than the ordinary forward workflow, and is enforced entirely by the entity.
// The trail entry is always recorded — the *_INTERNAL_REVIEW suppression in
// UpdateStatus does not apply here, since an override is never the routine
// submission event that suppression exists to avoid double-logging.
func (s *controlService) OverrideStatus(ctx context.Context, auditID, controlID int, req model.OverrideStatusRequest, actor string) error {
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
	if err := s.repo.OverrideStatus(ctx, auditID, controlID, req.Status, actor); err != nil {
		return err
	}

	details := map[string]any{"from": c.Status, "to": req.Status}
	s.recordTrail(ctx, auditID, controlID, "OVERRIDDEN", actor, details)
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

func (s *controlService) AssignedAuditID(ctx context.Context, userID int, controlID int) (int, bool, error) {
	return s.repo.AssignedAuditID(ctx, userID, controlID)
}

func (s *controlService) ActivePopulationID(ctx context.Context, controlID int) (int, bool, error) {
	return s.repo.ActivePopulationID(ctx, controlID)
}

func (s *controlService) ListAllForReminders(ctx context.Context) ([]*model.AuditControl, error) {
	return s.repo.ListAllForReminders(ctx)
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
