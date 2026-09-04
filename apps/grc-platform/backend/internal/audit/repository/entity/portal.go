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

package entity

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type portalControlReader struct{ c *entityclient.Client }

// NewPortalControlReader returns an entity-backed PortalControlReader. Both of
// its reads are served by the entity's existing global control search — the
// same endpoint ListAllForReminders and InScope already reuse — so no bespoke
// entity route is needed.
func NewPortalControlReader(c *entityclient.Client) repository.PortalControlReader {
	return &portalControlReader{c: c}
}

// TeamControls pages the global control search filtered to one team and a
// status set, with an optional owner-id narrowing. The entity validates each
// statusKey against the control-status enum and rejects an unknown one before
// it reaches SQL, so a typo is a 4xx rather than a silently short list.
func (r *portalControlReader) TeamControls(ctx context.Context, teamID int, statuses []string, ownerIDs []int) ([]*model.AuditControl, error) {
	var all []*model.AuditControl
	for offset := 0; ; offset += pageLimit {
		body := map[string]any{
			"statusKeys": statuses,
			"teamIds":    []int{teamID},
			"ownerIds":   ownerIDs,
			"scope":      model.ScopeAll,
			"pagination": map[string]int{"limit": pageLimit, "offset": offset},
		}
		var resp struct {
			Controls []*model.AuditControl `json:"controls"`
		}
		if err := r.c.Post(ctx, "/controls/search", body, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Controls...)
		if len(resp.Controls) < pageLimit {
			return all, nil
		}
	}
}

// ControlByID fetches one control by id alone (the portal path carries no
// audit id) via the same global search, so control.AuditID comes back
// populated. Returns (nil, nil) when the id matches nothing.
func (r *portalControlReader) ControlByID(ctx context.Context, controlID int) (*model.AuditControl, error) {
	body := map[string]any{
		"controlIds": []int{controlID},
		"scope":      model.ScopeAll,
		"pagination": map[string]int{"limit": 1, "offset": 0},
	}
	var resp struct {
		Controls []*model.AuditControl `json:"controls"`
	}
	if err := r.c.Post(ctx, "/controls/search", body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Controls) == 0 {
		return nil, nil
	}
	return resp.Controls[0], nil
}
