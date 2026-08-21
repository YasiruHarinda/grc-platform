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

// GrantHandler serves role-grant routes — who holds which role, in which scope.
type GrantHandler struct{ svc service.GrantService }

// NewGrantHandler constructs a GrantHandler.
func NewGrantHandler(svc service.GrantService) *GrantHandler { return &GrantHandler{svc: svc} }

// GrantsByEmail handles GET /grants/by-email/{email}.
//
// The hot path: the GRC backend calls this on every authenticated request to
// build the caller's scoped privilege set, so it resolves the user and their
// grants in one round trip.
//
// Responses must not be cached — by this service, by the caller, or by anything
// between them. A revoked grant has to take effect on the caller's very next
// request; that is the entire reason this data does not ride along on the user
// payload, which is cached for five minutes.
func (h *GrantHandler) GrantsByEmail(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GrantsForUserEmail(r.Context(), r.PathValue("email"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// GrantsByUUID handles GET /grants/by-uuid/{uuid}.
//
// The replacement for GrantsByEmail, keyed on the Asgardeo id the caller's token
// already carries. Same contract, same no-store requirement — see GrantsByEmail
// for why a revoked grant must never be served from a cache.
func (h *GrantHandler) GrantsByUUID(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GrantsForUserUUID(r.Context(), r.PathValue("uuid"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// GrantsByUserID handles GET /grants/user/{id}.
func (h *GrantHandler) GrantsByUserID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	resp, err := h.svc.GrantsForUserID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateGrant handles POST /grants/user/{id} — granting a role in a scope.
//
// Re-granting something the user already holds is idempotent: it reactivates
// the existing row and returns 201 either way, rather than 409ing on a request
// whose intent is already satisfied.
func (h *GrantHandler) CreateGrant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.CreateUserGrantRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	grant, err := h.svc.CreateGrant(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(grant)
}

// RevokeGrant handles DELETE /grants/user/{id}/{grantId}?revokedBy=.
//
// Deactivates rather than deletes: who held what, and when it was taken away,
// is precisely the history an authorisation change should leave behind — which
// is why revokedBy is required and recorded in updated_by. The actor rides in a
// query param rather than a body, matching DELETE /audits/{id}?deletedBy=.
func (h *GrantHandler) RevokeGrant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	grantID, err := strconv.Atoi(r.PathValue("grantId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "grantId must be a positive integer"})
		return
	}
	if err := h.svc.RevokeGrant(r.Context(), id, grantID, r.URL.Query().Get("revokedBy")); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Candidates handles GET /grants/candidates?privilege=X&teamId=1&teamId=2 —
// every active user holding privilege X, GLOBAL or scoped to one of the given
// risk teams. Powers the Risk Hub's Risk Owner / Management Approver pickers.
func (h *GrantHandler) Candidates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var teamIDs []int
	for _, raw := range q["teamId"] {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "teamId must be a positive integer"})
			return
		}
		teamIDs = append(teamIDs, id)
	}
	resp, err := h.svc.CandidatesForPrivilege(r.Context(), q.Get("privilege"), teamIDs)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ListRoles handles GET /roles — the role catalogue, for a grant editor.
// Each role carries its module, which determines the scopes it may be granted
// against, so a UI can offer only the valid ones.
func (h *GrantHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.ListRoles(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
