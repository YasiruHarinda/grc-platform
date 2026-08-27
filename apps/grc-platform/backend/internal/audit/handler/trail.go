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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// trailDateFormat is the ?from=/?to= query-param format (date-only), matching a
// typical date-range picker.
const trailDateFormat = "2006-01-02"

type trailHandler struct {
	svc service.TrailService
	// controlSvc/auditSvc back the controlInScope/auditInScope checks below —
	// see their doc comments in dashboard.go.
	controlSvc service.ControlService
	auditSvc   service.AuditService
	directory  *directory.Service
}

// resolveTrailActors fills each entry's CreatedByName from CreatedBy (the
// actor's raw uuid, per audit_trail's write path), batching the directory
// lookups for the whole page rather than one call per row. CreatedBy itself
// is left untouched — see AuditTrailEntry.CreatedByName.
//
// Falls back to email, then the bare uuid, for an actor the directory can't
// resolve. Uses LookupAllTyped, routed by each entry's CreatedByUserType: an
// external auditor (AUDIT_VALIDATE_EVIDENCE/AUDIT_REVIEW_EVIDENCE are
// external-reachable — see evidenceHandler.requireEvidenceFileAccess) can be
// the actor of a trail row via controlService.UpdateStatus, and the internal-
// only Lookup could never resolve their identity.
func (h *trailHandler) resolveTrailActors(ctx context.Context, entries []*model.AuditTrailEntry) {
	uuidTypes := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.CreatedBy != "" {
			uuidTypes[e.CreatedBy] = e.CreatedByUserType
		}
	}
	people := h.directory.LookupAllTyped(ctx, uuidTypes)
	for _, e := range entries {
		p, ok := people[e.CreatedBy]
		if !ok {
			e.CreatedByName = e.CreatedBy
			continue
		}
		switch {
		case strings.TrimSpace(p.DisplayName) != "":
			e.CreatedByName = strings.TrimSpace(p.DisplayName)
		case p.Email != "":
			e.CreatedByName = p.Email
		default:
			e.CreatedByName = e.CreatedBy
		}
	}
}

// filterInternalComments drops COMMENTED entries whose details mark them
// isInternal when the caller lacks ViewInternalComments — same contract as commentService.List.
func filterInternalComments(entries []*model.AuditTrailEntry, includeInternal bool) []*model.AuditTrailEntry {
	if includeInternal {
		return entries
	}
	visible := make([]*model.AuditTrailEntry, 0, len(entries))
	for _, e := range entries {
		if e.Action == "COMMENTED" {
			var details struct {
				IsInternal *bool `json:"isInternal"`
			}
			// Fail closed: exclude unless the details explicitly say isInternal:false.
			err := json.Unmarshal(e.Details, &details)
			if err != nil || details.IsInternal == nil || *details.IsInternal {
				continue
			}
		}
		visible = append(visible, e)
	}
	return visible
}

// listControlTrail handles GET /api/v1/audits/{id}/controls/{controlId}/trail.
//
// Returns the control's immutable history (append-only audit_trail), newest
// first, for the History tab. Read access is gated on ViewAudits plus the
// same controlInScope check getControl uses — ViewAudits alone is a coarse
// boolean with no row scope, so without it a caller scoped to one control
// (e.g. an assigned external auditor) could read another audit's trail by
// guessing its id.
func (h *trailHandler) listControlTrail(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !controlInScope(w, r, h.controlSvc, auditID, controlID) {
		return
	}

	entries, total, err := h.svc.ListByControl(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	entries = filterInternalComments(entries, auth.HasPrivilege(r.Context(), privilege.ViewInternalComments))
	total = len(entries)
	if entries == nil {
		entries = []*model.AuditTrailEntry{}
	}
	h.resolveTrailActors(r.Context(), entries)
	response.WriteJSONValue(w, http.StatusOK, &model.TrailListResponse{
		Items: entries,
		Total: total,
	})
}

// listAuditTrail handles GET /api/v1/audits/{id}/trail.
//
// Returns the whole audit's activity — audit-level events (created/updated/
// deleted) and every control's events together, newest first — for the
// audit-wide Activity Log page. Same ViewAudits gate as the per-control
// history, plus the same auditInScope check getAudit uses.
func (h *trailHandler) listAuditTrail(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	if !auditInScope(w, r, h.auditSvc, auditID) {
		return
	}

	q := r.URL.Query()
	var limit, offset int
	if raw := q.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = v
	}
	if raw := q.Get("offset"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "offset must be an integer")
			return
		}
		if v < 0 {
			v = 0
		}
		offset = v
	}

	var filter model.TrailFilter
	for _, raw := range q["controlId"] {
		cid, err := strconv.Atoi(raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "controlId must be a positive integer")
			return
		}
		filter.ControlIDs = append(filter.ControlIDs, cid)
	}
	if raw := q.Get("from"); raw != "" {
		from, err := time.Parse(trailDateFormat, raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
			return
		}
		filter.From = &from
	}
	if raw := q.Get("to"); raw != "" {
		to, err := time.Parse(trailDateFormat, raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
			return
		}
		filter.To = &to
	}

	// Row-scope control-level entries — auditInScope alone lets a
	// single-control caller see every other control's trail rows too.
	filter.Scope, _ = deriveScopes(r.Context())
	if user := auth.FromContext(r.Context()); user != nil {
		filter.UserID = user.UserID
	}
	filter.ScopeTeamIDs = managedTeamIDs(auth.Grants(r.Context()))

	entries, total, err := h.svc.ListByAudit(r.Context(), auditID, filter, limit, offset)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	includeInternal := auth.HasPrivilege(r.Context(), privilege.ViewInternalComments)
	if !includeInternal {
		dropped := len(entries)
		entries = filterInternalComments(entries, includeInternal)
		dropped -= len(entries)
		// Approximate: only accounts for this page's dropped rows, not other pages.
		total -= dropped
	}
	if entries == nil {
		entries = []*model.AuditTrailEntry{}
	}
	h.resolveTrailActors(r.Context(), entries)
	response.WriteJSONValue(w, http.StatusOK, &model.TrailListResponse{
		Items: entries,
		Total: total,
	})
}
