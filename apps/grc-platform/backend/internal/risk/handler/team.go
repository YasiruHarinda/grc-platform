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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
)

// handleListTeams serves GET /api/v1/teams.
// Optional ?type=SOURCE_REGISTER or ?type=ASSIGNMENT — semantic filter, BOTH teams
// appear in both result sets.
//
// Optional ?mine=true restricts the list to the caller's own risk teams — how
// the Dashboard/Analytics/Registers-list register filters avoid offering a
// register the caller can't see any data for anyway. Only actually narrows
// anything for a team-scoped-only caller (Risk Assigner/Owner); Compliance/
// Management/Admin see everything regardless, so narrowing their own picker
// would be wrong (they may belong to zero teams themselves). AddRisk's
// create-flow register picker never sets this param, so it always sees every
// register — raising a risk under a register you don't belong to is a
// legitimate action this scoping was never meant to restrict.
func (d *Deps) handleListTeams(w http.ResponseWriter, r *http.Request) {
	filter := model.ListTeamsFilter{
		Type: r.URL.Query().Get("type"),
	}

	teams, err := d.Team.List(r.Context(), filter)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	if teams == nil {
		teams = []*model.Team{}
	}

	// ?mine=true narrows the list to registers the caller holds a grant on —
	// register-capable scopes only, since every consumer of this is a register
	// dropdown. A grant on an ASSIGNMENT-only team (HR, Legal) is not a register
	// and never belongs here.
	//
	// A GLOBAL holder is deliberately unaffected: they may hold no team-scoped
	// grant at all, and narrowing them to nothing would empty a dropdown for
	// someone entitled to every entry in it.
	//
	// ?privilege=<NAME> narrows further to registers where the caller holds that
	// privilege. Add Risk passes RISK_CREATE, because "registers I can see" and
	// "registers I may raise a risk in" are different questions — a Risk Owner
	// on a BOTH team can see its dashboard but cannot create there, and offering
	// it in the create picker would produce a choice the server refuses.
	if r.URL.Query().Get("mine") == "true" && !seesEveryRisk(r.Context()) {
		set := callerGrants(r.Context())
		wantPriv := r.URL.Query().Get("privilege")

		mine := map[int]bool{}
		for _, id := range set.RegisterScopeIDs() {
			if wantPriv == "" || auth.HasPrivilegeIn(r.Context(), wantPriv, id) {
				mine[id] = true
			}
		}
		filtered := make([]*model.Team, 0, len(teams))
		for _, t := range teams {
			if mine[t.ID] {
				filtered = append(filtered, t)
			}
		}
		teams = filtered
	}

	response.WriteJSONValue(w, http.StatusOK, teams)
}
