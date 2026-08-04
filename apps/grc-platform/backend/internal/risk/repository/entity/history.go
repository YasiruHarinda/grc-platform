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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type historyRepository struct {
	c *entityclient.Client
}

// NewHistoryRepository creates a Compliance Entity-backed HistoryRepository over
// the entity's /risks/{id}/changes endpoints.
func NewHistoryRepository(c *entityclient.Client) repository.HistoryRepository {
	return &historyRepository{c: c}
}

// entHistory is the entity's camelCase change-log row.
type entHistory struct {
	ID           int64     `json:"id"`
	RiskID       int       `json:"riskId"`
	CreatedBy    string    `json:"createdBy"`
	Action       string    `json:"action"`
	FieldChanged *string   `json:"fieldChanged"`
	OldValue     *string   `json:"oldValue"`
	NewValue     *string   `json:"newValue"`
	Details      *string   `json:"details"`
	CreatedOn    time.Time `json:"createdOn"`
}

func (e entHistory) toModel() *model.HistoryEntry {
	h := &model.HistoryEntry{
		ID:           e.ID,
		RiskID:       e.RiskID,
		Action:       e.Action,
		FieldChanged: e.FieldChanged,
		OldValue:     e.OldValue,
		NewValue:     e.NewValue,
		CreatedBy:    e.CreatedBy,
		CreatedAt:    e.CreatedOn,
	}
	// details is stored as a JSON string; hand it to the client as raw JSON so
	// it isn't double-encoded. A row written before this column existed, or one
	// holding something unparseable, simply carries no details rather than
	// failing the whole listing.
	if e.Details != nil && json.Valid([]byte(*e.Details)) {
		h.Details = json.RawMessage(*e.Details)
	}
	return h
}

// historyPageLimit caps a single listing. A risk accumulating more events than
// this would be extraordinary — the log records deliberate actions, not
// telemetry — and the cap keeps one pathological risk from stalling the drawer.
const historyPageLimit = 500

func (r *historyRepository) List(ctx context.Context, riskID int) ([]*model.HistoryEntry, error) {
	var resp struct {
		Changes []entHistory `json:"changes"`
	}
	path := fmt.Sprintf("/risks/%d/changes?limit=%d&offset=0", riskID, historyPageLimit)
	if err := r.c.Get(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("list history for risk %d: %w", riskID, err)
	}
	out := make([]*model.HistoryEntry, 0, len(resp.Changes))
	for _, e := range resp.Changes {
		out = append(out, e.toModel())
	}
	return out, nil
}

func (r *historyRepository) Record(ctx context.Context, riskID int, req model.RecordHistoryRequest, createdBy string) error {
	body := map[string]any{
		"createdBy":    createdBy,
		"action":       req.Action,
		"fieldChanged": req.FieldChanged,
		"oldValue":     req.OldValue,
		"newValue":     req.NewValue,
	}
	if req.Details != nil {
		encoded, err := json.Marshal(req.Details)
		if err != nil {
			return fmt.Errorf("encode history details: %w", err)
		}
		s := string(encoded)
		body["details"] = &s
	}
	var out entHistory
	if err := r.c.Post(ctx, fmt.Sprintf("/risks/%d/changes", riskID), body, &out); err != nil {
		return fmt.Errorf("record history for risk %d: %w", riskID, err)
	}
	return nil
}
