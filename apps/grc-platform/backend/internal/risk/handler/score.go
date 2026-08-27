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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleListRiskScores serves GET /api/v1/risk/scores.
// Returns all 9 cells of the likelihood × impact matrix.
//
// Gated on ViewRisks OR ManageRiskHub — same shape as GET /api/v1/risk/teams and
// GET /api/v1/risk/users (team.go, users.go). Unlike those, this route has no
// write counterpart to compare against (routes.go: "read-only by design...
// No write route exists"), so it was simply ungated outright, letting any
// authenticated caller read the score matrix.
func (d *Deps) handleListRiskScores(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.ViewRisks, privilege.ManageRiskHub) {
		return
	}

	scores, err := d.Score.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	if scores == nil {
		scores = []*model.RiskScore{}
	}
	response.WriteJSONValue(w, http.StatusOK, scores)
}
