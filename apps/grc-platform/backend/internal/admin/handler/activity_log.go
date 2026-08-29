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

package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleListActivityLog serves GET /api/v1/admin/activity-log, gated on
// holding MANAGE_USERS, MANAGE_RISK_HUB, AND MANAGE_AUDIT_HUB together.
func (d *Deps) handleListActivityLog(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAllPrivileges(r.Context(), w, privilege.ManageUsers, privilege.ManageRiskHub, privilege.ManageAuditHub) {
		return
	}
	if d.ActivityLog == nil {
		response.WriteError(w, http.StatusInternalServerError, response.ErrMsgInternal)
		return
	}

	q := r.URL.Query()
	limit, offset := 50, 0
	if raw := q.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			response.WriteError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = v
	}
	if raw := q.Get("offset"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			response.WriteError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = v
	}

	filter := adminactivity.Filter{
		ActorID:    q.Get("actorId"),
		Action:     q.Get("action"),
		EntityType: q.Get("entityType"),
		From:       q.Get("from"),
		To:         q.Get("to"),
	}

	resp, err := d.ActivityLog.List(r.Context(), filter, limit, offset)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	resolveActivityLogActors(r.Context(), d.Directory, resp.Entries)
	response.WriteJSONValue(w, http.StatusOK, resp)
}

// resolveActivityLogActors fills each entry's ActorName/ActorEmail from
// ActorID, batching directory lookups for the whole page.
func resolveActivityLogActors(ctx context.Context, dir *directory.Service, entries []adminactivity.Entry) {
	if dir == nil {
		return
	}
	uuidTypes := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.ActorID != "" {
			uuidTypes[e.ActorID] = e.ActorUserType
		}
	}
	people := dir.LookupAllTyped(ctx, uuidTypes)
	for i := range entries {
		p, ok := people[entries[i].ActorID]
		if !ok {
			continue
		}
		switch {
		case strings.TrimSpace(p.DisplayName) != "":
			entries[i].ActorName = strings.TrimSpace(p.DisplayName)
		case p.Email != "":
			entries[i].ActorName = p.Email
		}
		entries[i].ActorEmail = p.Email
	}
}
