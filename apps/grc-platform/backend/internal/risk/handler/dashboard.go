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
	"strconv"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleDashboard serves GET /api/v1/risks/dashboard.
// Optional query param register_id scopes the payload to one register.
func (d *Deps) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRiskDashboard) {
		return
	}

	var registerID *int
	if raw := r.URL.Query().Get("register_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			response.WriteError(w, http.StatusBadRequest, "register_id must be a positive integer")
			return
		}
		registerID = &id
	}

	// Scoped to registers where the caller specifically holds ViewRiskDashboard
	// — not seesEveryRisk (RISK_VIEW_RISKS), and not RegisterScopeIDs (any
	// grant at all). A caller can hold different privileges in different
	// registers, so this page's own privilege is what decides which registers
	// contribute to it. A caller with no grants at all gets a zeroed dashboard
	// — the aggregate counterpart of seeing only the risks they are personally
	// named on, which do not aggregate meaningfully. Fails closed like
	// handleListRisks' equivalent scoping: an empty list means "unrestricted"
	// downstream, so it must never reach the query.
	var registerIDs []int
	if !callerGrants(r.Context()).HasGlobal(privilege.ViewRiskDashboard) {
		// Register-capable scopes only: a grant on an ASSIGNMENT-only team (HR,
		// Legal) contributes nothing — there is no register page for it to
		// appear on. A grant on a BOTH team does contribute, which is why
		// "Risk Owner @ Asgardeo" still gets an Asgardeo dashboard while
		// "Risk Owner @ HR" gets none.
		registerIDs = callerGrants(r.Context()).RegisterScopeIDsFor(privilege.ViewRiskDashboard)
		if len(registerIDs) == 0 {
			response.WriteJSONValue(w, http.StatusOK, model.DashboardSummary{
				TreatmentByRegister:     []model.RegisterTreatmentCount{},
				LevelCounts:             []model.RiskLevelCount{},
				OrgHeatmap:              []model.HeatmapCell{},
				CertDistribution:        []model.RegisterCertShare{},
				Registers:               []model.RegisterAnalytics{},
				RepeatedComplianceRisks: []model.RepeatedComplianceRisk{},
				HighRisks:               []model.HighRiskItem{},
			})
			return
		}
	}

	summary, err := d.Dashboard.Summary(r.Context(), registerID, registerIDs)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.enrichDashboard(r.Context(), summary)
	response.WriteJSONValue(w, http.StatusOK, summary)
}
