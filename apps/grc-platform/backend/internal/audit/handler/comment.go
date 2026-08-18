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

type commentHandler struct {
	svc        service.CommentService
	controlSvc service.ControlService
}

// listComments handles GET /api/v1/audits/{id}/controls/{controlId}/comments.
func (h *commentHandler) listComments(w http.ResponseWriter, r *http.Request) {
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
	control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	teamID := 0
	if control.TeamID != nil {
		teamID = *control.TeamID
	}
	// Internal-only comments are shown to holders of the internal-comments
	// privilege (the internal roles) and hidden from external auditors, who
	// lack it. See docs/adr/0002-privilege-derived-scope.md. Checked against
	// control's own team (HasPrivilegeIn), since the privilege can be granted
	// scoped to a single team (module=AUDIT) — the unscoped HasPrivilege would
	// let a team-scoped grant see every other team's internal comments too.
	includeInternal := auth.HasPrivilegeIn(r.Context(), privilege.ViewInternalComments, teamID)
	comments, err := h.svc.List(r.Context(), auditID, controlID, includeInternal)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if comments == nil {
		comments = []*model.AuditComment{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.CommentListResponse{Items: comments})
}

// addComment handles POST /api/v1/audits/{id}/controls/{controlId}/comments.
func (h *commentHandler) addComment(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.AddComment) {
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
	var req model.AddCommentRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	c, err := h.svc.Add(r.Context(), auditID, controlID, req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, c)
}
