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

	if r.URL.Query().Get("mine") == "true" && isTeamScopedOnly(r.Context()) {
		email, ok := requireUserEmail(w, r)
		if !ok {
			return
		}
		caller, err := d.Users.GetByEmail(r.Context(), email)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		myTeamIDs := map[int]bool{}
		if caller != nil {
			for _, id := range caller.RiskTeamIDs {
				myTeamIDs[id] = true
			}
		}
		filtered := make([]*model.Team, 0, len(teams))
		for _, t := range teams {
			if myTeamIDs[t.ID] {
				filtered = append(filtered, t)
			}
		}
		teams = filtered
	}

	response.WriteJSONValue(w, http.StatusOK, teams)
}
