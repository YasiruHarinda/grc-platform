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

type frameworkHandler struct {
	svc service.FrameworkService
}

// listFrameworks handles GET /api/v1/audit/frameworks.
func (h *frameworkHandler) listFrameworks(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	frameworks, err := h.svc.ListFrameworks(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if frameworks == nil {
		frameworks = []*model.AuditFramework{}
	}
	response.WriteJSONValue(w, http.StatusOK, frameworks)
}

// createFramework handles POST /api/v1/audit/frameworks.
func (h *frameworkHandler) createFramework(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	var req model.CreateFrameworkRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	fw, err := h.svc.CreateFramework(r.Context(), req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, fw)
}

// listProducts handles GET /api/v1/audit/products.
func (h *frameworkHandler) listProducts(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	products, err := h.svc.ListProducts(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if products == nil {
		products = []*model.AuditProduct{}
	}
	response.WriteJSONValue(w, http.StatusOK, products)
}

// createProduct handles POST /api/v1/audit/products.
func (h *frameworkHandler) createProduct(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	var req model.CreateProductRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	p, err := h.svc.CreateProduct(r.Context(), req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, p)
}

// listFrameworkControls handles GET /api/v1/audit/frameworks/{id}/controls.
func (h *frameworkHandler) listFrameworkControls(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	id, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controls, err := h.svc.ListFrameworkControls(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if controls == nil {
		controls = []*model.AuditFrameworkControl{}
	}
	response.WriteJSONValue(w, http.StatusOK, model.FrameworkControlListResponse{
		Controls: controls,
		Total:    len(controls),
	})
}

// createFrameworkControl handles POST /api/v1/audit/frameworks/{id}/controls.
func (h *frameworkHandler) createFrameworkControl(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	id, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	var req model.CreateFrameworkControlRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	fc, err := h.svc.CreateFrameworkControl(r.Context(), id, req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, fc)
}
