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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
)

const (
	portalIssuer   = "https://idp.example/oauth2/token"
	portalWebAud   = "webapp-consumer-key"
	portalM2MAud   = "evidence-portal-consumer-key"
	portalClientID = "evidence-portal-consumer-key"
)

func portalTestChain(t *testing.T, clients map[string]int) http.Handler {
	t.Helper()
	v, err := middleware.NewIdPVerifierWithKeyFuncs(
		[]config.IdPConfig{idpCfg(portalIssuer, portalWebAud)},
		map[string]jwt.Keyfunc{portalIssuer: testKeyFunc},
	)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	return middleware.ClientCredentials(middleware.ClientCredConfig{
		Verifier: v,
		Audience: portalM2MAud,
		Clients:  clients,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := middleware.PortalCallerFromContext(r.Context())
		if c == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(c.ClientID))
	}))
}

func portalReq(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/evidence-portal/controls", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// TestPortalRejectsWebappUserToken is the highest-value test in the package: a
// fully valid webapp user token — right issuer, right signature — must get 401
// on a portal route, because its aud is the webapp's, not the portal's.
func TestPortalRejectsWebappUserToken(t *testing.T) {
	chain := portalTestChain(t, map[string]int{portalClientID: 7})
	tok := signedToken(portalIssuer, portalWebAud, "user-uuid", "person@wso2.com", nil)

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, portalReq(tok))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestPortalAcceptsValidClientToken(t *testing.T) {
	chain := portalTestChain(t, map[string]int{portalClientID: 7})
	tok := signedToken(portalIssuer, portalM2MAud, portalClientID, "", nil)

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, portalReq(tok))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	if rr.Body.String() != portalClientID {
		t.Fatalf("caller not in context: %q", rr.Body.String())
	}
}

func TestPortalRejectsUnknownClient(t *testing.T) {
	chain := portalTestChain(t, map[string]int{portalClientID: 7})
	tok := signedToken(portalIssuer, portalM2MAud, "some-other-client", "", nil)

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, portalReq(tok))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestPortalRejectsUnknownIssuerAndMissingToken(t *testing.T) {
	chain := portalTestChain(t, map[string]int{portalClientID: 7})

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, portalReq(""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: want 401, got %d", rr.Code)
	}

	tok := signedToken("https://evil.example", portalM2MAud, portalClientID, "", nil)
	rr = httptest.NewRecorder()
	chain.ServeHTTP(rr, portalReq(tok))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unknown issuer: want 401, got %d", rr.Code)
	}
}

func TestPortalRateLimitPerClient(t *testing.T) {
	// PortalPerClientRateLimit sits inside the auth chain; drive it directly with a
	// caller already in context.
	limited := middleware.PortalPerClientRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := middleware.WithPortalCaller(context.Background(), &middleware.PortalCaller{ClientID: "c1", TeamID: 1})

	var got429 bool
	for range 40 {
		rr := httptest.NewRecorder()
		limited.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx))
		if rr.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("per-client bucket never returned 429 under a burst well above its size")
	}
}

func TestPortalIngressLimitPerRemoteAddr(t *testing.T) {
	limited := middleware.PortalPerRemoteAddrRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	var got429 bool
	for range 200 {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "203.0.113.9:5555"
		limited.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("ingress bucket never returned 429")
	}
}
