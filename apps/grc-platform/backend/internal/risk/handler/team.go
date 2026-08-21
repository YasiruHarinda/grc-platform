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

// handleListTeams serves GET /api/v1/teams.
// Optional ?type=SOURCE_REGISTER or ?type=ASSIGNMENT — semantic filter, BOTH teams
// appear in both result sets.
//
// Optional ?includeInactive=true returns every status, not just ACTIVE — the
// Admin Console's Risk Teams table is the only caller that sets this; every
// picker leaves it off.
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
		Type:            r.URL.Query().Get("type"),
		IncludeInactive: r.URL.Query().Get("includeInactive") == "true",
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
	// A GLOBAL ViewRisks holder is deliberately unaffected by ?mine=true: they
	// may hold no team-scoped grant at all, and narrowing them to nothing would
	// empty a dropdown for someone entitled to every entry in it.
	//
	// ?privilege=<NAME> narrows further to registers where the caller holds that
	// privilege — applied unconditionally, never short-circuited by
	// seesEveryRisk. Add Risk passes RISK_CREATE, because "registers I can see"
	// and "registers I may raise a risk in" are different questions answered by
	// different privileges (ViewRisks vs. RISK_CREATE): a Management approver
	// sees every risk globally but may hold RISK_CREATE on only one register
	// (or none), and offering the rest in the create picker would produce
	// choices the server refuses.
	mineOnly := r.URL.Query().Get("mine") == "true"
	wantPriv := r.URL.Query().Get("privilege")
	if mineOnly || wantPriv != "" {
		set := callerGrants(r.Context())
		visibleEverywhere := seesEveryRisk(r.Context())

		registerScoped := map[int]bool{}
		for _, id := range set.RegisterScopeIDs() {
			registerScoped[id] = true
		}

		filtered := make([]*model.Team, 0, len(teams))
		for _, t := range teams {
			if mineOnly && !visibleEverywhere && !registerScoped[t.ID] {
				continue
			}
			if wantPriv != "" && !auth.HasPrivilegeIn(r.Context(), wantPriv, t.ID) {
				continue
			}
			filtered = append(filtered, t)
		}
		teams = filtered
	}

	response.WriteJSONValue(w, http.StatusOK, teams)
}

// handleCreateTeam serves POST /api/v1/teams — the Admin Console's Risk
// Teams "Add Team" dialog. Gated on MANAGE_RISK_HUB, not MANAGE_USERS: this
// is reference-data management, a separate authorisation boundary from user
// provisioning even though only grc-platform-admin holds either today.
func (d *Deps) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	var req model.CreateTeamRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Name == "" {
		response.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TeamType == "" {
		response.WriteError(w, http.StatusBadRequest, "team_type is required")
		return
	}

	createdBy := ""
	if user := auth.FromContext(r.Context()); user != nil {
		createdBy = user.Subject
	}

	t, err := d.Team.Create(r.Context(), req, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, t)
}

// handleUpdateTeam serves PUT /api/v1/teams/{id}.
func (d *Deps) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	var req model.UpdateTeamRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	// UpdateTeamRequest.Name is a plain string, not a pointer — an omitted or
	// empty name decodes to "" and the entity repository forwards it
	// unconditionally (it doesn't support partial updates), so without this
	// check a caller could blank out a team's name. Same guard as
	// handleCreateTeam.
	if req.Name == "" {
		response.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	updatedBy := ""
	if user := auth.FromContext(r.Context()); user != nil {
		updatedBy = user.Subject
	}

	if err := d.Team.Update(r.Context(), id, req, updatedBy); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
