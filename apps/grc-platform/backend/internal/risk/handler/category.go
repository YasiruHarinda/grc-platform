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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleListRiskCategories serves GET /api/v1/risk-categories.
//
// Gated on ViewRisks OR ManageRiskHub — same shape as GET /api/v1/teams and
// GET /api/v1/users (team.go, users.go): read here was ungated while its
// write counterparts (Create/Update/Delete below) were already gated on
// ManageRiskHub, letting any authenticated caller enumerate the risk
// category reference data.
func (d *Deps) handleListRiskCategories(w http.ResponseWriter, r *http.Request) {
	if !auth.HasPrivilege(r.Context(), privilege.ViewRisks) && !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	cats, err := d.Category.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	if cats == nil {
		cats = []*model.RiskCategory{}
	}
	response.WriteJSONValue(w, http.StatusOK, cats)
}

// handleCreateRiskCategory serves POST /api/v1/risk-categories — the Admin
// Console's Risk Categories "Add Category" dialog. Gated on MANAGE_RISK_HUB.
func (d *Deps) handleCreateRiskCategory(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	var req model.CreateRiskCategoryRequest
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

	cat, err := d.Category.Create(r.Context(), req, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, cat)
}

// handleUpdateRiskCategory serves PUT /api/v1/risk-categories/{id}.
func (d *Deps) handleUpdateRiskCategory(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	var req model.UpdateRiskCategoryRequest
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

	cat, err := d.Category.Update(r.Context(), id, req, updatedBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, cat)
}

// handleDeleteRiskCategory serves DELETE /api/v1/risk-categories/{id}. The
// entity refuses (409) when the category is still used by a risk — see
// repository.RiskCategoryRepository.Delete's doc comment for why that check
// can't be left to a DB constraint here.
func (d *Deps) handleDeleteRiskCategory(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	if err := d.Category.Delete(r.Context(), id); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
