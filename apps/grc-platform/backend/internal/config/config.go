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
	"strings"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Port is the net.Listen address, always colon-prefixed. PORT itself is
	// written bare ("8081") to match component.yaml; listenAddr bridges them.
	Port                    string
	Auth                    AuthConfig
	ComplianceEntityBaseURL string
	HREntity                HREntityConfig
	SCIM                    SCIMConfig
	CORSAllowedOrigin       string
	AIValidation            AIValidationConfig
	Email                   EmailConfig
	// LeadEscalationEmailsEnabled turns on emailing a person's HR line manager
	// (their "lead" — see EmailConfig's comment) when work they are responsible
	// for is escalated: an overdue audit item's owner's lead (the audit
	// reminder sweep), and a risk's assigner/action-owner leads (on
	// escalation). One switch for both modules — it replaces the old
	// AUDIT_LEAD_ESCALATION_ENABLED. See LeadEscalationEmailsDefault.
	LeadEscalationEmailsEnabled bool
	// SchedulerEnabled turns the background scheduler (internal/scheduler) on
	// or off. It is the single switch for every daily sweep at once — today
	// the overdue-risk escalation and the audit due-date reminder digest. See
	// SchedulerEnabledDefault.
	SchedulerEnabled bool
}

// SchedulerEnabledDefault is the built-in setting for the background
// scheduler, used by every environment that does not override it. On by
// default: the daily sweeps are core behaviour, and an operator disabling them
// is the exception.
//
// SCHEDULER_ENABLED overrides it with exactly "true" or "false"; any other
// value — including unset — leaves this default alone, so a typo can never
// silently stop the sweeps.
const SchedulerEnabledDefault = true

// schedulerEnabled resolves the SCHEDULER_ENABLED override against the default
// above.
func schedulerEnabled() bool {
	switch os.Getenv("SCHEDULER_ENABLED") {
	case "true":
		return true
	case "false":
		return false
	default:
		return SchedulerEnabledDefault
	}
}

// LeadEscalationEmailsDefault is the built-in setting for the lead-escalation
// emails, used by every environment that does not override it. Off by default:
// this preserves the behaviour before the switch existed — audit already
// defaulted off (as AUDIT_LEAD_ESCALATION_ENABLED, which this replaces), and
// risk never sent a lead email at all — so turning it on is a deliberate
// per-environment opt-in.
//
// LEAD_ESCALATION_EMAILS_ENABLED overrides it with exactly "true" or "false".
const LeadEscalationEmailsDefault = false

// leadEscalationEmailsEnabled resolves the override against the default above.
// Any other value — including unset — leaves the default alone, so a typo can
// never silently start mailing leads.
func leadEscalationEmailsEnabled() bool {
	switch os.Getenv("LEAD_ESCALATION_EMAILS_ENABLED") {
	case "true":
		return true
	case "false":
		return false
	default:
		return LeadEscalationEmailsDefault
	}
}

// EmailNotificationsDefault is the built-in setting for the master email
// switch. On by default: email is the platform's only notification channel, so
// an environment sending no email at all is the exception. Set
// EMAIL_NOTIFICATIONS_ENABLED to exactly "false" to disable every send from
// both modules — see EmailConfig.Enabled.
const EmailNotificationsDefault = true

// emailNotificationsEnabled resolves EMAIL_NOTIFICATIONS_ENABLED against the
// default above. Any other value — including unset — leaves the default alone,
// so a typo can never silently mute every notification.
func emailNotificationsEnabled() bool {
	switch os.Getenv("EMAIL_NOTIFICATIONS_ENABLED") {
	case "true":
		return true
	case "false":
		return false
	default:
		return EmailNotificationsDefault
	}
}

// EmailConfig holds the connection details for the shared email-sending
// service (email-service), used to notify a risk's owner when the risk is
// created. Normally required (mustEnv) like HREntityConfig: unlike AI
// validation, a misconfigured notifier fails silently from the product's
// perspective (nobody gets told the risk exists), so this is treated as
// load-bearing rather than optional — EXCEPT when Enabled is false, where the
// five service fields are relaxed to optional (see Load) because a disabled
// client never reads them.
//
// The service's own code (service.bal) has no inbound auth check, but the
// real Choreo-hosted instance sits behind API Manager with OAuth2
// client-credentials, same as HREntityConfig — ClientID/ClientSecret/TokenURL
// are required for real calls to succeed.
//
// "lead" (used throughout both modules for the recipient of an escalation
// email) means a person's HR line manager, resolved from the HR entity's
// managerEmail — frozen per-escalation in risk, resolved per-sweep in audit.
type EmailConfig struct {
	ServiceURL      string
	FromAddress     string
	FrontendBaseURL string
	ClientID        string
	ClientSecret    string
	TokenURL        string
	// Enabled is the master switch (EMAIL_NOTIFICATIONS_ENABLED). When false,
	// emailer.Client short-circuits every send to a no-op before any token
	// fetch or HTTP call — no module sends any email. Upstream work
	// (recipient resolution, lead resolution, compliance-admin lookups, the
	// daily sweeps) still runs; only the send is suppressed.
	Enabled bool
}

// AIValidationConfig configures the fire-and-forget trigger to the AI Validation
// Agent. When Enabled is false the backend never contacts the agent.
type AIValidationConfig struct {
	Enabled      bool
	AgentBaseURL string
	AgentAPIKey  string
}

// IdPConfig describes one trusted identity provider (Asgardeo organization).
// Tokens are validated against the matching issuer's JWKS/audience only.
type IdPConfig struct {
	Issuer       string
	JWKSEndpoint string
	Audience     string
}

type AuthConfig struct {
	// IdPs holds every trusted issuer. Empty when TokenValidatorEnabled is false (local dev decodes tokens without verification).
	IdPs                  []IdPConfig
	ClockSkew             time.Duration
	TokenValidatorEnabled bool
	// InternalEmailDomains decides whether a caller is internal. From
	// AUTH_INTERNAL_EMAIL_DOMAINS, which only defaults to SCIM_USER_DOMAIN and
	// never reads it — that one is the directory cache's filter, and sharing
	// it would let a cache tweak lock the company out.
	InternalEmailDomains []string
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

// SCIMConfig holds the connection details for calling Asgardeo's own SCIM2
// API directly, which this platform uses as its identity directory:
// resolving an email to a person's Asgardeo id, and a uuid back to their
// name. A security review required that the platform stop storing names and
// emails, so this is where both now come from.
//
// Internal users and external auditors live in genuinely separate Asgardeo
// organizations (see internal/scim.NewClient vs NewExternalClient), each
// with its own OAuth2 app registration — hence the separate
// SCIM_INTERNAL_*/SCIM_EXTERNAL_* env vars per org below (Org/ClientID/
// ClientSecret/Scopes fields, unprefixed here since they're already
// disambiguated by sitting next to their External* siblings). BaseURL is the
// one thing shared: both orgs sit under the same Asgardeo API root
// (https://api.asgardeo.io), just a different /t/{org}/... tenant path.
//
// TokenURL and ExternalTokenURL have no env var: SCIMTokenURL derives them
// from BaseURL and the org, so they cannot drift out of step with it.
//
// Optional, unlike HREntityConfig. An unset BaseURL disables directory
// lookups rather than failing startup: local development frequently runs
// without Asgardeo credentials, and the flows that use it are written to
// degrade (a user is provisioned without a uuid) rather than break. Configure
// it in every deployed environment.
//
// Scopes/ExternalScopes are space-separated OAuth2 scope strings. Only
// internal_user_mgt_view and internal_user_mgt_list are requested — this
// client only ever searches users, never groups/bulk/update, even though the
// underlying Asgardeo app may be authorised for more. Asgardeo silently
// drops a scope the application is not authorised for rather than failing
// the token request, so a missing grant appears as a 403 at call time rather
// than a startup failure.
//
// UserDomain is the email-domain suffix the directory's bulk cache is scoped
// to (see internal/directory.Service.StartBulkRefresh). An unfiltered
// users-search returns the whole org — 300,000+ records last checked,
// overwhelmingly load-test accounts — and this deployment's directory has no
// working "active" filter to narrow that with, so a domain suffix is what
// keeps the cache to real employees instead. Internal-org only: the bulk
// cache has no external-org equivalent (see internal/directory.Service.LookupTyped).
type SCIMConfig struct {
	BaseURL string

	Org          string
	ClientID     string
	ClientSecret string
	TokenURL     string
	Scopes       string

	ExternalOrg          string
	ExternalClientID     string
	ExternalClientSecret string
	ExternalTokenURL     string
	ExternalScopes       string

	UserDomain string
}

// Configured reports whether enough is set to build a working internal-org
// client. Independent of ExternalConfigured — local dev or a partial
// rollout can have Asgardeo credentials for one org without the other, and
// each client degrades to "unknown" on its own when unset (see
// internal/scim.Client's nil-tolerance).
//
// Satisfied by credentials alone: TokenURL is derived from BaseURL and Org, so
// testing it here could never fail. An environment that previously forgot
// SCIM_INTERNAL_TOKEN_URL and silently ran with no directory now gets a
// client — deliberately (see TestLoadSCIMConfiguredWithoutTokenURLVar).
func (c SCIMConfig) Configured() bool {
	return c.BaseURL != "" && c.Org != "" && c.ClientID != "" && c.ClientSecret != ""
}

// ExternalConfigured is Configured for the external-org client.
func (c SCIMConfig) ExternalConfigured() bool {
	return c.BaseURL != "" && c.ExternalOrg != "" &&
		c.ExternalClientID != "" && c.ExternalClientSecret != ""
}

// Load reads configuration from environment variables.
//
// There is no database configuration: the backend reaches all data through the
// Compliance Entity, so DB_DSN is neither read nor required.
// AUTH_JWKS_ENDPOINT, AUTH_ISSUER, and AUTH_AUDIENCE are only required when
// AUTH_TOKEN_VALIDATOR_ENABLED is true (the default). They are not needed for
// local development (set AUTH_TOKEN_VALIDATOR_ENABLED=false).
//
// AUTH_TOKEN_VALIDATOR_ENABLED=false is a full auth bypass, not just a
// signature-check toggle: middleware.Auth decodes the token without
// verifying it AND, because privStore is never built in this mode
// (cmd/server/main.go), auth.HasPrivilege/HasPrivilegeIn answer true for
// every check — allow-all. Requiring APP_ENV=local alongside it closes one
// specific gap: a *single* mistyped or copy-pasted env var (e.g. an entire
// local .env pasted into a Choreo environment) can no longer silently open
// this bypass — the server now crashes loudly at boot instead. It is not a
// defence against someone who can already write to the same Choreo variable
// store deliberately setting both vars together; that requires either
// restricting who can write AUTH_TOKEN_VALIDATOR_ENABLED/APP_ENV in each
// deployed environment, or removing the unverified-decode path from
// non-local builds entirely (a build-tag split around the ParseUnverified
// branch in middleware/auth.go — tracked as a follow-up, not done here).
func Load() (Config, error) {
	tokenValidatorEnabled := os.Getenv("AUTH_TOKEN_VALIDATOR_ENABLED") != "false"
	if !tokenValidatorEnabled && os.Getenv("APP_ENV") != "local" {
		return Config{}, fmt.Errorf(
			"AUTH_TOKEN_VALIDATOR_ENABLED=false disables JWT signature verification and every " +
				"privilege check (allow-all); refusing to start without APP_ENV=local also set, so " +
				"this doesn't take effect from a single accidentally-set variable")
	}

	scimUserDomain := envOrDefault("SCIM_USER_DOMAIN", "wso2.com")
	internalEmailDomains, err := loadInternalEmailDomains(scimUserDomain)
	if err != nil {
		return Config{}, err
	}

	authCfg := AuthConfig{
		ClockSkew:             5 * time.Second,
		TokenValidatorEnabled: tokenValidatorEnabled,
		InternalEmailDomains:  internalEmailDomains,
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

	// FRONTEND_BASE_URL stays required regardless of the email switch — it is
	// also the CORS-allowed origin (see CORSAllowedOrigin below).
	frontendBaseURL, err := mustEnv("FRONTEND_BASE_URL")
	if err != nil {
		return Config{}, err
	}

	// EMAIL_NOTIFICATIONS_ENABLED=false relaxes the five email-service vars
	// from required to optional: a disabled emailer.Client never reads them,
	// which makes "run with no email" a first-class mode for local dev and CI.
	// Any other value keeps them required — a half-configured notifier is
	// worse than a loud startup failure.
	emailEnabled := emailNotificationsEnabled()
	emailEnv := mustEnv
	if !emailEnabled {
		emailEnv = func(key string) (string, error) { return os.Getenv(key), nil }
	}
	emailServiceURL, err := emailEnv("EMAIL_SERVICE_URL")
	if err != nil {
		return Config{}, err
	}
	emailFromAddress, err := emailEnv("EMAIL_FROM_ADDRESS")
	if err != nil {
		return Config{}, err
	}
	emailClientID, err := emailEnv("EMAIL_CLIENT_ID")
	if err != nil {
		return Config{}, err
	}
	emailClientSecret, err := emailEnv("EMAIL_CLIENT_SECRET")
	if err != nil {
		return Config{}, err
	}
	emailTokenURL, err := emailEnv("EMAIL_TOKEN_URL")
	if err != nil {
		return Config{}, err
	}

	// Token endpoints are derived, not configured — see SCIMTokenURL. The
	// trailing slash is stripped here rather than in the helper because
	// scim.Client concatenates this same BaseURL raw.
	scimBaseURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv("SCIM_BASE_URL")), "/")
	scimInternalOrg := os.Getenv("SCIM_INTERNAL_ORG")
	scimExternalOrg := os.Getenv("SCIM_EXTERNAL_ORG")

	return Config{
		Port:                    listenAddr(envOrDefault("PORT", "8081")),
		Auth:                    authCfg,
		ComplianceEntityBaseURL: complianceEntityBaseURL,
		HREntity: HREntityConfig{
			GraphQLURL:   hrEntityGraphQLURL,
			TokenURL:     hrEntityTokenURL,
			ClientID:     hrEntityClientID,
			ClientSecret: hrEntityClientSecret,
		},
		SCIM: SCIMConfig{
			BaseURL:    scimBaseURL,
			UserDomain: scimUserDomain,

			Org:          scimInternalOrg,
			ClientID:     os.Getenv("SCIM_INTERNAL_CLIENT_ID"),
			ClientSecret: os.Getenv("SCIM_INTERNAL_CLIENT_SECRET"),
			TokenURL:     SCIMTokenURL(scimBaseURL, scimInternalOrg),
			Scopes:       envOrDefault("SCIM_INTERNAL_SCOPES", "internal_user_mgt_view internal_user_mgt_list"),

			ExternalOrg:          scimExternalOrg,
			ExternalClientID:     os.Getenv("SCIM_EXTERNAL_CLIENT_ID"),
			ExternalClientSecret: os.Getenv("SCIM_EXTERNAL_CLIENT_SECRET"),
			ExternalTokenURL:     SCIMTokenURL(scimBaseURL, scimExternalOrg),
			ExternalScopes:       envOrDefault("SCIM_EXTERNAL_SCOPES", "internal_user_mgt_view internal_user_mgt_list"),
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
			Enabled:         emailEnabled,
		},
		LeadEscalationEmailsEnabled: leadEscalationEmailsEnabled(),
		SchedulerEnabled:            schedulerEnabled(),
	}, nil
}

// loadIdPs builds the trusted-issuer list from the environment — today, just
// the GRC web app's IdP.
func loadIdPs() ([]IdPConfig, error) {
	idp1 := IdPConfig{}
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
	return []IdPConfig{idp1}, nil
}

// loadInternalEmailDomains reads AUTH_INTERNAL_EMAIL_DOMAINS, comma-separated,
// falling back to the SCIM user domain. Empty set and empty element are both
// startup failures: the first 403s the whole company, the second (a stray
// comma) matches any address with an empty domain part, failing open.
func loadInternalEmailDomains(scimUserDomain string) ([]string, error) {
	raw := envOrDefault("AUTH_INTERNAL_EMAIL_DOMAINS", scimUserDomain)
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf(
			"AUTH_INTERNAL_EMAIL_DOMAINS (or SCIM_USER_DOMAIN, its default) resolved to an empty set; " +
				"every caller would be classified external and blocked from all but the audit routes")
	}
	parts := strings.Split(raw, ",")
	domains := make([]string, 0, len(parts))
	for _, p := range parts {
		d := strings.ToLower(strings.TrimSpace(p))
		if d == "" {
			return nil, fmt.Errorf(
				"AUTH_INTERNAL_EMAIL_DOMAINS contains an empty domain (check for a stray comma): %q", raw)
		}
		domains = append(domains, d)
	}
	return domains, nil
}

// listenAddr turns a bare PORT ("8081", matching component.yaml and the
// entity's SERVER_PORT) into the colon-prefixed address net.Listen requires —
// net.Listen("tcp", "8081") fails with "missing port in address".
//
// A value already containing a colon passes through, so an environment still
// holding the old ":8081" keeps booting. Whitespace is trimmed: a trailing
// space in a web console's variable editor is invisible but fatal.
func listenAddr(port string) string {
	port = strings.TrimSpace(port)
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

// SCIMTokenURL builds one Asgardeo org's OAuth2 token endpoint:
// {baseURL}/t/{org}/oauth2/token — the same shape internal/scim.Client uses for
// /t/{org}/scim2/Users/.search. Replaces the hand-maintained
// SCIM_INTERNAL_TOKEN_URL / SCIM_EXTERNAL_TOKEN_URL, which could drift from
// their org. Exported for the cmd/backfill-* tools, which bypass Load.
//
// baseURL must have no trailing slash — Load normalises it, since scim.Client
// concatenates the same value raw. Returns "" if either input is empty, so
// Configured still reports false.
func SCIMTokenURL(baseURL, org string) string {
	if baseURL == "" || org == "" {
		return ""
	}
	return baseURL + "/t/" + org + "/oauth2/token"
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
