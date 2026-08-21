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
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	// Reassessment has no per-risk identity gate, so this scoped check is
	// the whole authorisation. It is also the one write action authorised by
	// the grant axis alone — worth remembering when adding another.
	//
	// Checked against both team dimensions: RISK_ASSESS is held by
	// grc-platform-risk-assigner (scope_basis SOURCE_REGISTER — they raised
	// it) AND grc-platform-risk-owner (scope_basis ASSIGNMENT_TEAM — it was
	// routed to them), so either alone must be enough. See
	// auth.HasPrivilegeInEither.
	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeInEither(r.Context(), w, privilege.AssessRisk, detail.SourceRegisterID, detail.AssignmentTeamID) {
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
	// Progress carries the assessor's notes into the timeline itself — the
	// reassessment's own row is the source of truth, but the dedicated
	// Assessment History view that used to read it directly was replaced by
	// this unified timeline, so the notes need to travel here too or they're
	// fetched and never shown anywhere. Level/PreviousLevel already give the
	// score's before/after, and CreatedBy (set by recordEvent below) already
	// gives who — Comment is the one piece genuinely missing.
	details := model.HistoryDetails{Level: result.ResidualLevel, Comment: result.Progress}
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
