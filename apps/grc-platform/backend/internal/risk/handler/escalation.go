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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleEscalateRisk serves POST /api/v1/risks/{id}/escalate — the manual
// trigger for Compliance/Admin to escalate an overdue IN_REMEDIATION risk on
// demand, rather than waiting for the daily job (up to 24h delay) to reach
// it. Same outcome as the automatic path: OPEN escalation created, risk
// flips to ESCALATED. The entity re-validates IN_REMEDIATION + overdue, so a
// risk that's already moved on (e.g. someone just closed it, or the job beat
// this click to it) returns a clear 4xx rather than being escalated wrongly.
func (d *Deps) handleEscalateRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.EscalateRisk) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	escalation, err := d.Escalation.Escalate(r.Context(), riskID, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, escalation)
}

// handleListEscalations serves GET /api/v1/risks/{id}/escalations. Visible to
// anyone who can view the risk — escalation history is system-generated (see
// model.Escalation) and shown the same as any other risk field — except an
// Action-Owner-only caller, who is further scoped to risks where they own a
// plan (riskVisibleToCaller), matching handleListRisks' list scoping.
func (d *Deps) handleListEscalations(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	visible, err := d.riskVisibleToCaller(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !visible {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	escalations, err := d.Escalation.List(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, escalations)
}
