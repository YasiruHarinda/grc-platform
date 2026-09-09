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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// entityTestServer wires an EntityClient to a one-shot httptest handler.
func entityTestServer(t *testing.T, h http.HandlerFunc) *EntityClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewEntityClient(srv.URL, 5*time.Second)
}

func TestEntityClient_GetUnwrapsAndDecodes(t *testing.T) {
	ec := entityTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/roles" {
			t.Errorf("got %s %s, want GET /roles", r.Method, r.URL.Path)
		}
		if acc := r.Header.Get("Accept"); acc != "application/json" {
			t.Errorf("Accept = %q", acc)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"roles":[
			{"id":7,"roleName":"grc-platform-risk-owner","module":"RISK","status":"ACTIVE"},
			{"id":8,"roleName":"grc-platform-risk-assigner","module":"RISK","status":"ACTIVE"}
		]}`))
	})

	roles, err := ec.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) != 2 || roles[0].ID != 7 || roles[0].RoleName != "grc-platform-risk-owner" {
		t.Fatalf("roles = %+v", roles)
	}
}

func TestEntityClient_PostSendsJSONBodyAndDecodesResult(t *testing.T) {
	ec := entityTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users" {
			t.Errorf("got %s %s, want POST /users", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		var req CreateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if req.UUID != "abc-123" || req.UserType != "INTERNAL" || req.CreatedBy != marker {
			t.Errorf("body = %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":42,"uuid":"abc-123","userType":"INTERNAL","status":"ACTIVE"}`))
	})

	id, err := ec.CreateUser(context.Background(), CreateUserRequest{
		UUID: "abc-123", UserType: "INTERNAL", Status: "ACTIVE", CreatedBy: marker,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
}

func TestEntityClient_Non2xxBecomesAPIError(t *testing.T) {
	ec := entityTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"invalid treatmentStrategy"}`))
	})

	_, err := ec.CreateRisk(context.Background(), CreateRiskRequest{CreatedBy: marker})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	ae, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if ae.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", ae.Status)
	}
	if !ae.IsValidation() || ae.IsNotFound() || ae.IsConflict() {
		t.Errorf("predicate classification wrong: %+v", ae)
	}
	if !strings.Contains(ae.Body, "invalid treatmentStrategy") {
		t.Errorf("Body = %q, want it to carry the entity message", ae.Body)
	}
	if !strings.Contains(ae.Error(), "POST /risks") || !strings.Contains(ae.Error(), "400") {
		t.Errorf("Error() = %q", ae.Error())
	}
}

func TestEntityClient_GetUserByUUID_404IsNotAnError(t *testing.T) {
	ec := entityTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/by-uuid/missing-uuid" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"user not found"}`))
	})

	id, found, err := ec.GetUserByUUID(context.Background(), "missing-uuid")
	if err != nil {
		t.Fatalf("want nil error on 404, got %v", err)
	}
	if found || id != 0 {
		t.Fatalf("want (0, false), got (%d, %v)", id, found)
	}
}

func TestEntityClient_GetUserByUUID_500IsAnError(t *testing.T) {
	ec := entityTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"message":"internal server error"}`))
	})

	if _, _, err := ec.GetUserByUUID(context.Background(), "u"); err == nil {
		t.Fatal("want an error on 500, got nil")
	} else if ae, ok := AsAPIError(err); !ok || ae.Status != 500 {
		t.Fatalf("want APIError 500, got %v", err)
	}
}
