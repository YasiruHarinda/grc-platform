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

// Lead-escalation emails reach people outside the risk/audit itself, so only
// the two exact spellings may override the built-in default — a typo must leave
// it alone rather than resolve to "on". Replaces AUDIT_LEAD_ESCALATION_ENABLED.
func TestLeadEscalationEmailsOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string // always set (possibly to ""), so an ambient value can't leak in
		want bool
	}{
		{"unset/empty uses the code default", "", LeadEscalationEmailsDefault},
		{"true enables", "true", true},
		{"false disables", "false", false},
		{"typo uses the code default", "ture", LeadEscalationEmailsDefault},
		{"TRUE is not true", "TRUE", LeadEscalationEmailsDefault},
		{"1 is not true", "1", LeadEscalationEmailsDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LEAD_ESCALATION_EMAILS_ENABLED", tt.env)
			if got := leadEscalationEmailsEnabled(); got != tt.want {
				t.Errorf("leadEscalationEmailsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Guards the shipped default. Flipping it is a deliberate release decision, so
// this failing is the reminder to update the docs and each environment's
// override.
func TestLeadEscalationEmailsDefaultIsOff(t *testing.T) {
	if LeadEscalationEmailsDefault {
		t.Error("lead-escalation emails ship enabled — intended? update the design docs and .env.example")
	}
}

// EMAIL_NOTIFICATIONS_ENABLED is the master mute for both modules, so only the
// two exact spellings override the default; a typo must not silence every
// notification.
func TestEmailNotificationsOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string // always set (possibly to ""), so an ambient value can't leak in
		want bool
	}{
		{"unset/empty uses the code default", "", EmailNotificationsDefault},
		{"true enables", "true", true},
		{"false disables", "false", false},
		{"typo uses the code default", "flase", EmailNotificationsDefault},
		{"FALSE is not false", "FALSE", EmailNotificationsDefault},
		{"0 is not false", "0", EmailNotificationsDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EMAIL_NOTIFICATIONS_ENABLED", tt.env)
			if got := emailNotificationsEnabled(); got != tt.want {
				t.Errorf("emailNotificationsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Guards the shipped default: email is the platform's only notification
// channel, so it must be on unless an operator explicitly mutes it.
func TestEmailNotificationsDefaultIsOn(t *testing.T) {
	if !EmailNotificationsDefault {
		t.Error("email notifications ship disabled — intended? no module would send any email")
	}
}

// With the master switch off, the five EMAIL_* service vars are relaxed from
// mustEnv to optional so "run with no email" is a first-class local/CI mode;
// with it on (the default) they stay required.
func TestLoadEmailVarsRequiredUnlessDisabled(t *testing.T) {
	setAuthEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("AUTH_TOKEN_VALIDATOR_ENABLED", "false")
		t.Setenv("APP_ENV", "local")
	}

	// Explicitly blank the five EMAIL_* so an ambient value (a dev who ran
	// `source .env` before `go test`) can't make the "unset" cases pass or
	// fail for the wrong reason — mustEnv treats "" as missing.
	clearEmailEnv := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{
			"EMAIL_SERVICE_URL", "EMAIL_FROM_ADDRESS", "EMAIL_CLIENT_ID",
			"EMAIL_CLIENT_SECRET", "EMAIL_TOKEN_URL",
		} {
			t.Setenv(k, "")
		}
	}

	t.Run("missing EMAIL_* fails when notifications enabled", func(t *testing.T) {
		setAuthEnv(t)
		clearEmailEnv(t)
		t.Setenv("EMAIL_NOTIFICATIONS_ENABLED", "true") // explicit, not ambient
		for _, k := range []string{
			"COMPLIANCE_ENTITY_BASE_URL", "HR_ENTITY_GRAPHQL_URL", "HR_ENTITY_TOKEN_URL",
			"HR_ENTITY_CLIENT_ID", "HR_ENTITY_CLIENT_SECRET", "FRONTEND_BASE_URL",
		} {
			t.Setenv(k, "x")
		}
		if _, err := Load(); err == nil {
			t.Fatal("Load() with EMAIL_* unset and notifications enabled = nil error, want failure")
		}
	})

	t.Run("missing EMAIL_* is fine when notifications disabled", func(t *testing.T) {
		setAuthEnv(t)
		clearEmailEnv(t)
		t.Setenv("EMAIL_NOTIFICATIONS_ENABLED", "false")
		for _, k := range []string{
			"COMPLIANCE_ENTITY_BASE_URL", "HR_ENTITY_GRAPHQL_URL", "HR_ENTITY_TOKEN_URL",
			"HR_ENTITY_CLIENT_ID", "HR_ENTITY_CLIENT_SECRET", "FRONTEND_BASE_URL",
		} {
			t.Setenv(k, "x")
		}
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with EMAIL_* unset and notifications disabled = %v, want success", err)
		}
		if cfg.Email.Enabled {
			t.Error("cfg.Email.Enabled = true, want false")
		}
	})
}

// SCHEDULER_ENABLED is an operational kill-switch, so — like the lead
// escalation flag — only the two exact spellings may override the built-in
// default; a typo must leave it alone rather than silently stop every sweep.
func TestSchedulerEnabledOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{"empty (as good as unset) uses the code default", "", SchedulerEnabledDefault},
		{"true enables", "true", true},
		{"false disables", "false", false},
		{"typo uses the code default", "flase", SchedulerEnabledDefault},
		{"FALSE is not false", "FALSE", SchedulerEnabledDefault},
		{"0 is not false", "0", SchedulerEnabledDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set it on every row — including the empty default case — so a
			// SCHEDULER_ENABLED already in the process env (CI, a sourced
			// .env) can't decide the outcome. os.Getenv can't tell unset from
			// empty, so "" exercises the same default path as unset.
			t.Setenv("SCHEDULER_ENABLED", tt.env)
			if got := schedulerEnabled(); got != tt.want {
				t.Errorf("schedulerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Guards the shipped default: the daily sweeps run unless an operator
// explicitly opts out.
func TestSchedulerEnabledDefaultIsOn(t *testing.T) {
	if !SchedulerEnabledDefault {
		t.Error("the background scheduler ships disabled — intended? overdue-risk escalation and audit reminders will not run automatically")
	}
}
