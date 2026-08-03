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
	"context"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleAssessRisk serves POST /api/v1/risks/{id}/assess.
// Records a residual risk assessment (likelihood, impact, progress, reassessment_date)
// stored in the risk_assessment table. This is separate from "Submit for Approval".
func (d *Deps) handleAssessRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.AssessRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}

	var req model.CreateAssessmentRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	result, err := d.Assessment.Create(r.Context(), id, req, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// The reassessment's own row already records the score; the history entry
	// is what places it in the risk's timeline alongside everything else.
	details := model.HistoryDetails{Level: result.ResidualLevel}
	if prev := previousLevel(r.Context(), d, id, result.ID); prev != "" {
		details.PreviousLevel = prev
	}
	d.recordEvent(r.Context(), id, by, model.HistoryAssess, details)
	response.WriteJSONValue(w, http.StatusCreated, result)
}

// previousLevel returns the residual level immediately before assessmentID, so
// a reassessment can render as "HIGH → MEDIUM" rather than just its new level.
// Returns "" when this is the first reassessment or the lookup fails — the
// history entry then simply shows the new level on its own.
func previousLevel(ctx context.Context, d *Deps, riskID, assessmentID int) string {
	prior, err := d.Assessment.ListByRiskID(ctx, riskID)
	if err != nil {
		return ""
	}
	// ListByRiskID is newest-first and already includes the one just created,
	// so the previous level is the next entry along.
	for i, a := range prior {
		if a.ID == assessmentID && i+1 < len(prior) {
			return prior[i+1].ResidualLevel
		}
	}
	return ""
}
