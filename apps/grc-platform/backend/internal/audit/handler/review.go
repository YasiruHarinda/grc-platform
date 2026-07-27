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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// requireAssignedAuditor authorizes population-validation, sample-selection, and
// evidence-validation actions to the control's assigned auditor POC only —
// matched by email against control.AuditorEmail — since those are external-
// auditor decisions, not internal-reviewer ones (design doc §2, §5.4).
//
// ManageControls holders bypass this check, consistent with requireAssignment
// elsewhere: they already have full read/write over all audit data.
//
// The caller must also hold ViewAudits (baseline GRC access) so a valid IdP
// token with zero GRC privileges cannot act purely by an auditor_id coincidence.
// There is no dedicated "external auditor" privilege in the platform today — if
// one is introduced later, swap this baseline check for it.
func requireAssignedAuditor(w http.ResponseWriter, r *http.Request, control *model.AuditControl) bool {
	if auth.HasPrivilege(r.Context(), privilege.ManageControls) {
		return true
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return false
	}
	actor := auth.FromContext(r.Context())
	if control.AuditorEmail == nil || !strings.EqualFold(*control.AuditorEmail, actor.Email) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return false
	}
	return true
}

// decodeReviewDecision reads {"decision":"APPROVE"|"REJECT","comment":"..."} and
// normalizes decision to upper case. Writes a 400 and returns ok=false on a
// missing body or an unrecognized decision value.
func decodeReviewDecision(w http.ResponseWriter, r *http.Request) (model.ReviewDecisionRequest, bool) {
	var req model.ReviewDecisionRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return req, false
	}
	req.Decision = strings.ToUpper(req.Decision)
	if req.Decision != "APPROVE" && req.Decision != "REJECT" {
		response.WriteError(w, http.StatusBadRequest, `decision must be "APPROVE" or "REJECT"`)
		return req, false
	}
	return req, true
}
