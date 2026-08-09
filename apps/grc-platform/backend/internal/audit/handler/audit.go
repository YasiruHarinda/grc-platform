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
	"strconv"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type auditHandler struct {
	svc service.AuditService
}

// listAudits handles GET /api/v1/audits.
func (h *auditHandler) listAudits(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	user := auth.FromContext(ctx)
	var email string
	if user != nil {
		email = user.Email
	}
	audits, err := h.svc.ListScoped(ctx, scope, email)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if audits == nil {
		audits = []*model.Audit{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.AuditListResponse{
		Items: audits,
		Total: len(audits),
	})
}

// getAudit handles GET /api/v1/audits/{id}.
func (h *auditHandler) getAudit(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	id, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	if scope != model.ScopeAll {
		user := auth.FromContext(ctx)
		var email string
		if user != nil {
			email = user.Email
		}
		ok, err := h.svc.InScope(ctx, id, scope, email)
		if err != nil {
			response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
			return
		}
		if !ok {
			response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
			return
		}
	}
	audit, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, audit)
}

// createAudit handles POST /api/v1/audits.
func (h *auditHandler) createAudit(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.CreateAudit) {
		return
	}
	var req model.CreateAuditRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	audit, err := h.svc.Create(r.Context(), req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, audit)
}

// updateAudit handles PUT /api/v1/audits/{id}.
func (h *auditHandler) updateAudit(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.UpdateAudit) {
		return
	}
	id, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	var req model.UpdateAuditRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	if err := h.svc.Update(r.Context(), id, req, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAudit handles DELETE /api/v1/audits/{id}.
func (h *auditHandler) deleteAudit(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.UpdateAudit) {
		return
	}
	id, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	if err := h.svc.Delete(r.Context(), id, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseIntParam extracts a named path value and writes a 400 on failure.
func parseIntParam(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	s := r.PathValue(name)
	v, err := strconv.Atoi(s)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid "+name+" parameter")
		return 0, false
	}
	return v, true
}
