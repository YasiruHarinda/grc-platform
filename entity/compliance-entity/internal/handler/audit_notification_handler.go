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

// CheckAuditNotificationExists handles POST /audit/notifications/exists — the
// reminder job's de-dup check. POST-with-body (rather than GET-with-query)
// since it has five filter fields, two of them nullable ints.
func (h *AuditNotificationHandler) CheckAuditNotificationExists(w http.ResponseWriter, r *http.Request) {
	var req domain.AuditNotificationExistsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	exists, err := h.svc.AuditNotificationExists(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(domain.AuditNotificationExistsResponse{Exists: exists})
}
