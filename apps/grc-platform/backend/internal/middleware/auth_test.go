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
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// ── shared test helpers ────────────────────────────────────────────────────────

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

// testRSAKey is a package-level RSA key pair generated once for all signed-token tests.
var testRSAKey = func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("test RSA keygen failed: " + err.Error())
	}
	return k
}()

// signedToken creates an RS256-signed JWT using testRSAKey.
func signedToken(issuer, audience, sub, email string, groups []string) string {
	claims := jwt.MapClaims{
		"iss":    issuer,
		"aud":    audience,
		"sub":    sub,
		"email":  email,
		"groups": groups,
		"exp":    time.Now().Add(time.Hour).Unix(),
		"iat":    time.Now().Unix(),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testRSAKey)
	if err != nil {
		panic("test token sign failed: " + err.Error())
	}
	return tok
}

// testKeyFunc is a jwt.Keyfunc that always returns testRSAKey's public key.
func testKeyFunc(t *jwt.Token) (interface{}, error) {
	return &testRSAKey.PublicKey, nil
}

// idpCfg builds a minimal IdPConfig for use with TestKeyFuncs.
func idpCfg(issuer, audience, scope string, groupRoleMap map[string]string) config.IdPConfig {
	return config.IdPConfig{
		Issuer:       issuer,
		Audience:     audience,
		Scope:        scope,
		GroupRoleMap: groupRoleMap,
	}
}

// ── existing tests ─────────────────────────────────────────────────────────────

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

// ── new security tests ─────────────────────────────────────────────────────────

// TestAuth_UnknownIssuer_Returns401 verifies that a token whose iss claim does not
// match any configured IdP is rejected with 401, not silently passed through.
func TestAuth_UnknownIssuer_Returns401(t *testing.T) {
	const knownIssuer = "https://idp.example.com"
	cfg := middleware.Config{
		TokenValidatorEnabled: true,
		IdPs:                  []config.IdPConfig{idpCfg(knownIssuer, "api", config.ScopeFull, nil)},
		TestKeyFuncs:          map[string]jwt.Keyfunc{knownIssuer: testKeyFunc},
	}

	tok := signedToken("https://unknown-issuer.evil.com", "api", "uid-x", "x@example.com", nil)
	h := middleware.Auth(cfg)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown issuer: got %d, want 401", rec.Code)
	}
}

// TestAuth_IdP2TokenCappedAtSubmitEvidence verifies that a token from the evidence-app
// IdP (ScopeEvidenceApp) has its resolved privileges intersected with the ceiling so
// only SUBMIT_EVIDENCE survives, regardless of what the group→role map resolves to.
func TestAuth_IdP2TokenCappedAtSubmitEvidence(t *testing.T) {
	const issuer2 = "https://idp2.example.com"

	// Role "full_access" has many privileges; evidence-app ceiling must strip all but SUBMIT_EVIDENCE.
	store := privilege.NewForTest(map[string]map[string]bool{
		"full_access": {
			privilege.CreateAudit:    true,
			privilege.ManageControls: true,
			privilege.SubmitEvidence: true,
		},
	})

	cfg := middleware.Config{
		TokenValidatorEnabled: true,
		PrivilegeStore:        store,
		IdPs: []config.IdPConfig{idpCfg(issuer2, "api2", config.ScopeEvidenceApp, map[string]string{
			"ext_group": "full_access",
		})},
		TestKeyFuncs: map[string]jwt.Keyfunc{issuer2: testKeyFunc},
	}

	tok := signedToken(issuer2, "api2", "ext-uid", "ext@example.com", []string{"ext_group"})

	var privs map[string]bool
	h := middleware.Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privs = privilege.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-app/controls", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("IdP-2 token: got %d, want 200", rec.Code)
	}
	if privs[privilege.CreateAudit] || privs[privilege.ManageControls] {
		t.Error("IdP-2 token: ceiling did not strip non-evidence privileges")
	}
	if !privs[privilege.SubmitEvidence] {
		t.Error("IdP-2 token: SUBMIT_EVIDENCE should be allowed by ceiling")
	}
}

// TestIssuerScope_EvidenceAppToken_BlockedOutsidePath verifies that an
// evidence-app-scoped token cannot reach routes outside /api/v1/evidence-app/.
func TestIssuerScope_EvidenceAppToken_BlockedOutsidePath(t *testing.T) {
	info := &middleware.UserInfo{Scope: "evidence-app"}
	h := middleware.IssuerScope(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audits", nil)
	req = req.WithContext(middleware.WithUserInfo(req.Context(), info))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("evidence-app token outside prefix: got %d, want 403", rec.Code)
	}
}

// TestIssuerScope_EvidenceAppToken_AllowedOnPath verifies that the scope middleware
// passes evidence-app tokens through when the path starts with /api/v1/evidence-app/.
func TestIssuerScope_EvidenceAppToken_AllowedOnPath(t *testing.T) {
	info := &middleware.UserInfo{Scope: "evidence-app"}
	h := middleware.IssuerScope(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-app/controls", nil)
	req = req.WithContext(middleware.WithUserInfo(req.Context(), info))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("evidence-app token on prefix: got %d, want 200", rec.Code)
	}
}

// TestRateLimiter_Blocks429WithRetryAfter verifies that exhausting the burst budget
// returns 429 with a non-empty Retry-After header.
func TestRateLimiter_Blocks429WithRetryAfter(t *testing.T) {
	// burst=1 means the first request consumes the only token; the second is denied.
	rl := middleware.NewRateLimiter(1, 1)
	h := rl.Wrap(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-app/controls/1/submit", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if first := send(); first.Code != http.StatusOK {
		t.Fatalf("first request (within burst): got %d, want 200", first.Code)
	}

	second := send()
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request (burst exhausted): got %d, want 429", second.Code)
	}
	ra := second.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("429 response missing Retry-After header")
	}
	secs, err := strconv.Atoi(ra)
	if err != nil || secs < 1 {
		t.Fatalf("Retry-After %q: want a positive integer seconds value", ra)
	}
}

// TestRateLimiter_AllowsWithinBurst verifies that requests within the burst budget
// are not rate-limited when spread across distinct callers.
func TestRateLimiter_AllowsWithinBurst(t *testing.T) {
	rl := middleware.NewRateLimiter(10, 5)
	h := rl.Wrap(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-app/controls", nil)
			req.Header.Set("X-Forwarded-For", "10.0.0."+strconv.Itoa(i+1))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("caller %d within burst: got %d, want 200", i, rec.Code)
			}
		}(i)
	}
	wg.Wait()
}
