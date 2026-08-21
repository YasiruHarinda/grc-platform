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

type teamHandler struct {
	svc service.TeamService
}

// listTeams handles GET /api/v1/audit/teams.
//
// Gated on ViewAudits (not ManageAuditHub) by default: every audit user needs
// this for the control assignment dropdown, and it returns ACTIVE teams only.
//
// ?includeInactive=true switches to the Manage Audit Hub admin table's
// contract instead — every status, Status populated on each row — and is
// additionally gated on ManageAuditHub, since an inactive team list is
// reference-data administration, not everyday Audit Hub use.
func (h *teamHandler) listTeams(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}

	includeInactive := r.URL.Query().Get("includeInactive") == "true"
	if includeInactive && !auth.RequirePrivilege(r.Context(), w, privilege.ManageAuditHub) {
		return
	}

	var teams []*model.AuditTeam
	var err error
	if includeInactive {
		teams, err = h.svc.ListAll(r.Context())
	} else {
		teams, err = h.svc.List(r.Context())
	}
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if teams == nil {
		teams = []*model.AuditTeam{}
	}
	response.WriteJSONValue(w, http.StatusOK, teams)
}

// createTeam handles POST /api/v1/audit/teams — the Manage Audit Hub "Add
// Team" dialog. Gated on ManageAuditHub, not ManageUsers: reference-data
// management, a separate authorisation boundary from user provisioning, same
// precedent as Risk Teams' MANAGE_RISK_HUB gate.
func (h *teamHandler) createTeam(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageAuditHub) {
		return
	}

	var req model.CreateTeamRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Name == "" {
		response.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	createdBy := ""
	if user := auth.FromContext(r.Context()); user != nil {
		createdBy = user.Subject
	}

	t, err := h.svc.Create(r.Context(), req, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, t)
}

// updateTeam handles PUT /api/v1/audit/teams/{id}.
func (h *teamHandler) updateTeam(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageAuditHub) {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	var req model.UpdateTeamRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Name == "" {
		response.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	updatedBy := ""
	if user := auth.FromContext(r.Context()); user != nil {
		updatedBy = user.Subject
	}

	t, err := h.svc.Update(r.Context(), id, req, updatedBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, t)
}
