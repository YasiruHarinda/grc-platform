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
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

const guardIssuer = "https://idp.example.com"

// guardToken signs a token with email and an arbitrary email_verified. A nil
// emailVerified omits the claim.
func guardToken(email string, emailVerified any) string {
	claims := jwt.MapClaims{
		"iss":   guardIssuer,
		"aud":   "api",
		"sub":   "uid-1",
		"email": email,
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	if emailVerified != nil {
		claims["email_verified"] = emailVerified
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testRSAKey)
	if err != nil {
		panic("test token sign failed: " + err.Error())
	}
	return tok
}

// guardMux uses real production patterns, not invented ones.
func guardMux() *http.ServeMux {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("GET /api/v1/risks", ok)
	mux.HandleFunc("GET /api/v1/audits", ok)
	mux.HandleFunc("GET /api/v1/audits/users", ok)
	mux.HandleFunc("GET /api/v1/admin/users", ok)
	return mux
}

func guardCfg(mux *http.ServeMux, domains ...string) middleware.Config {
	if domains == nil {
		domains = []string{"wso2.com"}
	}
	return middleware.Config{
		TokenValidatorEnabled: true,
		IdPs:                  []config.IdPConfig{idpCfg(guardIssuer, "api")},
		TestKeyFuncs:          map[string]jwt.Keyfunc{guardIssuer: testKeyFunc},
		Router:                mux,
		InternalEmailDomains:  domains,
	}
}

// serveGuarded runs one request through Auth and returns the status code.
// Auth wraps cfg.Router itself, matching production, so an allowed request is
// answered by the real mux — including its own 404/405.
func serveGuarded(cfg middleware.Config, method, path, token string) int {
	var next http.Handler = okHandler()
	if cfg.Router != nil {
		next = cfg.Router
	}
	h := middleware.Auth(cfg)(next)
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// The control itself: no Risk or Admin route is reachable, even with no
// privilege check anywhere in this test.
func TestGuard_ExternalCallerBlockedOffAuditSurface(t *testing.T) {
	cfg := guardCfg(guardMux())
	tok := guardToken("auditor@external-firm.com", nil)

	for _, path := range []string{"/api/v1/risks", "/api/v1/admin/users", "/api/v1/audits/users"} {
		if code := serveGuarded(cfg, http.MethodGet, path, tok); code != http.StatusForbidden {
			t.Errorf("external caller GET %s: got %d, want 403", path, code)
		}
	}
}

func TestGuard_ExternalCallerReachesAllowListedRoute(t *testing.T) {
	cfg := guardCfg(guardMux())
	tok := guardToken("auditor@external-firm.com", nil)

	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/audits", tok); code != http.StatusOK {
		t.Fatalf("external caller GET /api/v1/audits: got %d, want 200", code)
	}
}

func TestGuard_InternalCallerReachesEverything(t *testing.T) {
	cfg := guardCfg(guardMux())
	tok := guardToken("staff@wso2.com", nil)

	for _, path := range []string{"/api/v1/risks", "/api/v1/admin/users", "/api/v1/audits"} {
		if code := serveGuarded(cfg, http.MethodGet, path, tok); code != http.StatusOK {
			t.Errorf("internal caller GET %s: got %d, want 200", path, code)
		}
	}
}

// Pins the ordering: the guard runs before the grant load, so a rejected
// request costs no entity round trip — and cannot see its own user_type.
func TestGuard_GrantsNotLoadedForBlockedRequest(t *testing.T) {
	cfg := guardCfg(guardMux())
	cfg.PrivilegeStore = privilege.NewForTest(map[string]map[string]bool{})
	cfg.Grants = &failingGrants{t: t}

	tok := guardToken("auditor@external-firm.com", nil)
	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", code)
	}
}

// The fail-open trap: a bare "wso2.com" must not read as the corporate domain,
// which the naive LastIndex implementation would let through.
func TestGuard_MalformedEmailIsExternal(t *testing.T) {
	cfg := guardCfg(guardMux())

	for _, email := range []string{"wso2.com", "", "@wso2.com", "staff@", "staff"} {
		tok := guardToken(email, nil)
		if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusForbidden {
			t.Errorf("email %q: got %d, want 403", email, code)
		}
	}
}

// Guards against a suffix or substring match creeping in.
func TestGuard_SubdomainIsNotTheDomain(t *testing.T) {
	cfg := guardCfg(guardMux())

	for _, email := range []string{"a@evil-wso2.com", "a@wso2.com.attacker.net", "a@sub.wso2.com"} {
		tok := guardToken(email, nil)
		if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusForbidden {
			t.Errorf("email %q: got %d, want 403", email, code)
		}
	}
}

func TestGuard_DomainMatchIsCaseInsensitive(t *testing.T) {
	cfg := guardCfg(guardMux(), "WSO2.com")
	tok := guardToken("Staff@WSO2.COM", nil)

	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
}

func TestGuard_MultipleInternalDomains(t *testing.T) {
	cfg := guardCfg(guardMux(), "wso2.com", "acquired.example")
	tok := guardToken("staff@acquired.example", nil)

	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
}

// Absent must not be held against the caller, or every employee is external on
// deploy; the string "true" must count exactly like the bool.
func TestGuard_EmailVerified(t *testing.T) {
	cfg := guardCfg(guardMux())

	cases := []struct {
		name     string
		claim    any
		wantCode int
	}{
		{"absent", nil, http.StatusOK},
		{"bool true", true, http.StatusOK},
		{"string true", "true", http.StatusOK},
		{"bool false", false, http.StatusForbidden},
		{"string false", "false", http.StatusForbidden},
		{"unrecognised shape", 1, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := guardToken("staff@wso2.com", tc.claim)
			if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != tc.wantCode {
				t.Fatalf("got %d, want %d", code, tc.wantCode)
			}
		})
	}
}

// Records the accepted error-code shift: an external caller sees 403 where an
// internal one sees 404 or 405.
func TestGuard_UnmatchedRouteDeniesExternalCaller(t *testing.T) {
	cfg := guardCfg(guardMux())
	external := guardToken("auditor@external-firm.com", nil)
	internal := guardToken("staff@wso2.com", nil)

	cases := []struct {
		method, path string
		wantInternal int
	}{
		{http.MethodGet, "/api/v1/does-not-exist", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/audits", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		if code := serveGuarded(cfg, c.method, c.path, external); code != http.StatusForbidden {
			t.Errorf("external %s %s: got %d, want 403", c.method, c.path, code)
		}
		if code := serveGuarded(cfg, c.method, c.path, internal); code != c.wantInternal {
			t.Errorf("internal %s %s: got %d, want %d", c.method, c.path, code, c.wantInternal)
		}
	}
}

// The mux's path-cleaning redirect returns a real pattern, not "": a path that
// cleans onto a Risk route must be denied as that route.
func TestGuard_DirtyPathStillResolvesToItsRoute(t *testing.T) {
	cfg := guardCfg(guardMux())
	tok := guardToken("auditor@external-firm.com", nil)

	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/audits/../risks", tok); code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", code)
	}
}

// A Config without a router must work, as a nil PrivilegeStore already does.
func TestGuard_NilRouterDisablesGuard(t *testing.T) {
	cfg := guardCfg(nil)
	tok := guardToken("auditor@external-firm.com", nil)

	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
}

// An unverified email claim is no basis for fencing off routes.
func TestGuard_SkippedInLocalDev(t *testing.T) {
	cfg := middleware.Config{
		TokenValidatorEnabled: false,
		Router:                guardMux(),
		InternalEmailDomains:  []string{"wso2.com"},
	}
	tok := devToken("uid-1", "auditor@external-firm.com", nil)

	if code := serveGuarded(cfg, http.MethodGet, "/api/v1/risks", tok); code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
}

// The probe is on no allow-list and carries no token; it must still pass.
func TestGuard_HealthUnaffected(t *testing.T) {
	h := middleware.Auth(guardCfg(guardMux()))(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

// captureLogs swaps the default slog handler for the duration of fn.
func captureLogs(fn func()) string {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)
	fn()
	return buf.String()
}

// An unrecognised user_type counts as internal, matching the column's
// NOT NULL DEFAULT 'INTERNAL' and directory.Service.LookupTyped. Comparing
// against "INTERNAL" instead would warn on every such row.
func TestGuard_UserTypeCrossCheck(t *testing.T) {
	cases := []struct {
		userType string
		email    string
		wantWarn bool
	}{
		{"INTERNAL", "staff@wso2.com", false},
		{"EXTERNAL", "staff@wso2.com", true},  // external row, corporate email
		{"", "staff@wso2.com", false},         // no user row: skipped
		{"internal", "staff@wso2.com", false}, // unrecognised => internal
		{"SOMETHING_NEW", "staff@wso2.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.userType, func(t *testing.T) {
			cfg := guardCfg(guardMux())
			cfg.PrivilegeStore = privilege.NewForTest(map[string]map[string]bool{})
			cfg.Grants = &stubGrants{userType: tc.userType}

			out := captureLogs(func() {
				serveGuarded(cfg, http.MethodGet, "/api/v1/risks", guardToken(tc.email, nil))
			})
			got := strings.Contains(out, "disagrees with user_type")
			if got != tc.wantWarn {
				t.Fatalf("warned=%v, want %v (log: %s)", got, tc.wantWarn, out)
			}
		})
	}
}
