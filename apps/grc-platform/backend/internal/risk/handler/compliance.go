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

// handleListComplianceReferences serves GET /api/v1/compliance-references.
//
// Gated on ViewRisks OR ManageRiskHub — same shape as GET /api/v1/teams and
// GET /api/v1/users (team.go, users.go): read here was ungated while its
// write counterparts (Create/Update/Delete below) were already gated on
// ManageRiskHub, letting any authenticated caller enumerate the compliance
// reference data.
func (d *Deps) handleListComplianceReferences(w http.ResponseWriter, r *http.Request) {
	if !auth.HasPrivilege(r.Context(), privilege.ViewRisks) && !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	refs, err := d.Compliance.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	if refs == nil {
		refs = []*model.ComplianceReference{}
	}
	response.WriteJSONValue(w, http.StatusOK, refs)
}

// handleCreateComplianceReference serves POST /api/v1/compliance-references —
// the Admin Console's Compliance References "Add Reference" dialog. Gated on
// MANAGE_RISK_HUB, same boundary as the other reference-data write routes.
func (d *Deps) handleCreateComplianceReference(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	var req model.CreateComplianceRefRequest
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

	ref, err := d.Compliance.Create(r.Context(), req, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, ref)
}

// handleUpdateComplianceReference serves PUT /api/v1/compliance-references/{id}.
func (d *Deps) handleUpdateComplianceReference(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	var req model.UpdateComplianceRefRequest
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

	ref, err := d.Compliance.Update(r.Context(), id, req, updatedBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, ref)
}

// handleDeleteComplianceReference serves DELETE /api/v1/compliance-references/{id}.
// The entity refuses (409) when the reference is still tagged on a risk — see
// repository.ComplianceReferenceRepository.Delete's doc comment.
func (d *Deps) handleDeleteComplianceReference(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	if err := d.Compliance.Delete(r.Context(), id); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
