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
	"encoding/json"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// defaultTrailLimit caps how many history entries a single control view fetches.
// A control's lifecycle rarely exceeds a few dozen events; this matches the
// entity's own max page size (100), so it is the most we can get in one call.
const defaultTrailLimit = 100

// defaultAuditTrailLimit is the default page size for the audit-wide activity log.
const defaultAuditTrailLimit = 100

// TrailService defines business operations for the audit trail (append-only log).
type TrailService interface {
	// ListByControl returns a control's history, newest first, with the total count.
	// includeInternal=false excludes internal COMMENTED entries entity-side before the limit.
	ListByControl(ctx context.Context, auditID, controlID int, includeInternal bool) ([]*model.AuditTrailEntry, int, error)
	// ListByAudit returns the whole audit's trail (audit-level and every control's
	// events together), newest first, narrowed by filter, with the total count.
	ListByAudit(ctx context.Context, auditID int, filter model.TrailFilter, limit, offset int) ([]*model.AuditTrailEntry, int, error)
	// RecordEvidenceAction appends an attribution entry for an evidence/population
	// action, tagging the channel it came through and the token issuer.
	// evidenceID may be 0 (population submit) — it is then omitted. fileNames is the round's file
	// names at the time of this action (nil/empty when not applicable, e.g.
	// population/sample submissions), recorded so the History tab and the
	// audit-wide Activity Log can show what was actually submitted without a
	// separate lookup.
	RecordEvidenceAction(ctx context.Context, auditID, controlID, evidenceID int, action, actor, via, issuer string, fileNames []string) error
	// RecordControlAction appends a control-scoped lifecycle entry (e.g. CREATED, or
	// a status transition carrying {"from","to"} in details). details may be nil.
	RecordControlAction(ctx context.Context, auditID, controlID int, action, actor string, details map[string]any) error
	// RecordAuditAction appends an audit-level lifecycle entry (CREATED, UPDATED,
	// DELETED) — no control_id, since these describe the audit record itself.
	// details may be nil.
	RecordAuditAction(ctx context.Context, auditID int, action, actor string, details map[string]any) error
}

type trailService struct {
	repo repository.TrailRepository
}

func NewTrailService(repo repository.TrailRepository) TrailService {
	return &trailService{repo: repo}
}

func (s *trailService) ListByControl(ctx context.Context, auditID, controlID int, includeInternal bool) ([]*model.AuditTrailEntry, int, error) {
	entries, total, err := s.repo.ListByControl(ctx, auditID, controlID, defaultTrailLimit, includeInternal)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (s *trailService) ListByAudit(ctx context.Context, auditID int, filter model.TrailFilter, limit, offset int) ([]*model.AuditTrailEntry, int, error) {
	if limit <= 0 {
		limit = defaultAuditTrailLimit
	}
	entries, total, err := s.repo.ListByAudit(ctx, auditID, filter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (s *trailService) RecordControlAction(ctx context.Context, auditID, controlID int, action, actor string, details map[string]any) error {
	var detailsJSON string
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			return err
		}
		detailsJSON = string(b)
	}
	ctrl := controlID
	return s.repo.Create(ctx, auditID, &ctrl, nil, action, detailsJSON, actor)
}

func (s *trailService) RecordAuditAction(ctx context.Context, auditID int, action, actor string, details map[string]any) error {
	var detailsJSON string
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			return err
		}
		detailsJSON = string(b)
	}
	return s.repo.Create(ctx, auditID, nil, nil, action, detailsJSON, actor)
}

func (s *trailService) RecordEvidenceAction(ctx context.Context, auditID, controlID, evidenceID int, action, actor, via, issuer string, fileNames []string) error {
	payload := map[string]any{"via": via, "issuer": issuer}
	if len(fileNames) > 0 {
		payload["files"] = fileNames
	}
	details, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var evidencePtr *int
	if evidenceID > 0 {
		evidencePtr = &evidenceID
	}
	ctrl := controlID
	return s.repo.Create(ctx, auditID, &ctrl, evidencePtr, action, string(details), actor)
}
