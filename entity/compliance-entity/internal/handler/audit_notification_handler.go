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

// AuditNotificationHandler handles /audit/notifications routes.
type AuditNotificationHandler struct {
	svc service.AuditNotificationService
}

// NewAuditNotificationHandler constructs an AuditNotificationHandler.
func NewAuditNotificationHandler(svc service.AuditNotificationService) *AuditNotificationHandler {
	return &AuditNotificationHandler{svc: svc}
}

// CreateAuditNotification handles POST /audit/notifications.
func (h *AuditNotificationHandler) CreateAuditNotification(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAuditNotificationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	n, err := h.svc.CreateAuditNotification(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(n)
}

// ClaimAuditNotification handles POST /audit/notifications/claim — the
// reminder job's atomic de-dup claim. POST-with-body (rather than
// GET-with-query) since it has five filter fields, two of them nullable
// ints. Losing the race (claimed=false) is a normal 200, not an error — see
// docs/new/Reminder-Notification-Atomic-Claim-Design.md.
func (h *AuditNotificationHandler) ClaimAuditNotification(w http.ResponseWriter, r *http.Request) {
	var req domain.ClaimAuditNotificationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	claimed, id, err := h.svc.ClaimAuditNotification(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(domain.ClaimAuditNotificationResponse{Claimed: claimed, ID: id})
}

// ReleaseAuditNotificationClaim handles DELETE
// /audit/notifications/{id}/claim — releases a claim whose send failed, so
// the item is retried on a future run.
func (h *AuditNotificationHandler) ReleaseAuditNotificationClaim(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	if err := h.svc.ReleaseAuditNotificationClaim(r.Context(), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
