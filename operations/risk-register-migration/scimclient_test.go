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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// scimStub is an httptest handler standing in for Asgardeo: one token endpoint
// and one Users/.search endpoint, with the search results paged.
type scimStub struct {
	t          *testing.T
	org        string
	tokenHits  int
	searchHits int
	// pages maps a 1-indexed startIndex to the Resources JSON array for that page.
	pages map[int]string
	total int
}

func (s *scimStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/t/"+s.org+"/oauth2/token":
			s.tokenHits++
			if err := r.ParseForm(); err != nil {
				s.t.Fatalf("parse token form: %v", err)
			}
			if got := r.PostForm.Get("grant_type"); got != "client_credentials" {
				s.t.Errorf("grant_type = %q", got)
			}
			if got := r.PostForm.Get("scope"); got != "internal_user_mgt_list" {
				s.t.Errorf("scope = %q", got)
			}
			if r.PostForm.Get("tokenBindingId") == "" {
				s.t.Error("tokenBindingId not sent")
			}
			user, pass, ok := r.BasicAuth()
			if !ok || user != "cid" || pass != "csecret" {
				s.t.Errorf("basic auth = (%q, %q, %v)", user, pass, ok)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))

		case r.Method == http.MethodPost && r.URL.Path == "/t/"+s.org+"/scim2/Users/.search":
			s.searchHits++
			if got := r.Header.Get("Authorization"); got != "Bearer tok-abc" {
				s.t.Errorf("Authorization = %q", got)
			}
			var in scimUserSearchInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				s.t.Fatalf("decode search body: %v", err)
			}
			if in.Filter != `userName ew "@wso2.com"` {
				s.t.Errorf("filter = %q", in.Filter)
			}
			if in.Domain != "DEFAULT" {
				s.t.Errorf("domain = %q", in.Domain)
			}
			resources, ok := s.pages[in.StartIndex]
			if !ok {
				resources = "[]"
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"totalResults":%d,"Resources":%s}`, s.total, resources)

		default:
			s.t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}
}

func newTestSCIMClient(t *testing.T, stub *scimStub) *SCIMClient {
	t.Helper()
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	return NewSCIMClient(
		srv.URL, srv.URL+"/t/"+stub.org+"/oauth2/token",
		"cid", "csecret", "internal_user_mgt_list", stub.org, 5*time.Second,
	)
}

func TestSCIMListUsersByDomain_PagesAndMaps(t *testing.T) {
	stub := &scimStub{
		t:     t,
		org:   "wso2",
		total: 3,
		pages: map[int]string{
			1: `[
				{"id":"uuid-1","userName":"DEFAULT/user1@wso2.com","name":{"givenName":"User","familyName":"One"}},
				{"id":"uuid-2","userName":"user2@wso2.com","name":{"givenName":"User","familyName":"Two"}}
			]`,
			3: `[
				{"id":"uuid-3","userName":"user3@wso2.com","name":{"givenName":"UserThree","familyName":""}}
			]`,
		},
	}
	c := newTestSCIMClient(t, stub)

	users, err := c.ListUsersByDomain(context.Background(), "wso2.com")
	if err != nil {
		t.Fatalf("ListUsersByDomain: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("got %d users, want 3: %+v", len(users), users)
	}
	if users[0].UUID != "uuid-1" || users[0].Email != "user1@wso2.com" || users[0].DisplayName != "User One" {
		t.Errorf("user[0] = %+v (userstore prefix should be stripped)", users[0])
	}
	if users[2].UUID != "uuid-3" || users[2].DisplayName != "UserThree" {
		t.Errorf("user[2] = %+v (name join should tolerate a missing family name)", users[2])
	}
	if stub.searchHits != 2 {
		t.Errorf("searchHits = %d, want 2 (page at index 1, page at index 3)", stub.searchHits)
	}
	if stub.tokenHits != 1 {
		t.Errorf("tokenHits = %d, want 1 (token cached across pages)", stub.tokenHits)
	}
}

func TestSCIMListUsersByDomain_SinglePageStops(t *testing.T) {
	stub := &scimStub{
		t:     t,
		org:   "wso2",
		total: 1,
		pages: map[int]string{
			1: `[{"id":"u","userName":"user4@wso2.com","name":{"givenName":"User","familyName":"Four"}}]`,
		},
	}
	c := newTestSCIMClient(t, stub)

	users, err := c.ListUsersByDomain(context.Background(), "wso2.com")
	if err != nil {
		t.Fatalf("ListUsersByDomain: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("got %d, want 1", len(users))
	}
	if stub.searchHits != 1 {
		t.Errorf("searchHits = %d, want 1 (startIndex+len > total ends the walk)", stub.searchHits)
	}
}

func TestSCIMToken_NonOKIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	t.Cleanup(srv.Close)
	c := NewSCIMClient(srv.URL, srv.URL+"/token", "cid", "bad", "s", "wso2", 5*time.Second)

	_, err := c.ListUsersByDomain(context.Background(), "wso2.com")
	if err == nil || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("want an error carrying the token body, got %v", err)
	}
}
