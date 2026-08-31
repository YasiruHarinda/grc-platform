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

// Package adminactivity is a thin client for the Admin Console's own
// "who did what and when" log (admin_activity_log), separate from audit_trail.
package adminactivity

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// logTimeout bounds the detached POST in Log. The request context is cancelled
// the moment the handler returns, so the write runs on a context of its own.
const logTimeout = 30 * time.Second

// Action names — mirror admin_activity_log.action's ENUM exactly.
const (
	ActionCreated       = "CREATED"
	ActionUpdated       = "UPDATED"
	ActionDeleted       = "DELETED"
	ActionStatusChanged = "STATUS_CHANGED"
	ActionGranted       = "GRANTED"
	ActionRevoked       = "REVOKED"
)

// Entity type names — mirror admin_activity_log.entity_type's ENUM exactly.
const (
	EntityUser                = "USER"
	EntityGrant               = "GRANT"
	EntityRiskTeam            = "RISK_TEAM"
	EntityRiskCategory        = "RISK_CATEGORY"
	EntityComplianceReference = "COMPLIANCE_REFERENCE"
	EntityRiskScore           = "RISK_SCORE"
	EntityAuditTeam           = "AUDIT_TEAM"
)

// Client talks to the Compliance Entity's /admin-activity-log endpoints.
type Client struct{ c *entityclient.Client }

// NewClient constructs a Client over the shared entity HTTP client.
func NewClient(c *entityclient.Client) *Client { return &Client{c: c} }

// Entry is one activity-log row. ActorName/ActorEmail start empty — the
// caller resolves them from the identity directory.
type Entry struct {
	ID            int64          `json:"id"`
	ActorID       string         `json:"actorId"`
	ActorName     string         `json:"actorName"`
	ActorEmail    string         `json:"actorEmail"`
	ActorUserType string         `json:"actorUserType"`
	Action        string         `json:"action"`
	EntityType    string         `json:"entityType"`
	EntityID      int            `json:"entityId"`
	Details       map[string]any `json:"details,omitempty"`
	CreatedOn     string         `json:"createdOn"`
}

// ListResponse is List's return shape.
type ListResponse struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
	Limit   int     `json:"limit"`
	Offset  int     `json:"offset"`
}

// Filter narrows List; every field is optional (zero-value skips it).
type Filter struct {
	ActorID    string
	Action     string
	EntityType string
	From       string // YYYY-MM-DD
	To         string // YYYY-MM-DD
}

// Log records one activity-log entry, best-effort — never blocks the caller.
// The POST runs detached on a background context: it must still land when the
// client disconnects right after a mutation commits, which cancels the request
// context. The details map is marshalled synchronously so a later mutation by
// the caller cannot race the write.
func (cl *Client) Log(ctx context.Context, actor, action, entityType string, entityID int, details map[string]any) {
	if cl == nil {
		return
	}
	body := map[string]any{
		"actorId":    actor,
		"action":     action,
		"entityType": entityType,
		"entityId":   entityID,
	}
	if len(details) > 0 {
		b, err := json.Marshal(details)
		if err != nil {
			slog.WarnContext(ctx, "admin activity log: marshal details failed", "action", action, "entityType", entityType, "err", err)
		} else {
			body["details"] = string(b)
		}
	}
	go func() {
		// A bare goroutine has no panic net; an unguarded panic here would
		// take the whole process down instead of losing one log entry.
		defer func() {
			if p := recover(); p != nil {
				slog.Error("admin activity log: panic", "action", action, "entityType", entityType, "entityId", entityID, "panic", p)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), logTimeout)
		defer cancel()
		if err := cl.c.Post(ctx, "/admin-activity-log", body, nil); err != nil {
			slog.WarnContext(ctx, "admin activity log: record failed",
				"action", action, "entityType", entityType, "entityId", entityID, "err", err)
		}
	}()
}

// List returns activity-log entries, newest first, unresolved (ActorName/
// ActorEmail empty — see Entry's doc comment).
func (cl *Client) List(ctx context.Context, filter Filter, limit, offset int) (ListResponse, error) {
	type entEntry struct {
		ID            int64  `json:"id"`
		ActorID       string `json:"actorId"`
		ActorUserType string `json:"actorUserType"`
		Action        string `json:"action"`
		EntityType    string `json:"entityType"`
		EntityID      int    `json:"entityId"`
		Details       string `json:"details"`
		CreatedOn     string `json:"createdOn"`
	}
	var resp struct {
		Entries []entEntry `json:"entries"`
		Total   int        `json:"total"`
		Limit   int        `json:"limit"`
		Offset  int        `json:"offset"`
	}

	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	if filter.ActorID != "" {
		q.Set("actorId", filter.ActorID)
	}
	if filter.Action != "" {
		q.Set("action", filter.Action)
	}
	if filter.EntityType != "" {
		q.Set("entityType", filter.EntityType)
	}
	if filter.From != "" {
		q.Set("from", filter.From)
	}
	if filter.To != "" {
		q.Set("to", filter.To)
	}

	if err := cl.c.Get(ctx, "/admin-activity-log?"+q.Encode(), &resp); err != nil {
		return ListResponse{}, err
	}

	out := ListResponse{Total: resp.Total, Limit: resp.Limit, Offset: resp.Offset, Entries: make([]Entry, 0, len(resp.Entries))}
	for _, e := range resp.Entries {
		entry := Entry{
			ID: e.ID, ActorID: e.ActorID, ActorUserType: e.ActorUserType,
			Action: e.Action, EntityType: e.EntityType, EntityID: e.EntityID,
			CreatedOn: e.CreatedOn,
		}
		if e.Details != "" {
			var d map[string]any
			if json.Unmarshal([]byte(e.Details), &d) == nil {
				entry.Details = d
			}
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}
