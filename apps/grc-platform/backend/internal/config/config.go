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
	"fmt"
	"os"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Port                    string
	Auth                    AuthConfig
	ComplianceEntityBaseURL string
	HREntity                HREntityConfig
	RiskGroups              RiskGroupsConfig
	CORSAllowedOrigin       string
	AIValidation            AIValidationConfig
	Email                   EmailConfig
}

// EmailConfig holds the connection details for the shared email-sending
// service (email-service), used to notify a risk's owner when the risk is
// created. Required (mustEnv) like HREntityConfig: unlike AI validation, a
// misconfigured/disabled notifier fails silently from the product's
// perspective (nobody gets told the risk exists), so this is treated as
// load-bearing rather than optional.
//
// The service's own code (service.bal) has no inbound auth check, but the
// real Choreo-hosted instance sits behind API Manager with OAuth2
// client-credentials, same as HREntityConfig — ClientID/ClientSecret/TokenURL
// are required for real calls to succeed.
type EmailConfig struct {
	ServiceURL      string
	FromAddress     string
	FrontendBaseURL string
	ClientID        string
	ClientSecret    string
	TokenURL        string
}

// AIValidationConfig configures the fire-and-forget trigger to the AI Validation
// Agent. When Enabled is false the backend never contacts the agent.
type AIValidationConfig struct {
	Enabled      bool
	AgentBaseURL string
	AgentAPIKey  string
}

// Auth scope values classify what an IdP's tokens are allowed to reach.
// A full-scope token (IdP-1, the GRC web app) can reach the whole API; an
// evidence-app-scoped token (IdP-2, the Evidence Portal) is restricted to
// /api/v1/evidence-app/* and capped at the evidence-app privilege ceiling.
const (
	ScopeFull        = "full"
	ScopeEvidenceApp = "evidence-app"
)

// IdPConfig describes one trusted identity provider (Asgardeo organization).
// Tokens are validated against the matching issuer's JWKS/audience only.
type IdPConfig struct {
	Issuer       string
	JWKSEndpoint string
	Audience     string
	Scope        string // ScopeFull | ScopeEvidenceApp
}

type AuthConfig struct {
	// IdPs holds every trusted issuer. IdP-1 (index 0) is the GRC web app; IdP-2,
	// when configured, is the Evidence Portal. Empty when TokenValidatorEnabled is
	// false (local dev decodes tokens without verification).
	IdPs                  []IdPConfig
	ClockSkew             time.Duration
	TokenValidatorEnabled bool
}

// HREntityConfig holds the connection details for the WSO2 HR entity GraphQL
// service (hr_entity), used to look up employees for the Risk module's
// "Risk Identified By: Employee" field. Employee data is never stored in the
// GRC platform's own database — it is fetched live on every search.
// GraphQLURL points at the real service on Choreo in production, or a local
// mock server during development; the code is identical either way.
type HREntityConfig struct {
	GraphQLURL   string
	TokenURL     string
	ClientID     string
	ClientSecret string
}

// RiskGroupsConfig holds Asgardeo group names still awaiting a Risk Hub
// consumer. The Management Approver and Risk Owner pickers used to be here
// too, but those now read user_role_grant directly (see
// riskhandler.Deps.Grants) — an Asgardeo group and a platform grant are two
// independently-maintained sources, and nothing kept them in sync, so a
// candidate sourced from the group could lack the grant their approval action
// actually checks and 403 on first use.
type RiskGroupsConfig struct {
	// ComplianceAdmin is provisioned for the future Compliance Admin email
	// notification (see notifyComplianceAdmins in risk/handler/notify.go,
	// currently a deliberate no-op) — nothing reads this yet.
	ComplianceAdmin string
}

// Load reads configuration from environment variables.
//
// There is no database configuration: the backend reaches all data through the
// Compliance Entity, so DB_DSN is neither read nor required.
// AUTH_JWKS_ENDPOINT, AUTH_ISSUER, and AUTH_AUDIENCE are only required when
// AUTH_TOKEN_VALIDATOR_ENABLED is true (the default). They are not needed for
// local development (set AUTH_TOKEN_VALIDATOR_ENABLED=false).
func Load() (Config, error) {
	tokenValidatorEnabled := os.Getenv("AUTH_TOKEN_VALIDATOR_ENABLED") != "false"

	authCfg := AuthConfig{
		ClockSkew:             5 * time.Second,
		TokenValidatorEnabled: tokenValidatorEnabled,
	}
	if tokenValidatorEnabled {
		idps, err := loadIdPs()
		if err != nil {
			return Config{}, err
		}
		authCfg.IdPs = idps
	}

	complianceEntityBaseURL, err := mustEnv("COMPLIANCE_ENTITY_BASE_URL")
	if err != nil {
		return Config{}, err
	}

	hrEntityGraphQLURL, err := mustEnv("HR_ENTITY_GRAPHQL_URL")
	if err != nil {
		return Config{}, err
	}
	hrEntityTokenURL, err := mustEnv("HR_ENTITY_TOKEN_URL")
	if err != nil {
		return Config{}, err
	}
	hrEntityClientID, err := mustEnv("HR_ENTITY_CLIENT_ID")
	if err != nil {
		return Config{}, err
	}
	hrEntityClientSecret, err := mustEnv("HR_ENTITY_CLIENT_SECRET")
	if err != nil {
		return Config{}, err
	}

	emailServiceURL, err := mustEnv("EMAIL_SERVICE_URL")
	if err != nil {
		return Config{}, err
	}
	emailFromAddress, err := mustEnv("EMAIL_FROM_ADDRESS")
	if err != nil {
		return Config{}, err
	}
	frontendBaseURL, err := mustEnv("FRONTEND_BASE_URL")
	if err != nil {
		return Config{}, err
	}
	emailClientID, err := mustEnv("EMAIL_CLIENT_ID")
	if err != nil {
		return Config{}, err
	}
	emailClientSecret, err := mustEnv("EMAIL_CLIENT_SECRET")
	if err != nil {
		return Config{}, err
	}
	emailTokenURL, err := mustEnv("EMAIL_TOKEN_URL")
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:                    envOrDefault("PORT", ":8081"),
		Auth:                    authCfg,
		ComplianceEntityBaseURL: complianceEntityBaseURL,
		HREntity: HREntityConfig{
			GraphQLURL:   hrEntityGraphQLURL,
			TokenURL:     hrEntityTokenURL,
			ClientID:     hrEntityClientID,
			ClientSecret: hrEntityClientSecret,
		},
		RiskGroups: RiskGroupsConfig{
			ComplianceAdmin: envOrDefault("RISK_COMPLIANCE_ADMIN_GROUP", "grc-platform-risk-compliance-admin"),
		},
		// Derived from FRONTEND_BASE_URL rather than its own env var: both are
		// "the webapp's public origin", and having two meant one could be
		// correctly set (this one is mustEnv, so a typo fails startup loudly)
		// while the other silently defaulted to localhost — a deployment
		// could boot with email links pointing somewhere CORS doesn't trust.
		CORSAllowedOrigin: frontendBaseURL,
		AIValidation: AIValidationConfig{
			Enabled:      os.Getenv("AI_VALIDATION_ENABLED") == "true",
			AgentBaseURL: envOrDefault("AI_AGENT_BASE_URL", "http://localhost:8090"),
			AgentAPIKey:  os.Getenv("AI_AGENT_API_KEY"),
		},
		Email: EmailConfig{
			ServiceURL:      emailServiceURL,
			FromAddress:     emailFromAddress,
			FrontendBaseURL: frontendBaseURL,
			ClientID:        emailClientID,
			ClientSecret:    emailClientSecret,
			TokenURL:        emailTokenURL,
		},
	}, nil
}

// loadIdPs builds the trusted-issuer list from the environment. IdP-1 (the GRC
// web app) is always required. IdP-2 (the Evidence Portal) is optional — it is
// appended only when AUTH_ISSUER_2 is set, so single-IdP deployments are
// unchanged. When AUTH_ISSUER_2 is set, all of its companion vars are required
// (fail fast).
//
// AUTH_GROUP_ROLE_MAP_2 is gone. It mapped the Evidence Portal's group claims
// onto GRC role names, which were then resolved to privileges and intersected
// down to exactly {SUBMIT_EVIDENCE} — so the whole chain only ever produced one
// bit. An evidence-app token now carries that capability by virtue of its
// issuer (see middleware.evidenceAppPrivileges), and no token's group claim is
// read anywhere.
func loadIdPs() ([]IdPConfig, error) {
	idp1 := IdPConfig{Scope: ScopeFull}
	var err error
	if idp1.JWKSEndpoint, err = mustEnv("AUTH_JWKS_ENDPOINT"); err != nil {
		return nil, err
	}
	if idp1.Issuer, err = mustEnv("AUTH_ISSUER"); err != nil {
		return nil, err
	}
	if idp1.Audience, err = mustEnv("AUTH_AUDIENCE"); err != nil {
		return nil, err
	}
	idps := []IdPConfig{idp1}

	if os.Getenv("AUTH_ISSUER_2") == "" {
		return idps, nil // single-IdP deployment
	}

	idp2 := IdPConfig{Scope: ScopeEvidenceApp}
	if idp2.Issuer, err = mustEnv("AUTH_ISSUER_2"); err != nil {
		return nil, err
	}
	if idp2.JWKSEndpoint, err = mustEnv("AUTH_JWKS_ENDPOINT_2"); err != nil {
		return nil, err
	}
	if idp2.Audience, err = mustEnv("AUTH_AUDIENCE_2"); err != nil {
		return nil, err
	}
	return append(idps, idp2), nil
}

func mustEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable is not set: %s", key)
	}
	return v, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
