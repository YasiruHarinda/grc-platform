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
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type trailHandler struct {
	svc service.TrailService
}

// listControlTrail handles GET /api/v1/audits/{id}/controls/{controlId}/trail.
//
// Returns the control's immutable history (append-only audit_trail), newest
// first, for the History tab. Read access is gated on ViewAudits — the trail
// only surfaces events, never evidence bytes.
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

	entries, total, err := h.svc.ListByControl(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if entries == nil {
		entries = []*model.AuditTrailEntry{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.TrailListResponse{
		Items: entries,
		Total: total,
	})
}
