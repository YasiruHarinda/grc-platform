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

package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type trailRepo struct{ c *entityclient.Client }

// NewTrailRepository returns an entity-backed TrailRepository.
func NewTrailRepository(c *entityclient.Client) repository.TrailRepository {
	return &trailRepo{c: c}
}

func (r *trailRepo) Create(ctx context.Context, auditID int, controlID, evidenceID *int, action, details, createdBy string) error {
	body := map[string]any{
		"controlId":  controlID,
		"evidenceId": evidenceID,
		"action":     action,
		"createdBy":  createdBy,
	}
	if details != "" {
		body["details"] = details
	}
	return r.c.Post(ctx, fmt.Sprintf("/audits/%d/trail", auditID), body, nil)
}

// entTrail mirrors the entity's AuditTrail JSON. Details is a raw JSON string on
// the wire (the column is MySQL JSON); we hand it back to the client verbatim.
type entTrail struct {
	ID         int64     `json:"id"`
	ControlID  *int      `json:"controlId"`
	EvidenceID *int      `json:"evidenceId"`
	Action     string    `json:"action"`
	Details    *string   `json:"details"`
	CreatedBy  *string   `json:"createdBy"`
	CreatedOn  time.Time `json:"createdOn"`
}

func (r *trailRepo) ListByControl(ctx context.Context, auditID, controlID, limit int) ([]*model.AuditTrailEntry, int, error) {
	var resp struct {
		Trail []entTrail `json:"trail"`
		Total int        `json:"total"`
	}
	path := fmt.Sprintf("/audits/%d/trail?controlId=%d&limit=%d", auditID, controlID, limit)
	if err := r.c.Get(ctx, path, &resp); err != nil {
		return nil, 0, err
	}

	out := make([]*model.AuditTrailEntry, 0, len(resp.Trail))
	for _, e := range resp.Trail {
		entry := &model.AuditTrailEntry{
			ID:         e.ID,
			Action:     e.Action,
			ControlID:  e.ControlID,
			EvidenceID: e.EvidenceID,
			CreatedAt:  e.CreatedOn,
		}
		if e.CreatedBy != nil {
			entry.CreatedBy = *e.CreatedBy
		}
		if e.Details != nil && *e.Details != "" {
			entry.Details = json.RawMessage(*e.Details)
		}
		out = append(out, entry)
	}
	return out, resp.Total, nil
}
