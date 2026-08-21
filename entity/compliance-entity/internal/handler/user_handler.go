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
type UserHandler struct{ svc service.UserService }

// NewUserHandler constructs a UserHandler.
func NewUserHandler(svc service.UserService) *UserHandler { return &UserHandler{svc: svc} }

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
