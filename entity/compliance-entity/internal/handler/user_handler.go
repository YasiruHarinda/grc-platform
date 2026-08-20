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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// UserHandler handles /users routes.
type UserHandler struct {
	svc service.UserService
	// grantSvc embeds grants into SearchUsers when IncludeGrants is set (the
	// Admin Console's user list). Composed here rather than added as a
	// UserService dependency: grants are deliberately never cached (see
	// GrantService's doc comment), and userSvc IS cached — pulling grants
	// through it would either break that guarantee or require punching a
	// cache-bypass hole through it just for this one field.
	grantSvc service.GrantService
}

// NewUserHandler constructs a UserHandler.
func NewUserHandler(svc service.UserService, grantSvc service.GrantService) *UserHandler {
	return &UserHandler{svc: svc, grantSvc: grantSvc}
}

// SearchUsers handles POST /users/search.
func (h *UserHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchUsersRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchUsers(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if req.IncludeGrants && len(resp.Users) > 0 {
		ids := make([]int, len(resp.Users))
		for i, u := range resp.Users {
			ids[i] = u.ID
		}
		grantsByUser, err := h.grantSvc.GrantsForUserIDs(r.Context(), ids)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		for i := range resp.Users {
			resp.Users[i].Grants = grantsByUser[resp.Users[i].ID]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetUserByID handles GET /users/{id}.
func (h *UserHandler) GetUserByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	user, err := h.svc.GetUserByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// GetUserByEmail handles GET /users/by-email/{email}.
func (h *UserHandler) GetUserByEmail(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	user, err := h.svc.GetUserByEmail(r.Context(), email)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// GetUserByUUID handles GET /users/by-uuid/{uuid}.
func (h *UserHandler) GetUserByUUID(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	user, err := h.svc.GetUserByUUID(r.Context(), uuid)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// CreateUser handles POST /users.
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateUserRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	user, err := h.svc.CreateUser(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

// UpdateUser handles PATCH /users/{id}.
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateUserRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	user, err := h.svc.UpdateUser(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}
