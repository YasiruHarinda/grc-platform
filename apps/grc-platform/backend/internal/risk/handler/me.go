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

// riskInvolvementResponse answers "does this caller reach the Risk Hub through
// the identity axis alone" — i.e. with no grant at all. Today that means being
// named as an action plan's action_owner_id on at least one risk.
type riskInvolvementResponse struct {
	NamedOnRisk bool `json:"namedOnRisk"`
}

// handleMyRiskInvolvement serves GET /api/v1/risks/me/involvement.
//
// The frontend nav hides the whole Risk Hub section from anyone holding no risk
// privilege (see modules/risk/nav.ts). But an Action Owner may be any employee,
// holding no role anywhere, and must still see the Registers tab for the risk
// they were handed. This endpoint is that missing signal: useRiskPrivileges
// ORs it in as a synthetic RISK_VIEW_RISKS so the section — and only the
// Registers item within it — becomes visible.
//
// Deliberately no privilege gate: gating on a privilege the caller cannot hold
// would defeat the purpose. Authentication is the only requirement.
//
// Implemented as a limit-1 list rather than a bespoke query: this is exactly
// the scoping handleListRisks' holdsNoGrants branch already applies, so "the
// section is visible" stays equivalent to "the Registers list is non-empty".
func (d *Deps) handleMyRiskInvolvement(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireCallerUUID(w, r); !ok {
		return
	}

	callerID, err := d.callerUserID(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	resp := riskInvolvementResponse{}
	// No platform user row means they cannot be any plan's action_owner_id, so
	// the honest answer is false without touching the risk store.
	if callerID != nil {
		page, err := d.Risk.List(r.Context(), model.ListRisksFilter{
			ActionOwnerID: callerID,
			Limit:         1,
		})
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		resp.NamedOnRisk = page.Total > 0
	}

	response.WriteJSONValue(w, http.StatusOK, resp)
}
