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

	"github.com/wso2-open-operations/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
)

// handleListTeams serves GET /api/v1/teams.
// Optional ?type=SOURCE_REGISTER or ?type=ASSIGNMENT — semantic filter, BOTH teams
// appear in both result sets.
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
	response.WriteJSONValue(w, http.StatusOK, teams)
}
