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
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// recordHistory appends one entry to a risk's history.
//
// Best-effort by design, matching the Audit Hub's recordEvidenceTrail: the
// action being recorded has already succeeded and been committed, so a failure
// to write history must never surface to the caller or undo their work. It is
// logged loudly instead, because a silently short history is the failure mode
// that matters here.
//
// Synchronous rather than detached: this is a single call to the Compliance
// Entity, which the handler is already talking to — unlike the email path,
// there is no cold-starting third party to wait out.
func (d *Deps) recordHistory(ctx context.Context, riskID int, by string, req model.RecordHistoryRequest) {
	if d.History == nil {
		return
	}
	if err := d.History.Record(ctx, riskID, req, by); err != nil {
		slog.WarnContext(ctx, "risk history: record failed",
			"riskId", riskID, "action", req.Action, "err", err)
	}
}

// recordEvent is recordHistory for a workflow event — the common case, where
// there is no field diff and everything interesting is in the details.
func (d *Deps) recordEvent(ctx context.Context, riskID int, by, action string, details model.HistoryDetails) {
	d.recordHistory(ctx, riskID, by, model.RecordHistoryRequest{Action: action, Details: &details})
}

// handleListRiskHistory serves GET /api/v1/risks/{id}/history — the full
// chronological record behind the drawer's History tab.
//
// Visible to anyone who can view the risk, scoped the same way every other
// risk-detail endpoint is: history is a view of actions already visible on the
// risk, so it needs no narrower rule of its own.
func (d *Deps) handleListRiskHistory(w http.ResponseWriter, r *http.Request) {
	// Unscoped on purpose: this gates only whether the caller may read risks at
	// all. WHICH risks they may read is decided by riskVisibleToCaller / the
	// list scoping, not by this privilege.
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
	entries, err := d.History.List(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.enrichHistory(r.Context(), entries)
	response.WriteJSONValue(w, http.StatusOK, entries)
}
