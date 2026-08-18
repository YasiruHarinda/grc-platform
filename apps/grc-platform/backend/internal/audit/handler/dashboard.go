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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type dashboardHandler struct {
	svc auditservice.DashboardService
}

func (h *dashboardHandler) getDashboard(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	user := auth.FromContext(r.Context())

	f := model.DashboardFilter{}
	if user != nil {
		f.Roles = user.Roles
		f.UserEmail = user.Email
	}

	data, err := h.svc.Get(r.Context(), f)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to load dashboard data.")
		return
	}

	response.WriteJSONValue(w, http.StatusOK, data)
}

func (h *dashboardHandler) getWorkQueue(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	user := auth.FromContext(r.Context())

	f := model.DashboardFilter{}
	if user != nil {
		f.Roles = user.Roles
		f.UserEmail = user.Email
	}

	q := r.URL.Query()
	tab := model.WorkQueueTab(q.Get("tab"))
	if tab == "" {
		tab = model.WorkQueueTabActionItems
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	for _, v := range q["teamIds"] {
		if id, err := strconv.Atoi(v); err == nil {
			f.TeamIDs = append(f.TeamIDs, id)
		}
	}
	for _, v := range q["ownerIds"] {
		if id, err := strconv.Atoi(v); err == nil {
			f.OwnerIDs = append(f.OwnerIDs, id)
		}
	}
	for _, v := range q["auditIds"] {
		if id, err := strconv.Atoi(v); err == nil {
			f.AuditIDs = append(f.AuditIDs, id)
		}
	}
	for _, v := range q["statuses"] {
		if v != "" {
			f.Statuses = append(f.Statuses, v)
		}
	}
	f.ControlNumber = strings.TrimSpace(q.Get("controlNumber"))
	f.DueSortDesc = q.Get("dueSort") == "desc"

	p, err := h.svc.GetWorkQueuePage(r.Context(), f, tab, page, limit)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusOK, p)
}
