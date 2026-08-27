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

package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	adminentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin/entity"
	adminhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin/handler"
	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditjob "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/job"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	riskjob "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/job"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/entity"
	userhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/handler"
)

func main() {
	middleware.ConfigureLogger()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "err", err)
		os.Exit(1)
	}

	// File operations go through the Compliance Entity (which holds the Azure key);
	// the backend never talks to Azure directly.
	fileSvc := file.NewService(cfg.ComplianceEntityBaseURL)

	// Typed HTTP client to the Compliance Entity for data access (migrating the
	// backend off direct MySQL, stage by stage).
	entityCli := entityclient.New(cfg.ComplianceEntityBaseURL)

	// Cancelled on SIGINT/SIGTERM. Established before the privilege store so
	// that store's periodic reload goroutine lives for the whole process and
	// stops only at shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Grants are read from (and, via the Admin Console, written to) the entity
	// on every call and never cached: role→privilege changes only on a deploy
	// (hence privStore's 15-minute refresh below), but a revoked grant must
	// take effect immediately, and a newly-created one must be usable right
	// away. Built unconditionally — deliberately NOT gated on
	// TokenValidatorEnabled like privStore below: it's a plain entity HTTP
	// client with no dependency on token verification, and the Admin
	// Console's grant editor (internal/admin/handler) needs a working one
	// even in local dev with TokenValidatorEnabled=false, same as it needs a
	// working entity client for everything else it does. Gating this the
	// same way privStore is gated silently 500s every grant create/revoke in
	// local dev — which is exactly the mode most manual admin-console testing
	// runs in.
	grantRepo := grant.NewRepository(entityCli)

	// Load the role→privilege mapping from the Compliance Entity. Built after
	// entityCli because it needs it.
	// When TokenValidatorEnabled=false (local dev), skip loading — HasPrivilege returns true for all checks.
	// When TokenValidatorEnabled=true (production), load is required — exit if it fails.
	// privilege.New bounds the initial load itself; ctx here governs only the
	// lifetime of its background refresh.
	var privStore *privilege.Store
	if cfg.Auth.TokenValidatorEnabled {
		privStore, err = privilege.New(ctx, entityCli)
		if err != nil {
			slog.Error("failed to load privilege mapping from the Compliance Entity", "err", err)
			os.Exit(1)
		}
		slog.Info("privilege store loaded")
	}

	hrClient := hrentity.NewClient(cfg.HREntity.GraphQLURL, cfg.HREntity.TokenURL, cfg.HREntity.ClientID, cfg.HREntity.ClientSecret)

	// The identity directory. Left nil when unconfigured, which the client
	// tolerates by answering "no such user" — see scim.Client.LookupByEmail.
	// Local development without Asgardeo credentials then provisions users
	// without a uuid instead of failing.
	// scimExternalClient resolves user_type=EXTERNAL identities (external
	// auditors), which live in a separate Asgardeo org from scimClient's —
	// see internal/scim.NewExternalClient. Only Audit has external users
	// today; Risk keeps resolving everyone through scimClient/dirSvc's
	// existing internal-only Lookup/LookupAll, untouched by this.
	// Configured/ExternalConfigured are independent: a deployment can have
	// Asgardeo credentials for one org without the other, and each client
	// degrades to "unknown" on its own when unset.
	var scimClient, scimExternalClient *scim.Client
	if cfg.SCIM.Configured() {
		scimClient = scim.NewClient(cfg.SCIM.BaseURL, cfg.SCIM.TokenURL,
			cfg.SCIM.ClientID, cfg.SCIM.ClientSecret, cfg.SCIM.Scopes, cfg.SCIM.Org)
	} else {
		slog.Warn("SCIM internal org is not configured; users will be provisioned without an Asgardeo uuid, " +
			"risk notifications will have no deliverable recipients, and the Risk Owner / " +
			"Management Approver pickers will return no candidates")
	}
	if cfg.SCIM.ExternalConfigured() {
		scimExternalClient = scim.NewExternalClient(cfg.SCIM.BaseURL, cfg.SCIM.ExternalTokenURL,
			cfg.SCIM.ExternalClientID, cfg.SCIM.ExternalClientSecret, cfg.SCIM.ExternalScopes, cfg.SCIM.ExternalOrg)
	} else {
		slog.Warn("SCIM external org is not configured; external auditor identities will not resolve")
	}

	// One directory for the whole process, so its cache is shared across every
	// request rather than rebuilt per call site.
	dirSvc := directory.NewWithExternal(scimClient, scimExternalClient, directory.DefaultTTL, directory.DefaultExternalTTL)
	if scimClient != nil && cfg.SCIM.UserDomain != "" {
		// Warms a bulk snapshot of everyone in the domain so Lookup/LookupAll
		// serve them without a per-uuid SCIM call, refreshing it every 12h.
		// Anyone outside the domain (or if this hasn't refreshed yet) still
		// resolves through the per-uuid TTL cache below.
		//
		// Off the startup path: StartBulkRefresh's first fetch is synchronous,
		// and this directory is remote and can cold-start slowly. Blocking
		// here would delay binding the listener (and /health) on an external
		// dependency the caller already tolerates being unready for.
		go dirSvc.StartBulkRefresh(ctx, cfg.SCIM.UserDomain, directory.DefaultBulkRefreshInterval)
	}

	userDeps := userhandler.Deps{
		Users:    userentity.NewRepository(entityCli),
		HREntity: hrClient,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	userhandler.RegisterRoutes(mux, userDeps)
	riskDeps := buildRiskDeps(entityCli, fileSvc, hrClient, grantRepo, dirSvc, scimClient, cfg.Email)
	riskhandler.RegisterRoutes(mux, riskDeps)
	// The HR client reaches the audit module only when lead escalation is on;
	// nil otherwise, so no line manager is ever resolved.
	var auditHRClient *hrentity.Client
	if cfg.AuditLeadEscalationEnabled {
		auditHRClient = hrClient
	}
	auditDeps := buildAuditDeps(fileSvc, entityCli, cfg.AIValidation, cfg.Email, grantRepo, dirSvc, auditHRClient)
	reminderJob := auditjob.NewReminderJob(auditDeps.Audit, auditDeps.Control, auditDeps.Notification, auditDeps.SendReminderDigestSync)
	if cfg.AuditLeadEscalationEnabled {
		reminderJob = reminderJob.WithLeadAlerts(auditDeps.ResolveOwnerLeads, auditDeps.SendOverdueLeadDigestSync)
		slog.Info("audit overdue lead escalation enabled")
	}
	auditDeps.TriggerReminderJob = reminderJob.RunOnce
	audithandler.RegisterRoutes(mux, auditDeps)
	adminhandler.RegisterRoutes(mux, adminhandler.Deps{
		Admin:     adminentity.NewRepository(entityCli),
		Users:     userDeps.Users,
		Grants:    grantRepo,
		Directory: dirSvc,
	})

	// Daily overdue-risk escalation. This lives here rather than in the
	// compliance-entity because escalation now resolves line managers from the
	// HR entity and sends email — neither of which the entity has a client for.
	// It shares the handler's own notifier so an automatic escalation notifies
	// exactly as a manual one does.
	jobCtx, jobCancel := context.WithCancel(context.Background())
	defer jobCancel()
	go riskjob.NewEscalationJob(riskDeps.Risk, riskDeps.Escalation, riskDeps.NotifyEscalationSync).Start(jobCtx)

	// Daily audit due-date reminder digest — fixed 08:00 UTC send time (not a
	// 24h-since-boot ticker like the escalation job above), so the digest
	// arrives at a predictable time regardless of server restarts.
	go reminderJob.Start(jobCtx)
	handler := middleware.SecurityHeaders(
		middleware.CORS(cfg.CORSAllowedOrigin)(
			middleware.CorrelationID(
				middleware.Logger(
					middleware.Auth(middleware.Config{
						IdPs:                  cfg.Auth.IdPs,
						ClockSkew:             cfg.Auth.ClockSkew,
						TokenValidatorEnabled: cfg.Auth.TokenValidatorEnabled,
						PrivilegeStore:        privStore,
						Grants:                grantRepo,
					})(
						mux,
					),
				),
			),
		),
	)

	ln, err := net.Listen("tcp", cfg.Port)
	if err != nil {
		slog.Error("failed to bind", "addr", cfg.Port, "err", err)
		os.Exit(1)
	}
	slog.Info("server started", "addr", cfg.Port)

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server exited unexpectedly", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
