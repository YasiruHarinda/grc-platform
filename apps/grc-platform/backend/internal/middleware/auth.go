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
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type contextKey string

const userInfoKey contextKey = "user-info"

// UserInfo holds the authenticated user's identity extracted from the Asgardeo JWT.
type UserInfo struct {
	Subject string
	Email   string
	Groups  []string // Asgardeo role/group claims
	Issuer  string   // the token's verified issuer (iss)
	Scope   string   // the matched IdP's scope: config.ScopeFull | config.ScopeEvidenceApp
}

// Config holds JWT validation settings loaded from environment variables.
type Config struct {
	// IdPs is the set of trusted issuers. Empty when TokenValidatorEnabled is false.
	IdPs                  []config.IdPConfig
	ClockSkew             time.Duration
	TokenValidatorEnabled bool
	// PrivilegeStore resolves role→privilege mappings after JWT validation.
	// When nil, privilege checking is skipped and HasPrivilege always returns true.
	// Set to nil for local dev (TokenValidatorEnabled=false); always set in production.
	PrivilegeStore *privilege.Store
	// TestKeyFuncs maps issuer → jwt.Keyfunc, bypassing JWKS cache construction.
	// Never set in production; used by unit tests to inject pre-built key functions.
	TestKeyFuncs map[string]jwt.Keyfunc
}

// evidenceAppPrivilegeCeiling is the single source of truth for what an
// evidence-app-scoped token (IdP-2) may ever do, no matter what groups it carries
// or how AUTH_GROUP_ROLE_MAP_2 is (mis)configured. Resolved privileges are
// intersected with this set for evidence-app tokens.
var evidenceAppPrivilegeCeiling = map[string]bool{privilege.SubmitEvidence: true}

// intersectCeiling drops any privilege not permitted by the ceiling.
func intersectCeiling(privs, ceiling map[string]bool) map[string]bool {
	out := make(map[string]bool, len(privs))
	for p := range privs {
		if ceiling[p] {
			out[p] = true
		}
	}
	return out
}

// idpRuntime pairs a configured IdP with its JWKS-backed key function.
type idpRuntime struct {
	cfg     config.IdPConfig
	keyFunc jwt.Keyfunc
}

type jwtClaims struct {
	Email  string   `json:"email"`
	Groups []string `json:"groups"`
	jwt.RegisteredClaims
}

type authErrorBody struct {
	Message string `json:"message"`
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(authErrorBody{Message: message})
}

// jwkEntry holds a single RSA public key extracted from a JWKS response.
type jwkEntry struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksDoc struct {
	Keys []jwkEntry `json:"keys"`
}

// jwksCache fetches and caches RSA public keys from a JWKS endpoint.
// It reads only the n/e parameters and ignores x5c/x5t entirely, which avoids
// compatibility issues with Asgardeo's JWKS certificates (negative serial numbers,
// x5t#S256 mismatches) introduced by Go 1.23's stricter x509 validation.
type jwksCache struct {
	mu       sync.RWMutex
	keys     map[string]*rsa.PublicKey
	endpoint string
	client   *http.Client
}

func newJWKSCache(ctx context.Context, endpoint string) (*jwksCache, error) {
	c := &jwksCache{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
		keys:     make(map[string]*rsa.PublicKey),
	}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := c.refresh(); err != nil {
					slog.Error("JWKS refresh failed", "endpoint", endpoint, "err", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return c, nil
}

func (c *jwksCache) refresh() error {
	resp, err := c.client.Get(c.endpoint)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: unexpected status %d", resp.StatusCode)
	}

	var doc jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	next := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kid == "" || k.Kty != "RSA" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			slog.Warn("skipping JWK with invalid RSA params", "kid", k.Kid, "err", err)
			continue
		}
		next[k.Kid] = pub
	}

	c.mu.Lock()
	c.keys = next
	c.mu.Unlock()
	slog.Info("JWKS refreshed", "keys", len(next))
	return nil
}

func (c *jwksCache) lookup(kid string) (*rsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok := c.keys[kid]
	return k, ok
}

// rsaPublicKeyFromJWK reconstructs an *rsa.PublicKey from the base64url-encoded
// modulus (n) and public exponent (e) carried in a JWK. No x5c or x5t needed.
func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e < 3 || e%2 == 0 {
		return nil, fmt.Errorf("invalid RSA exponent: %d", e)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

// Auth validates the Authorization: Bearer JWT on every request and stores
// the resulting UserInfo in the context.
// When TokenValidatorEnabled is false the token is only decoded without signature
// verification — for local development only.
func Auth(cfg Config) func(http.Handler) http.Handler {
	// Build one JWKS-backed key function per trusted issuer (reusing the shared
	// jwksCache). Indexed by issuer so extractUserInfo can select the right IdP
	// from the token's iss claim.
	idps := make(map[string]idpRuntime, len(cfg.IdPs))
	if cfg.TokenValidatorEnabled {
		for _, idp := range cfg.IdPs {
			if kf, ok := cfg.TestKeyFuncs[idp.Issuer]; ok {
				// Test override: skip JWKS cache and HTTPS requirement.
				idps[idp.Issuer] = idpRuntime{cfg: idp, keyFunc: kf}
				continue
			}
			u, parseErr := url.Parse(idp.JWKSEndpoint)
			if parseErr != nil || u.Scheme != "https" {
				panic("auth: JWKS endpoint must use https, got: " + idp.JWKSEndpoint)
			}
			cache, err := newJWKSCache(context.Background(), idp.JWKSEndpoint)
			if err != nil {
				panic("auth: failed to initialise JWKS from " + idp.JWKSEndpoint + ": " + err.Error())
			}
			// Capture cache per iteration for the closure.
			c := cache
			idps[idp.Issuer] = idpRuntime{
				cfg: idp,
				keyFunc: func(token *jwt.Token) (interface{}, error) {
					kid, _ := token.Header["kid"].(string)
					key, ok := c.lookup(kid)
					if !ok {
						return nil, fmt.Errorf("key %q not found in JWKS", kid)
					}
					return key, nil
				},
			}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			tokenStr := bearerToken(r)
			if tokenStr == "" {
				writeAuthError(w, "You are not authorized to perform this action. Please try again.")
				return
			}

			info, idp, err := extractUserInfo(tokenStr, cfg, idps)
			if err != nil {
				slog.ErrorContext(r.Context(), "auth: token validation failed", "err", err)
				writeAuthError(w, "You are not authorized to perform this action. Please try again.")
				return
			}

			ctx := context.WithValue(r.Context(), userInfoKey, info)
			if cfg.PrivilegeStore != nil {
				privs := cfg.PrivilegeStore.Resolve(mapGroups(info.Groups, idp))
				// Cap evidence-app-scoped tokens at the ceiling, independent of config.
				if idp != nil && idp.Scope == config.ScopeEvidenceApp {
					privs = intersectCeiling(privs, evidenceAppPrivilegeCeiling)
				}
				ctx = privilege.WithContext(ctx, privs)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// mapGroups applies an IdP's group→role map (dropping unmapped groups) so an
// external IdP's group names never leak directly into GRC role resolution. When
// the IdP has no map (nil), or in local dev (idp nil), groups pass through as-is.
func mapGroups(groups []string, idp *config.IdPConfig) []string {
	if idp == nil || idp.GroupRoleMap == nil {
		return groups
	}
	mapped := make([]string, 0, len(groups))
	for _, g := range groups {
		if role, ok := idp.GroupRoleMap[g]; ok {
			mapped = append(mapped, role)
		}
	}
	return mapped
}

// UserInfoFromContext retrieves the authenticated user from the context.
// Returns nil if the auth middleware was not applied.
func UserInfoFromContext(ctx context.Context) *UserInfo {
	v, _ := ctx.Value(userInfoKey).(*UserInfo)
	return v
}

// WithUserInfo injects a UserInfo into the context (test helper).
func WithUserInfo(ctx context.Context, user *UserInfo) context.Context {
	return context.WithValue(ctx, userInfoKey, user)
}

func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	after, ok := strings.CutPrefix(v, "Bearer ")
	if !ok {
		return ""
	}
	return after
}

// extractUserInfo validates the token and returns the caller's identity plus the
// IdP that issued it (nil in local-dev mode). In production it selects the IdP by
// the token's iss claim: an unknown issuer is rejected with the same generic error
// as any other invalid token, so the set of configured issuers is not leaked.
func extractUserInfo(tokenStr string, cfg Config, idps map[string]idpRuntime) (*UserInfo, *config.IdPConfig, error) {
	if !cfg.TokenValidatorEnabled {
		// Local dev: decode without signature verification. No IdP selection.
		var c jwtClaims
		if _, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &c); err != nil {
			return nil, nil, fmt.Errorf("decode token: %w", err)
		}
		sub, err := c.GetSubject()
		if err != nil || sub == "" {
			return nil, nil, fmt.Errorf("token missing sub claim")
		}
		return &UserInfo{Subject: sub, Email: c.Email, Groups: c.Groups, Issuer: c.Issuer}, nil, nil
	}

	// Read the issuer from the unverified token only to pick the IdP; nothing else
	// from this parse is trusted.
	var probe jwtClaims
	if _, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &probe); err != nil {
		return nil, nil, fmt.Errorf("decode token: %w", err)
	}
	rt, ok := idps[probe.Issuer]
	if !ok {
		return nil, nil, fmt.Errorf("unknown issuer")
	}

	var c jwtClaims
	token, err := jwt.ParseWithClaims(tokenStr, &c, rt.keyFunc,
		jwt.WithIssuer(rt.cfg.Issuer),
		jwt.WithAudience(rt.cfg.Audience),
		jwt.WithLeeway(cfg.ClockSkew),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("validate token: %w", err)
	}
	if !token.Valid {
		return nil, nil, fmt.Errorf("invalid token")
	}

	sub, err := c.GetSubject()
	if err != nil || sub == "" {
		return nil, nil, fmt.Errorf("token missing sub claim")
	}

	idpCfg := rt.cfg
	return &UserInfo{
		Subject: sub,
		Email:   c.Email,
		Groups:  c.Groups,
		Issuer:  rt.cfg.Issuer,
		Scope:   rt.cfg.Scope,
	}, &idpCfg, nil
}
