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
	"errors"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

type stubTeams struct {
	teams []*model.AuditTeam
	err   error
}

func (s stubTeams) ListAll(context.Context) ([]*model.AuditTeam, error) {
	return s.teams, s.err
}

func TestResolvePortalClients(t *testing.T) {
	teams := []*model.AuditTeam{
		{ID: 3, Name: "Platform"},
		{ID: 7, Name: "SRE Team"},
	}

	t.Run("single case-insensitive match resolves to the id", func(t *testing.T) {
		got, err := resolvePortalClients(context.Background(), stubTeams{teams: teams},
			map[string]string{"client-1": "  sre team "})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got["client-1"] != 7 {
			t.Fatalf("got %v, want client-1 -> 7", got)
		}
	})

	t.Run("no match refuses the boot", func(t *testing.T) {
		_, err := resolvePortalClients(context.Background(), stubTeams{teams: teams},
			map[string]string{"client-1": "Nonexistent"})
		if err == nil {
			t.Fatal("want error for an unmatched team name")
		}
	})

	t.Run("ambiguous name refuses the boot", func(t *testing.T) {
		dup := append([]*model.AuditTeam{{ID: 9, Name: "SRE Team"}}, teams...)
		_, err := resolvePortalClients(context.Background(), stubTeams{teams: dup},
			map[string]string{"client-1": "SRE Team"})
		if err == nil {
			t.Fatal("want error for a name matching two teams")
		}
	})

	t.Run("team service unreachable refuses the boot", func(t *testing.T) {
		_, err := resolvePortalClients(context.Background(), stubTeams{err: errors.New("boom")},
			map[string]string{"client-1": "SRE Team"})
		if err == nil {
			t.Fatal("want error when the team list cannot be fetched")
		}
	})
}
