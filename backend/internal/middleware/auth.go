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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/grc-platform/backend/internal/shared/privilege"
)

type contextKey string

const userInfoKey contextKey = "user-info"

// UserInfo holds the authenticated user's identity extracted from the Asgardeo JWT.
type UserInfo struct {
	Subject string
	Email   string
	Groups  []string // Asgardeo role/group claims
}

// Config holds JWT validation settings loaded from environment variables.
type Config struct {
	JWKSEndpoint          string
	Issuer                string
	Audience              string
	ClockSkew             time.Duration
	TokenValidatorEnabled bool
	// PrivilegeStore resolves role→privilege mappings after JWT validation.
	// When nil, privilege checking is skipped and HasPrivilege always returns true.
	// Set to nil for local dev (TokenValidatorEnabled=false); always set in production.
	PrivilegeStore *privilege.Store
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

// Auth validates the Authorization: Bearer JWT on every request and stores
// the resulting UserInfo in the context.
// When TokenValidatorEnabled is false the token is only decoded without signature
// verification — for local development only.
func Auth(cfg Config) func(http.Handler) http.Handler {
	var keyFunc jwt.Keyfunc
	if cfg.TokenValidatorEnabled {
		jwks, err := keyfunc.NewDefault([]string{cfg.JWKSEndpoint})
		if err != nil {
			panic("auth: failed to initialise JWKS from " + cfg.JWKSEndpoint + ": " + err.Error())
		}
		keyFunc = jwks.Keyfunc
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

			info, err := extractUserInfo(tokenStr, cfg, keyFunc)
			if err != nil {
				slog.ErrorContext(r.Context(), "auth: token validation failed", "err", err)
				writeAuthError(w, "You are not authorized to perform this action. Please try again.")
				return
			}

			ctx := context.WithValue(r.Context(), userInfoKey, info)
			if cfg.PrivilegeStore != nil {
				privs := cfg.PrivilegeStore.Resolve(info.Groups)
				ctx = privilege.WithContext(ctx, privs)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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

func extractUserInfo(tokenStr string, cfg Config, keyFunc jwt.Keyfunc) (*UserInfo, error) {
	var c jwtClaims

	if !cfg.TokenValidatorEnabled {
		_, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &c)
		if err != nil {
			return nil, fmt.Errorf("decode token: %w", err)
		}
	} else {
		token, err := jwt.ParseWithClaims(tokenStr, &c, keyFunc,
			jwt.WithIssuer(cfg.Issuer),
			jwt.WithAudience(cfg.Audience),
			jwt.WithLeeway(cfg.ClockSkew),
			jwt.WithExpirationRequired(),
		)
		if err != nil {
			return nil, fmt.Errorf("validate token: %w", err)
		}
		if !token.Valid {
			return nil, fmt.Errorf("invalid token")
		}
	}

	sub, err := c.GetSubject()
	if err != nil || sub == "" {
		return nil, fmt.Errorf("token missing sub claim")
	}

	return &UserInfo{Subject: sub, Email: c.Email, Groups: c.Groups}, nil
}
