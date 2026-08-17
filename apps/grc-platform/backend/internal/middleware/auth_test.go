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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
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

// unsignedToken builds an alg=none JWT with a known issuer but no real
// signature — what a forger who knows the issuer string can produce without
// the IdP's private key.
func unsignedToken(issuer, sub, email string, groups []string) string {
	claims := jwt.MapClaims{
		"iss":    issuer,
		"sub":    sub,
		"email":  email,
		"groups": groups,
		"exp":    time.Now().Add(time.Hour).Unix(),
	}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	return tok
}

// idpCfg builds a minimal IdPConfig for use with TestKeyFuncs.
func idpCfg(issuer, audience string) config.IdPConfig {
	return config.IdPConfig{
		Issuer:   issuer,
		Audience: audience,
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
	// No PrivilegeStore/Grants configured on devCfg(), so no grant resolution
	// runs and Roles stays unset — a JWT "groups" claim is never read; roles
	// come only from user_role_grant via the grant repository.
	if len(captured.Roles) != 0 {
		t.Errorf("Roles: got %v, want none (no grant repository configured)", captured.Roles)
	}
}

// ── new security tests ─────────────────────────────────────────────────────────

// TestAuth_XJwtAssertion_ValidSignature_PopulatesContext verifies that a
// correctly signed token carried on X-Jwt-Assertion (Choreo's gateway-forwarded
// header) is accepted and resolves the caller's identity, going through the
// same signature-verified path as a bearer token.
func TestAuth_XJwtAssertion_ValidSignature_PopulatesContext(t *testing.T) {
	const issuer = "https://idp.example.com"
	cfg := middleware.Config{
		TokenValidatorEnabled: true,
		IdPs:                  []config.IdPConfig{idpCfg(issuer, "api")},
		TestKeyFuncs:          map[string]jwt.Keyfunc{issuer: testKeyFunc},
	}

	tok := signedToken(issuer, "api", "uid-1", "user@example.com", []string{"risk-manager"})

	var captured *middleware.UserInfo
	h := middleware.Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = middleware.UserInfoFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("X-Jwt-Assertion", tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("assertion: got %d, want 200", rec.Code)
	}
	if captured == nil || captured.Subject != "uid-1" {
		t.Fatalf("expected identity from assertion, got %+v", captured)
	}
}

// TestAuth_XJwtAssertion_UnsignedForgery_Returns401 verifies that an unsigned
// (alg=none) forged assertion — with a *known* issuer, so this exercises
// signature verification specifically rather than issuer rejection — is
// rejected. This is the exact bypass a direct caller on an Organization-visible
// endpoint would attempt, skipping the gateway that would normally mint a
// genuinely-signed assertion; it proves the assertion path is
// signature-verified, not decode-only.
func TestAuth_XJwtAssertion_UnsignedForgery_Returns401(t *testing.T) {
	const issuer = "https://idp.example.com"
	cfg := middleware.Config{
		TokenValidatorEnabled: true,
		IdPs:                  []config.IdPConfig{idpCfg(issuer, "api")},
		TestKeyFuncs:          map[string]jwt.Keyfunc{issuer: testKeyFunc},
	}

	forged := unsignedToken(issuer, "uid-attacker", "attacker@example.com", []string{"admin"})
	h := middleware.Auth(cfg)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("X-Jwt-Assertion", forged)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned forged assertion: got %d, want 401", rec.Code)
	}
}

// TestAuth_XJwtAssertion_TakesPriorityOverBearerToken verifies that when both
// headers are present, X-Jwt-Assertion is used — both are still verified the
// same way, so this only checks header precedence, not a trust difference.
func TestAuth_XJwtAssertion_TakesPriorityOverBearerToken(t *testing.T) {
	const issuer = "https://idp.example.com"
	cfg := middleware.Config{
		TokenValidatorEnabled: true,
		IdPs:                  []config.IdPConfig{idpCfg(issuer, "api")},
		TestKeyFuncs:          map[string]jwt.Keyfunc{issuer: testKeyFunc},
	}

	assertionTok := signedToken(issuer, "api", "uid-assertion", "assertion@example.com", nil)

	var captured *middleware.UserInfo
	h := middleware.Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = middleware.UserInfoFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("X-Jwt-Assertion", assertionTok)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("assertion priority: got %d, want 200", rec.Code)
	}
	if captured == nil || captured.Subject != "uid-assertion" {
		t.Fatalf("expected identity from assertion, got %+v", captured)
	}
}

// TestAuth_UnknownIssuer_Returns401 verifies that a token whose iss claim does not
// match any configured IdP is rejected with 401, not silently passed through.
func TestAuth_UnknownIssuer_Returns401(t *testing.T) {
	const knownIssuer = "https://idp.example.com"
	cfg := middleware.Config{
		TokenValidatorEnabled: true,
		IdPs:                  []config.IdPConfig{idpCfg(knownIssuer, "api")},
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

