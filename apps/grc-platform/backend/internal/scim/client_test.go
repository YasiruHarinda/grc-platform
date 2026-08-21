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

package scim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListUsersByDomain_ShortPage exercises the case the review comment
// flagged: a non-final page that returns fewer records than the requested
// itemsPerPage (a gateway cap, or anything else short of the request). The
// mock only serves the exact startIndex the walk should ask for next — if
// ListUsersByDomain still advanced by the fixed usersPageSize instead of the
// page it actually got back, it would request an index this server never
// expects, and the walk would come back short.
func TestListUsersByDomain_ShortPage(t *testing.T) {
	const total = 130
	const firstPageLen = 60 // short: usersPageSize is requested as 100

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "test-token", ExpiresIn: 3600})
	}))
	defer tokenSrv.Close()

	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in userSearchInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("decode search request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var resources []scimUser
		switch in.StartIndex {
		case 1:
			resources = fakeUsers(1, firstPageLen)
		case 1 + firstPageLen:
			resources = fakeUsers(1+firstPageLen, total-firstPageLen)
		default:
			t.Errorf("unexpected startIndex %d — walk skipped or repeated a range", in.StartIndex)
			http.Error(w, "unexpected startIndex", http.StatusBadRequest)
			return
		}

		json.NewEncoder(w).Encode(userSearchResult{TotalResults: total, Resources: resources})
	}))
	defer searchSrv.Close()

	c := NewClient(searchSrv.URL, tokenSrv.URL, "id", "secret", "org_internal:users:read")

	got, err := c.ListUsersByDomain(context.Background(), "wso2.com")
	if err != nil {
		t.Fatalf("ListUsersByDomain: %v", err)
	}
	if len(got) != total {
		t.Fatalf("got %d users, want %d (pagination walk skipped records)", len(got), total)
	}

	seen := make(map[string]bool, total)
	for _, u := range got {
		seen[u.Email] = true
	}
	for i := 1; i <= total; i++ {
		email := fmt.Sprintf("user%d@wso2.com", i)
		if !seen[email] {
			t.Errorf("missing %s — pagination walk dropped this record", email)
		}
	}
}

func fakeUsers(startIndex, n int) []scimUser {
	users := make([]scimUser, n)
	for i := range users {
		idx := startIndex + i
		users[i] = scimUser{
			ID:       fmt.Sprintf("uuid-%d", idx),
			UserName: fmt.Sprintf("user%d@wso2.com", idx),
		}
	}
	return users
}
