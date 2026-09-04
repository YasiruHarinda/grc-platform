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

package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
)

const portalCallerKey contextKey = "portal-caller"

// PortalCaller is the verified identity of an Evidence Portal machine client.
// TeamID is resolved from PORTAL_CLIENTS at startup, not from the request.
type PortalCaller struct {
	ClientID string
	TeamID   int
}

// PortalCallerFromContext returns the portal caller set by ClientCredentials,
// or nil if the middleware did not run.
func PortalCallerFromContext(ctx context.Context) *PortalCaller {
	v, _ := ctx.Value(portalCallerKey).(*PortalCaller)
	return v
}

// WithPortalCaller injects a PortalCaller into the context (test helper).
func WithPortalCaller(ctx context.Context, c *PortalCaller) context.Context {
	return context.WithValue(ctx, portalCallerKey, c)
}

// ClientCredConfig configures the Evidence Portal ingress middleware.
type ClientCredConfig struct {
	// Verifier supplies the shared IdP key functions (same org, same JWKS
	// cache as user auth).
	Verifier *IdPVerifier
	// Audience is the expected `aud` — PORTAL_AUTH_AUDIENCE, distinct from any
	// webapp IdP audience.
	Audience string
	// Clients maps an accepted token `sub` (client_id) to its resolved
	// audit_team.id.
	Clients   map[string]int
	ClockSkew time.Duration
}

// ClientCredentials verifies a client_credentials JWT for the portal routes:
// signature + iss/exp against the shared IdPs, `aud` against
// PORTAL_AUTH_AUDIENCE, `sub` in Clients. Any failure is a generic 401.
func ClientCredentials(cfg ClientCredConfig) func(http.Handler) http.Handler {
	skew := cfg.ClockSkew
	if skew <= 0 {
		skew = 5 * time.Second
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Same header precedence as user auth (X-Jwt-Assertion, then Bearer).
			tokenStr := requestToken(r)
			if tokenStr == "" {
				portalUnauthorized(w, r, "missing bearer token")
				return
			}
			sub, err := verifyPortalToken(cfg.Verifier, tokenStr, cfg.Audience, skew)
			if err != nil {
				portalUnauthorized(w, r, err.Error())
				return
			}
			teamID, ok := cfg.Clients[sub]
			if !ok {
				portalUnauthorized(w, r, "client_id not in PORTAL_CLIENTS")
				return
			}
			caller := &PortalCaller{ClientID: sub, TeamID: teamID}
			next.ServeHTTP(w, r.WithContext(WithPortalCaller(r.Context(), caller)))
		})
	}
}

func portalUnauthorized(w http.ResponseWriter, r *http.Request, reason string) {
	slog.WarnContext(r.Context(), "portal auth rejected", "reason", reason)
	response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
}

// verifyPortalToken selects the IdP by the token's iss, then fully verifies the
// token with `aud` overridden to the portal audience.
func verifyPortalToken(v *IdPVerifier, tokenStr, audience string, skew time.Duration) (string, error) {
	if v == nil {
		return "", fmt.Errorf("portal verifier not configured")
	}
	var probe jwtClaims
	if _, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &probe); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	rt, ok := v.idps[probe.Issuer]
	if !ok {
		return "", fmt.Errorf("unknown issuer")
	}
	var c jwtClaims
	token, err := jwt.ParseWithClaims(tokenStr, &c, rt.keyFunc,
		jwt.WithIssuer(rt.cfg.Issuer),
		jwt.WithAudience(audience),
		jwt.WithLeeway(skew),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return "", fmt.Errorf("validate token: %w", err)
	}
	if !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	sub, err := c.GetSubject()
	if err != nil || sub == "" {
		return "", fmt.Errorf("token missing sub claim")
	}
	return sub, nil
}

// ── Rate limiting ────────────────────────────────────────────────────────────

// portalPerClientRate / portalPerClientBurst is the OpenAPI-documented limit
// for the authenticated portal proxy group (10 req/s sustained, burst 20).
const (
	portalPerClientRate  = 10.0
	portalPerClientBurst = 20.0
	// portalIngressRate / portalIngressBurst is the coarse pre-auth bucket,
	// sized well above normal traffic — it bounds the cost of unauthenticated
	// requests (each rejected token still costs an RS256 verification).
	portalIngressRate  = 50.0
	portalIngressBurst = 100.0
)

// PortalPerClientRateLimit is the per-client bucket (keyed on
// PortalCaller.ClientID). It runs INSIDE ClientCredentials, so PortalCaller is
// already in context.
func PortalPerClientRateLimit(next http.Handler) http.Handler {
	buckets := newBucketSet(portalPerClientRate, portalPerClientBurst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "unknown"
		if c := PortalCallerFromContext(r.Context()); c != nil {
			key = c.ClientID
		}
		if !buckets.allow(key) {
			response.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// PortalPerRemoteAddrRateLimit is the coarse per-remote-address bucket. It
// runs OUTSIDE ClientCredentials and must not consult anything derived from
// the token.
func PortalPerRemoteAddrRateLimit(next http.Handler) http.Handler {
	buckets := newBucketSet(portalIngressRate, portalIngressBurst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !buckets.allow(host) {
			response.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bucketSet is a small keyed token-bucket limiter. The map is capped at
// bucketSetMaxKeys and dropped when exceeded, so an unbounded set of remote
// addresses cannot grow it without limit.
type bucketSet struct {
	rate, burst float64
	mu          sync.Mutex
	m           map[string]*bucket
}

const bucketSetMaxKeys = 10000

type bucket struct {
	tokens float64
	last   time.Time
}

func newBucketSet(rate, burst float64) *bucketSet {
	return &bucketSet{rate: rate, burst: burst, m: make(map[string]*bucket)}
}

func (s *bucketSet) allow(key string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) > bucketSetMaxKeys {
		s.m = make(map[string]*bucket)
	}
	b, ok := s.m[key]
	if !ok {
		b = &bucket{tokens: s.burst, last: now}
		s.m[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * s.rate
	if b.tokens > s.burst {
		b.tokens = s.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
