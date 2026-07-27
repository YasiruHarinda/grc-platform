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

// TrailService defines business operations for the audit trail (append-only log).
type TrailService interface {
	// ListByControl returns a control's history, newest first, with the total count.
	ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditTrailEntry, int, error)
	// RecordEvidenceAction appends an attribution entry for an evidence/population
	// action, tagging the channel it came through (web-app vs evidence-app) and the
	// token issuer so portal actions stay distinguishable (design §I). evidenceID may
	// be 0 (population submit) — it is then omitted.
	RecordEvidenceAction(ctx context.Context, auditID, controlID, evidenceID int, action, actor, via, issuer string) error
	// RecordControlAction appends a control-scoped lifecycle entry (e.g. CREATED, or
	// a status transition carrying {"from","to"} in details). details may be nil.
	RecordControlAction(ctx context.Context, auditID, controlID int, action, actor string, details map[string]any) error
}

type trailService struct {
	repo repository.TrailRepository
}

func NewTrailService(repo repository.TrailRepository) TrailService {
	return &trailService{repo: repo}
}

func (s *trailService) ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditTrailEntry, int, error) {
	entries, total, err := s.repo.ListByControl(ctx, auditID, controlID, defaultTrailLimit)
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

func (s *trailService) RecordEvidenceAction(ctx context.Context, auditID, controlID, evidenceID int, action, actor, via, issuer string) error {
	details, err := json.Marshal(map[string]string{"via": via, "issuer": issuer})
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
