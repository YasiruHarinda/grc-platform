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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
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
//
// There is no group→role map any more: roles are assigned in this platform's
// database and resolved per request from user_role_grant, so no IdP config
// participates in authorisation beyond identifying the issuer.
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
	// runs and Roles stays unset. The token carries a "groups" claim, and it
	// must be ignored either way: role assignment lives in this platform's
	// database, not in the IdP. Honouring it would leave a second, invisible
	// source of authority that no admin could revoke from User Management.
	if len(captured.Roles) != 0 {
		t.Errorf("Roles: got %v, want empty — a token's groups claim must never confer roles", captured.Roles)
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

// ── grant resolution ───────────────────────────────────────────────────────────

// stubGrants is a grant.Repository returning fixed grants.
type stubGrants struct {
	userID int
	grants []grant.Grant
	err    error
	calls  int
}

func (s *stubGrants) ForEmail(_ context.Context, _ string) (int, []grant.Grant, error) {
	s.calls++
	return s.userID, s.grants, s.err
}

func (s *stubGrants) Candidates(_ context.Context, _ string, _ []int) ([]grant.Candidate, error) {
	return nil, nil
}

// failingGrants fails the test if it is ever consulted.
type failingGrants struct{ t *testing.T }

func (f *failingGrants) ForEmail(_ context.Context, email string) (int, []grant.Grant, error) {
	f.t.Errorf("grants must not be loaded for this caller (email %q)", email)
	return 0, nil, nil
}

func (f *failingGrants) Candidates(_ context.Context, _ string, _ []int) ([]grant.Candidate, error) {
	f.t.Errorf("candidates must not be loaded for this caller")
	return nil, nil
}

// grantCfg builds a full-scope config wired to the given grant repository.
func grantCfg(issuer string, store *privilege.Store, grants grant.Repository) middleware.Config {
	return middleware.Config{
		TokenValidatorEnabled: true,
		PrivilegeStore:        store,
		Grants:                grants,
		IdPs:                  []config.IdPConfig{idpCfg(issuer, "api")},
		TestKeyFuncs:          map[string]jwt.Keyfunc{issuer: testKeyFunc},
	}
}

// TestAuth_GrantsResolvePerScope is the core of the migration: one user holding
// two different roles in two different registers must not have them merged.
//
// Without per-scope resolution they would hold the union, and could approve as
// Risk Owner a risk belonging to the register where they are only an assigner.
func TestAuth_GrantsResolvePerScope(t *testing.T) {
	const issuer = "https://idp.example.com"
	const asgardeo, choreo, cleverCare = 1, 2, 3

	store := privilege.NewForTest(map[string]map[string]bool{
		"risk-owner":    {privilege.OwnerApproveRisk: true},
		"risk-assigner": {privilege.CreateRisk: true},
	})
	grants := &stubGrants{userID: 7, grants: []grant.Grant{
		{RoleName: "risk-owner", Module: "RISK", ScopeType: grant.ScopeRiskTeam, ScopeID: asgardeo},
		{RoleName: "risk-assigner", Module: "RISK", ScopeType: grant.ScopeRiskTeam, ScopeID: choreo},
	}}

	var set *grant.Set
	var info *middleware.UserInfo
	h := middleware.Auth(grantCfg(issuer, store, grants))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set = grant.FromContext(r.Context())
		info = middleware.UserInfoFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken(issuer, "api", "uid-7", "n@example.com", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if info.UserID != 7 {
		t.Errorf("UserID: got %d, want 7", info.UserID)
	}

	// Owner rights in Asgardeo only.
	if !set.HasIn(privilege.OwnerApproveRisk, asgardeo) {
		t.Error("should hold OwnerApprove in Asgardeo")
	}
	if set.HasIn(privilege.OwnerApproveRisk, choreo) {
		t.Error("must NOT hold OwnerApprove in Choreo — roles leaked across scopes")
	}
	// Assigner rights in Choreo only.
	if !set.HasIn(privilege.CreateRisk, choreo) {
		t.Error("should hold CreateRisk in Choreo")
	}
	if set.HasIn(privilege.CreateRisk, asgardeo) {
		t.Error("must NOT hold CreateRisk in Asgardeo — roles leaked across scopes")
	}
	// A register they hold nothing in.
	if set.HasIn(privilege.CreateRisk, cleverCare) || set.HasIn(privilege.OwnerApproveRisk, cleverCare) {
		t.Error("must hold nothing in a register with no grant")
	}
	// The union — published via PrivilegeMap, what route gating and the Audit
	// Hub actually read — still answers "anywhere".
	union := set.PrivilegeMap()
	if !union[privilege.OwnerApproveRisk] || !union[privilege.CreateRisk] {
		t.Error("union should contain both privileges")
	}
	if set.HasGlobal(privilege.OwnerApproveRisk) || set.HasGlobal(privilege.CreateRisk) {
		t.Error("a team-scoped caller must not hold either privilege globally")
	}
}

// TestAuth_GlobalGrantCoversEveryScope verifies that GLOBAL behaves as a
// wildcard — including for a team id that did not exist when it was granted,
// which is what makes "new registers are covered automatically" true.
func TestAuth_GlobalGrantCoversEveryScope(t *testing.T) {
	const issuer = "https://idp.example.com"

	store := privilege.NewForTest(map[string]map[string]bool{
		"compliance-admin": {privilege.ComplianceApproveRisk: true},
	})
	grants := &stubGrants{userID: 9, grants: []grant.Grant{
		{RoleName: "compliance-admin", Module: "RISK", ScopeType: grant.ScopeGlobal, ScopeID: 0},
	}}

	var set *grant.Set
	h := middleware.Auth(grantCfg(issuer, store, grants))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set = grant.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken(issuer, "api", "uid-9", "a@example.com", nil))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !set.HasGlobal(privilege.ComplianceApproveRisk) {
		t.Error("a GLOBAL grant holder must hold the privilege globally")
	}
	for _, teamID := range []int{1, 2, 4242} {
		if !set.HasIn(privilege.ComplianceApproveRisk, teamID) {
			t.Errorf("GLOBAL grant should cover team %d, including registers created later", teamID)
		}
	}
	if ids := set.RegisterScopeIDs(); len(ids) != 0 {
		t.Errorf("a GLOBAL-only caller has no team-scoped footprint: got %v", ids)
	}
}

// TestAuth_GrantLoadFailure_Returns503 verifies the request fails closed when
// grants cannot be loaded. Serving the request with no privileges would look to
// the user exactly like having been revoked, and serving it with stale ones
// would need a cache this design deliberately does not have. 503, not 401: this
// is a grant-store failure, not a rejection of the caller's identity.
func TestAuth_GrantLoadFailure_Returns503(t *testing.T) {
	const issuer = "https://idp.example.com"
	store := privilege.NewForTest(map[string]map[string]bool{})
	grants := &stubGrants{err: errors.New("entity unavailable")}

	called := false
	h := middleware.Auth(grantCfg(issuer, store, grants))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken(issuer, "api", "uid-1", "x@example.com", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("grant load failure: got %d, want 503", rec.Code)
	}
	if called {
		t.Error("handler must not run when grants could not be loaded")
	}
}

// TestAuth_NoGrants_HoldsNothing verifies that a caller with no grants is
// authenticated but holds nothing. This is a legitimate state, not an error:
// an Action Owner may be any employee, reaching only the risks they are
// personally named on.
func TestAuth_NoGrants_HoldsNothing(t *testing.T) {
	const issuer = "https://idp.example.com"
	store := privilege.NewForTest(map[string]map[string]bool{
		"risk-owner": {privilege.OwnerApproveRisk: true},
	})
	grants := &stubGrants{userID: 12, grants: nil}

	var set *grant.Set
	h := middleware.Auth(grantCfg(issuer, store, grants))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set = grant.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
	req.Header.Set("Authorization", "Bearer "+signedToken(issuer, "api", "uid-12", "ao@example.com", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 — no grants is authenticated, just unprivileged", rec.Code)
	}
	if !set.IsEmpty() {
		t.Error("a caller with no grants must report empty")
	}
	if set.PrivilegeMap()[privilege.OwnerApproveRisk] || set.HasIn(privilege.OwnerApproveRisk, 1) {
		t.Error("a caller with no grants must hold nothing")
	}
	if set.HasGlobal(privilege.OwnerApproveRisk) {
		t.Error("a caller with no grants is not global")
	}
}

// TestAuth_GrantsLoadedEveryRequest verifies grants are not cached across
// requests. Revocation is a security path: an admin removing a grant must see
// it take effect on the user's next request, not after a TTL elapses.
func TestAuth_GrantsLoadedEveryRequest(t *testing.T) {
	const issuer = "https://idp.example.com"
	store := privilege.NewForTest(map[string]map[string]bool{})
	grants := &stubGrants{userID: 3}

	h := middleware.Auth(grantCfg(issuer, store, grants))(okHandler())
	tok := signedToken(issuer, "api", "uid-3", "r@example.com", nil)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/risks", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	if grants.calls != 3 {
		t.Errorf("grants loaded %d times across 3 requests, want 3 — grants must never be cached", grants.calls)
	}
}
