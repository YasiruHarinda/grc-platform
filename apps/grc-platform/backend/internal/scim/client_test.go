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
	"time"
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

	c := NewClient(searchSrv.URL, tokenSrv.URL, "id", "secret", "internal_user_mgt_view internal_user_mgt_list", "wso2")

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

// TestNewExternalClient_HitsExternalOrgPath confirms NewExternalClient's
// requests target the external org's own tenant path
// (/t/wso2external/scim2/...), not the internal org's — the two orgs hold
// genuinely distinct identities (see the design doc's §1.4), so a client
// wired to the wrong path would silently never resolve anyone, or worse,
// resolve the wrong org's uuid space.
func TestNewExternalClient_HitsExternalOrgPath(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(tokenResponse{AccessToken: "test-token", ExpiresIn: 3600}); err != nil {
			t.Errorf("encode token response: %v", err)
		}
	}))
	defer tokenSrv.Close()

	var gotPath string
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewEncoder(w).Encode(userSearchResult{
			TotalResults: 1,
			Resources:    []scimUser{{ID: "ext-uuid-1", UserName: "auditor@external.example"}},
		}); err != nil {
			t.Errorf("encode user search response: %v", err)
		}
	}))
	defer searchSrv.Close()

	c := NewExternalClient(searchSrv.URL, tokenSrv.URL, "id", "secret",
		"internal_user_mgt_view internal_user_mgt_list", "wso2external")

	got, err := c.LookupByUUID(context.Background(), "ext-uuid-1")
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if got == nil || got.UUID != "ext-uuid-1" {
		t.Fatalf("got %+v, want a resolved external user", got)
	}
	if gotPath != "/t/wso2external/scim2/Users/.search" {
		t.Errorf("hit path %q, want /t/wso2external/scim2/Users/.search", gotPath)
	}
}

// TestNewClient_StillHitsInternalOrgPath guards the refactor that introduced
// the org field: NewClient's existing callers (main.go, the backfill tools)
// must keep resolving against the internal org's own tenant path, unchanged.
func TestNewClient_StillHitsInternalOrgPath(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "test-token", ExpiresIn: 3600})
	}))
	defer tokenSrv.Close()

	var gotPath string
	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(userSearchResult{Resources: []scimUser{}})
	}))
	defer searchSrv.Close()

	c := NewClient(searchSrv.URL, tokenSrv.URL, "id", "secret", "internal_user_mgt_view internal_user_mgt_list", "wso2")
	if _, err := c.LookupByUUID(context.Background(), "some-uuid"); err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if gotPath != "/t/wso2/scim2/Users/.search" {
		t.Errorf("hit path %q, want /t/wso2/scim2/Users/.search", gotPath)
	}
}

// TestAccessToken_SendsStableTokenBindingID confirms every token request
// from one Client instance carries the same tokenBindingId — a fresh random
// value per request would defeat the whole point (see Client.tokenBindingID's
// doc comment): Asgardeo revokes a client's previous token when it issues a
// new one for the same binding, so two requests from the same instance need
// to agree on the binding to avoid invalidating each other.
func TestAccessToken_SendsStableTokenBindingID(t *testing.T) {
	var gotBindingIDs []string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token request form: %v", err)
		}
		gotBindingIDs = append(gotBindingIDs, r.FormValue("tokenBindingId"))
		// ExpiresIn: 0 forces accessToken to skip the cache and re-request
		// every call, so a second accessToken call actually hits this server
		// again instead of reusing the first response.
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "test-token", ExpiresIn: 0})
	}))
	defer tokenSrv.Close()

	c := NewClient("https://unused.example", tokenSrv.URL, "id", "secret",
		"internal_user_mgt_view internal_user_mgt_list", "wso2")

	if _, err := c.accessToken(context.Background()); err != nil {
		t.Fatalf("accessToken (1st): %v", err)
	}
	c.tokenExpiry = time.Time{} // force the second call past the cache too
	if _, err := c.accessToken(context.Background()); err != nil {
		t.Fatalf("accessToken (2nd): %v", err)
	}

	if len(gotBindingIDs) != 2 {
		t.Fatalf("token endpoint hit %d times, want 2", len(gotBindingIDs))
	}
	if gotBindingIDs[0] == "" {
		t.Fatal("tokenBindingId was empty")
	}
	if gotBindingIDs[0] != gotBindingIDs[1] {
		t.Errorf("tokenBindingId changed between requests from the same Client: %q vs %q",
			gotBindingIDs[0], gotBindingIDs[1])
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
