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

// Package service implements the business logic for the Audit Hub module.
// Handlers call service methods; services call repository methods.
// Validation rules and workflow guards live here — not in handlers or repositories.
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

// AuditService defines business operations for audit engagements.
type AuditService interface {
	List(ctx context.Context) ([]*model.Audit, error)
	ListScoped(ctx context.Context, scope model.Scope, userEmail string, scopeTeamIDs []int) ([]*model.Audit, error)
	GetByID(ctx context.Context, id int) (*model.Audit, error)
	InScope(ctx context.Context, id int, scope model.Scope, userEmail string, scopeTeamIDs []int) (bool, error)
	Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error)
	Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error
	Delete(ctx context.Context, id int, deletedBy string) error
}

type auditService struct {
	repo          repository.AuditRepository
	frameworkRepo repository.FrameworkRepository
	productRepo   repository.ProductRepository
	// trail records audit-level lifecycle events (created, updated, deleted) to
	// the append-only audit trail. Best-effort; may be nil (recording is then
	// skipped) — same contract as controlService.recordTrail.
	trail TrailService
}

// NewAuditService wires audit, framework, and product repos so Create can
// validate references, plus trail for audit-level lifecycle logging.
func NewAuditService(
	repo repository.AuditRepository,
	frameworkRepo repository.FrameworkRepository,
	productRepo repository.ProductRepository,
	trail TrailService,
) AuditService {
	return &auditService{repo: repo, frameworkRepo: frameworkRepo, productRepo: productRepo, trail: trail}
}

// recordTrail appends a best-effort audit-level lifecycle entry. A failure here
// must never fail the operation that triggered it — the trail is advisory
// history, not part of the write it describes.
func (s *auditService) recordTrail(ctx context.Context, auditID int, action, actor string, details map[string]any) {
	if s.trail == nil {
		return
	}
	if err := s.trail.RecordAuditAction(ctx, auditID, action, actor, details); err != nil {
		slog.WarnContext(ctx, "record audit trail failed", "auditId", auditID, "action", action, "err", err)
	}
}

func (s *auditService) List(ctx context.Context) ([]*model.Audit, error) {
	return s.repo.List(ctx)
}

func (s *auditService) ListScoped(ctx context.Context, scope model.Scope, userEmail string, scopeTeamIDs []int) ([]*model.Audit, error) {
	return s.repo.ListScoped(ctx, scope, userEmail, scopeTeamIDs)
}

func (s *auditService) InScope(ctx context.Context, id int, scope model.Scope, userEmail string, scopeTeamIDs []int) (bool, error) {
	return s.repo.InScope(ctx, id, scope, userEmail, scopeTeamIDs)
}

func (s *auditService) GetByID(ctx context.Context, id int) (*model.Audit, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	return a, nil
}

func (s *auditService) Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error) {
	if req.FrameworkID <= 0 || req.ProductID <= 0 {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "frameworkId and productId are required"}
	}
	if req.PeriodStart == "" || req.PeriodEnd == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "periodStart and periodEnd are required"}
	}
	if createdBy == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "authenticated user email is missing from token — check Asgardeo app email scope"}
	}

	// Verify framework and product exist.
	fw, err := s.frameworkRepo.GetByID(ctx, req.FrameworkID)
	if err != nil {
		return nil, err
	}
	if fw == nil {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "framework not found"}
	}
	pr, err := s.productRepo.GetByID(ctx, req.ProductID)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "product not found"}
	}

	// The audit name is also the top-level Azure folder for its evidence (see
	// Human-Readable-Evidence-Blob-Paths design), so it is composed here rather
	// than left to a free-text field: auto-filled from framework/product/year
	// when blank, always run through the same path-safe charset, and — once
	// this audit exists — locked (see Update below).
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = composeAuditName(fw.Name, pr.Name, req.PeriodStart)
	}
	name = sanitizeSegment(name)
	if name == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name is required"}
	}
	if err := s.checkNameAvailable(ctx, name, 0); err != nil {
		return nil, err
	}
	req.Name = name

	created, err := s.repo.Create(ctx, req, createdBy)
	if err != nil {
		return nil, err
	}
	s.recordTrail(ctx, created.ID, "CREATED", createdBy, map[string]any{"name": created.Name})
	return created, nil
}

// composeAuditName builds the default audit name "{Framework} {Product} {Year}"
// (e.g. "SOC 2 Asgardeo 2026") from the period start's year.
func composeAuditName(frameworkName, productName, periodStart string) string {
	year := periodStart
	if len(periodStart) >= 4 {
		year = periodStart[:4]
	}
	return frameworkName + " " + productName + " " + year
}

// checkNameAvailable enforces case-insensitive uniqueness across all audits,
// excluding excludeID (used by Update to allow an audit to keep its own name).
// This is an app-level check only — it cannot close a create/create race, but a
// concurrent duplicate is vanishingly unlikely for a human-driven creation flow
// and the entity is out of scope for a DB-level constraint here.
func (s *auditService) checkNameAvailable(ctx context.Context, name string, excludeID int) error {
	existing, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, a := range existing {
		if a.ID != excludeID && strings.EqualFold(a.Name, name) {
			return &apierror.Error{StatusCode: http.StatusConflict, Body: "an audit named \"" + name + "\" already exists"}
		}
	}
	return nil
}

func (s *auditService) Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	// The name is a storage identifier once the audit exists (it is the audit's
	// Azure folder) — reject any attempt to change it. A no-op resubmission of
	// the same value is allowed so a broader "save all fields" update doesn't
	// have to special-case the name field.
	if req.Name != nil && !strings.EqualFold(strings.TrimSpace(*req.Name), a.Name) {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "audit name is locked once created — it is used as the evidence storage path"}
	}
	if err := s.repo.Update(ctx, id, req, updatedBy); err != nil {
		return err
	}
	// Logged as a generic UPDATED entry regardless of which fields changed — no
	// special-cased status-change event, even when Status is the only field set.
	s.recordTrail(ctx, id, "UPDATED", updatedBy, updatedAuditFields(req))
	return nil
}

// updatedAuditFields collects the non-nil fields of an UpdateAuditRequest into a
// details map for the trail entry, so the log shows what actually changed.
func updatedAuditFields(req model.UpdateAuditRequest) map[string]any {
	details := map[string]any{}
	if req.PeriodStart != nil {
		details["periodStart"] = *req.PeriodStart
	}
	if req.PeriodEnd != nil {
		details["periodEnd"] = *req.PeriodEnd
	}
	if req.ScopeDescription != nil {
		details["scopeDescription"] = *req.ScopeDescription
	}
	if req.Status != nil {
		details["status"] = *req.Status
	}
	return details
}

func (s *auditService) Delete(ctx context.Context, id int, deletedBy string) error {
	if deletedBy == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "authenticated user email is missing from token — check Asgardeo app email scope"}
	}
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	// Recorded before the delete call: the audit itself is soft-deleted (status
	// -> REMOVED, schema audit_schema.sql:111) so the row survives either way,
	// but this matches the "record before delete" ordering used for controls
	// (where the row is a hard delete and order matters) rather than relying on
	// that distinction holding forever.
	s.recordTrail(ctx, id, "DELETED", deletedBy, map[string]any{"name": a.Name})
	return s.repo.Delete(ctx, id, deletedBy)
}
