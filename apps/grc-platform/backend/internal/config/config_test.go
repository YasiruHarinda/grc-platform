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

package config

import (
	"strings"
	"testing"
)

// setRequiredNonAuthEnv sets every mustEnv-required variable Load() needs
// besides the Asgardeo JWT ones, so these tests can isolate the
// AUTH_TOKEN_VALIDATOR_ENABLED/APP_ENV guard without also exercising the rest
// of Load()'s required-env surface.
func setRequiredNonAuthEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"COMPLIANCE_ENTITY_BASE_URL": "http://localhost:8080",
		"HR_ENTITY_GRAPHQL_URL":      "http://localhost:8091/graphql",
		"HR_ENTITY_TOKEN_URL":        "http://localhost:8091/token",
		"HR_ENTITY_CLIENT_ID":        "id",
		"HR_ENTITY_CLIENT_SECRET":    "secret",
		"EMAIL_SERVICE_URL":          "http://localhost:8092",
		"EMAIL_FROM_ADDRESS":         "noreply@example.com",
		"FRONTEND_BASE_URL":          "http://localhost:3000",
		"EMAIL_CLIENT_ID":            "id",
		"EMAIL_CLIENT_SECRET":        "secret",
		"EMAIL_TOKEN_URL":            "http://localhost:8092/token",
	} {
		t.Setenv(k, v)
	}
}

// TestLoadRefusesTokenValidatorDisabledWithoutLocalAppEnv is the regression
// test for the kill-switch finding: AUTH_TOKEN_VALIDATOR_ENABLED=false
// disables both signature verification and every privilege check
// (allow-all — see HasPrivilege). A single misconfigured env var in a real
// deployment would be a silent, total auth bypass, so Load() must refuse to
// start unless APP_ENV=local also confirms this is a developer's machine.
func TestLoadRefusesTokenValidatorDisabledWithoutLocalAppEnv(t *testing.T) {
	setRequiredNonAuthEnv(t)
	t.Setenv("AUTH_TOKEN_VALIDATOR_ENABLED", "false")
	t.Setenv("APP_ENV", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with AUTH_TOKEN_VALIDATOR_ENABLED=false and no APP_ENV = nil error, want a refusal to start")
	}
	if !strings.Contains(err.Error(), "APP_ENV=local") {
		t.Fatalf("Load() error = %q, want it to mention APP_ENV=local", err.Error())
	}
}

// TestLoadRefusesTokenValidatorDisabledWithWrongAppEnv guards against a
// misconfigured deployment carrying some other APP_ENV value (e.g.
// "staging", "production") and satisfying a naive non-empty check.
func TestLoadRefusesTokenValidatorDisabledWithWrongAppEnv(t *testing.T) {
	setRequiredNonAuthEnv(t)
	t.Setenv("AUTH_TOKEN_VALIDATOR_ENABLED", "false")
	t.Setenv("APP_ENV", "staging")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with AUTH_TOKEN_VALIDATOR_ENABLED=false and APP_ENV=staging = nil error, want a refusal to start")
	}
}

// TestLoadAllowsTokenValidatorDisabledWithLocalAppEnv confirms the intended
// local-dev path still works: APP_ENV=local permits the bypass.
func TestLoadAllowsTokenValidatorDisabledWithLocalAppEnv(t *testing.T) {
	setRequiredNonAuthEnv(t)
	t.Setenv("AUTH_TOKEN_VALIDATOR_ENABLED", "false")
	t.Setenv("APP_ENV", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with AUTH_TOKEN_VALIDATOR_ENABLED=false and APP_ENV=local = error %v, want success", err)
	}
	if cfg.Auth.TokenValidatorEnabled {
		t.Fatal("cfg.Auth.TokenValidatorEnabled = true, want false")
	}
}

// TestLoadDefaultTokenValidatorEnabledIgnoresAppEnv confirms the guard only
// applies when the validator is actually disabled — an unset
// AUTH_TOKEN_VALIDATOR_ENABLED (secure default) must boot with no APP_ENV at
// all, the normal shape of a real deployment.
func TestLoadDefaultTokenValidatorEnabledIgnoresAppEnv(t *testing.T) {
	setRequiredNonAuthEnv(t)
	t.Setenv("AUTH_TOKEN_VALIDATOR_ENABLED", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("AUTH_JWKS_ENDPOINT", "https://example.asgardeo.io/jwks")
	t.Setenv("AUTH_ISSUER", "https://example.asgardeo.io")
	t.Setenv("AUTH_AUDIENCE", "aud")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with AUTH_TOKEN_VALIDATOR_ENABLED unset and no APP_ENV = error %v, want success", err)
	}
	if !cfg.Auth.TokenValidatorEnabled {
		t.Fatal("cfg.Auth.TokenValidatorEnabled = false, want true (secure default)")
	}
}
