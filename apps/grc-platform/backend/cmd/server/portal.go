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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// portalPrefixDispatch routes every /api/v1/evidence-portal/ request through
// portalChain and everything else through base. A dot-segment in the portal
// prefix is rejected outright rather than left to ServeMux's path-cleaning
// redirect (net/http does not clean r.URL.Path).
func portalPrefixDispatch(portalChain, base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/evidence-portal/") {
			if strings.Contains(r.URL.Path, "..") {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			portalChain.ServeHTTP(w, r)
			return
		}
		base.ServeHTTP(w, r)
	})
}

// teamLister is the slice of the team repository resolvePortalClients needs.
type teamLister interface {
	ListAll(ctx context.Context) ([]*model.AuditTeam, error)
}

// resolvePortalClients maps each configured client_id's team NAME to its
// audit_team.id, matching trimmed and case-insensitively against the live team
// list. Zero matches, ambiguity, or an unreachable team service refuse the boot.
func resolvePortalClients(ctx context.Context, teams teamLister, clients map[string]string) (map[string]int, error) {
	all, err := teams.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list audit teams: %w", err)
	}

	resolved := make(map[string]int, len(clients))
	for clientID, name := range clients {
		want := strings.ToLower(strings.TrimSpace(name))
		var matched []*model.AuditTeam
		for _, t := range all {
			if strings.ToLower(strings.TrimSpace(t.Name)) == want {
				matched = append(matched, t)
			}
		}
		switch len(matched) {
		case 1:
			resolved[clientID] = matched[0].ID
			// Format fixed by the config handover doc — DigiOps greps for this
			// exact line to confirm the mapping after a deploy.
			slog.Info(fmt.Sprintf("portal client %s -> team %d (%q)", clientID, matched[0].ID, matched[0].Name))
		case 0:
			return nil, fmt.Errorf("portal client %q: no audit team named %q", clientID, name)
		default:
			ids := make([]int, 0, len(matched))
			for _, t := range matched {
				ids = append(ids, t.ID)
			}
			return nil, fmt.Errorf("portal client %q: audit team name %q is ambiguous (ids %v)", clientID, name, ids)
		}
	}
	return resolved, nil
}
