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

//go:build live

// This file only compiles with `-tags live` — never part of `go test ./...`,
// CI, or the Choreo build (which does a plain `go build .`). It hits a real
// Asgardeo organization, so it needs real credentials and network access.
package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveSCIM_ListUsersByDomain exercises only hop 1 of identity resolution
// (email -> uuid, see resolve.go) against a real Asgardeo org — no
// compliance-entity, no MySQL, no CSV. Set every var below, then:
//
//	SCIM_BASE_URL=https://api.asgardeo.io \
//	SCIM_INTERNAL_ORG=<your-org> \
//	SCIM_INTERNAL_CLIENT_ID=<id> \
//	SCIM_INTERNAL_CLIENT_SECRET=<secret> \
//	SCIM_INTERNAL_SCOPES="internal_user_mgt_view internal_user_mgt_list" \
//	SCIM_DOMAIN=<the email domain your org's users actually carry> \
//	go test -tags live -run TestLiveSCIM_ListUsersByDomain -v .
func TestLiveSCIM_ListUsersByDomain(t *testing.T) {
	base := os.Getenv("SCIM_BASE_URL")
	org := os.Getenv("SCIM_INTERNAL_ORG")
	clientID := os.Getenv("SCIM_INTERNAL_CLIENT_ID")
	clientSecret := os.Getenv("SCIM_INTERNAL_CLIENT_SECRET")
	scopes := os.Getenv("SCIM_INTERNAL_SCOPES")
	domain := os.Getenv("SCIM_DOMAIN")
	if base == "" || org == "" || clientID == "" || clientSecret == "" || domain == "" {
		t.Skip("set SCIM_BASE_URL, SCIM_INTERNAL_ORG, SCIM_INTERNAL_CLIENT_ID, " +
			"SCIM_INTERNAL_CLIENT_SECRET and SCIM_DOMAIN to run this live test")
	}

	// Same derivation loadConfig does: {base}/t/{org}/oauth2/token.
	tokenURL := base + "/t/" + org + "/oauth2/token"

	c := NewSCIMClient(base, tokenURL, clientID, clientSecret, scopes, org, 30*time.Second)

	users, err := c.ListUsersByDomain(context.Background(), domain)
	if err != nil {
		t.Fatalf("ListUsersByDomain(%q): %v", domain, err)
	}
	if len(users) == 0 {
		t.Fatalf("0 users for domain %q — check the domain matches your org's actual "+
			"userName suffix, and that the app's SCIM2 Users API authorization includes "+
			"internal_user_mgt_view and internal_user_mgt_list", domain)
	}

	// Log only a small sample — the directory can be large, and the full dump
	// would bury the result and spill the whole corporate listing into terminal
	// scrollback / captured output.
	const sample = 5
	t.Logf("resolved %d user(s) for @%s (showing up to %d):", len(users), domain, sample)
	for i, u := range users {
		if i == sample {
			break
		}
		t.Logf("  %-30s uuid=%-38s name=%q", u.Email, u.UUID, u.DisplayName)
	}
}
