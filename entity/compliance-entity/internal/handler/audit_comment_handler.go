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

// CommentHandler handles evidence comment routes.
type CommentHandler struct{ svc service.CommentService }

// NewCommentHandler constructs a CommentHandler.
func NewCommentHandler(svc service.CommentService) *CommentHandler { return &CommentHandler{svc: svc} }

// CreateComment handles POST /audits/{auditId}/controls/{controlId}/comments.
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	var req domain.CreateAuditCommentRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	c, err := h.svc.CreateComment(r.Context(), controlID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(c)
}

// ListComments handles GET /audits/{auditId}/controls/{controlId}/comments.
func (h *CommentHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListCommentsByControl(r.Context(), controlID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteComment handles DELETE /comments/{commentId}.
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := strconv.Atoi(r.PathValue("commentId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "commentId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteComment(r.Context(), commentID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
