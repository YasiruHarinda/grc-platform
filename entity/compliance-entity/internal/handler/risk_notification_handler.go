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

// RiskNotificationHandler handles /notifications and /users/{userId}/notifications routes.
type RiskNotificationHandler struct {
	svc service.RiskNotificationService
}

// NewRiskNotificationHandler constructs a RiskNotificationHandler.
func NewRiskNotificationHandler(svc service.RiskNotificationService) *RiskNotificationHandler {
	return &RiskNotificationHandler{svc: svc}
}

// CreateRiskNotification handles POST /notifications.
func (h *RiskNotificationHandler) CreateRiskNotification(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRiskNotificationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	n, err := h.svc.CreateRiskNotification(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(n)
}

// ListRiskNotifications handles GET /notifications?recipientId=.
func (h *RiskNotificationHandler) ListRiskNotifications(w http.ResponseWriter, r *http.Request) {
	recipientID, err := strconv.Atoi(r.URL.Query().Get("recipientId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "recipientId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskNotifications(r.Context(), recipientID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// MarkRiskNotificationRead handles PATCH /notifications/{id}/read.
func (h *RiskNotificationHandler) MarkRiskNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.MarkRiskNotificationReadRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	n, err := h.svc.MarkRiskNotificationRead(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(n)
}
