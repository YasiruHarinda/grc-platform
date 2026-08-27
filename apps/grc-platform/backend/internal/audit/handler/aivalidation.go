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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type aiValidationHandler struct {
	svc         service.AIValidationService
	evidenceSvc service.EvidenceService
}

// listValidations handles GET /api/v1/evidence/{evidenceId}/ai-validations.
//
// Advisory review hints. Visible to anyone permitted to see the evidence —
// submitters (for the pre-review feedback loop) and reviewers (for the hint) —
// so it reuses SUBMIT_EVIDENCE OR REVIEW_EVIDENCE rather than a new privilege.
// The route carries only a bare evidenceId (no team/control context), so —
// same as requireEvidenceFileAccess for file downloads — the owning
// control's team/auditor must be resolved first and the privileges checked
// scoped to that team (HasPrivilegeIn), not the unscoped HasPrivilege/
// RequireAnyPrivilege: otherwise a grant scoped to one team would let its
// holder read every other team's AI validation results by evidenceId.
func (h *aiValidationHandler) listValidations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	evidenceID, ok := parseIntParam(w, r, "evidenceId")
	if !ok {
		return
	}
	auditorID, evidenceTeamID, err := h.evidenceSvc.EvidenceAuditorID(ctx, evidenceID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	teamID := 0
	if evidenceTeamID != nil {
		teamID = *evidenceTeamID
	}
	if !auth.HasPrivilegeIn(ctx, privilege.ManageControls, teamID) &&
		!auth.HasPrivilegeIn(ctx, privilege.SubmitEvidence, teamID) &&
		!auth.HasPrivilegeIn(ctx, privilege.ReviewEvidence, teamID) &&
		!auth.HasPrivilegeIn(ctx, privilege.ViewAllAudits, teamID) {
		actor := auth.FromContext(ctx)
		if auditorID == nil || *auditorID != actor.UserID {
			response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
			return
		}
	}
	validations, err := h.svc.ListByEvidence(ctx, evidenceID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if validations == nil {
		validations = []*model.AIValidationLog{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.AIValidationListResponse{Validations: validations})
}
