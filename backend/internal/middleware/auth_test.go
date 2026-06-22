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

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-platform/backend/internal/middleware"
)

// devToken builds an unsigned JWT accepted by Auth when TokenValidatorEnabled=false.
func devToken(sub, email string, groups []string) string {
	claims := jwt.MapClaims{
		"sub":    sub,
		"email":  email,
		"groups": groups,
		"exp":    time.Now().Add(time.Hour).Unix(),
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	return tok
}

func devCfg() middleware.Config {
	return middleware.Config{TokenValidatorEnabled: false}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuth_HealthBypassesAuth(t *testing.T) {
	h := middleware.Auth(devCfg())(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health bypass: got %d, want 200", rec.Code)
	}
}

func TestAuth_MissingToken_Returns401(t *testing.T) {
	h := middleware.Auth(devCfg())(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d, want 401", rec.Code)
	}
}

func TestAuth_MalformedToken_Returns401(t *testing.T) {
	h := middleware.Auth(devCfg())(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("malformed token: got %d, want 401", rec.Code)
	}
}

func TestAuth_ValidDevToken_PopulatesContext(t *testing.T) {
	tok := devToken("uid-1", "dev@example.com", []string{"risk-manager"})

	var captured *middleware.UserInfo
	h := middleware.Auth(devCfg())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = middleware.UserInfoFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, want 200", rec.Code)
	}
	if captured == nil {
		t.Fatal("UserInfo not set in context")
	}
	if captured.Subject != "uid-1" {
		t.Errorf("Subject: got %q, want %q", captured.Subject, "uid-1")
	}
	if captured.Email != "dev@example.com" {
		t.Errorf("Email: got %q, want %q", captured.Email, "dev@example.com")
	}
	if len(captured.Groups) != 1 || captured.Groups[0] != "risk-manager" {
		t.Errorf("Groups: got %v, want [risk-manager]", captured.Groups)
	}
}
