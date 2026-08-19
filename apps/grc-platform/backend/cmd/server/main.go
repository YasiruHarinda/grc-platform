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

	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
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

	// Load the role→privilege mapping from the Compliance Entity. Built after
	// entityCli because it needs it.
	// When TokenValidatorEnabled=false (local dev), skip loading — HasPrivilege returns true for all checks.
	// When TokenValidatorEnabled=true (production), load is required — exit if it fails.
	// privilege.New bounds the initial load itself; ctx here governs only the
	// lifetime of its background refresh.
	var privStore *privilege.Store
	var grantRepo grant.Repository
	if cfg.Auth.TokenValidatorEnabled {
		privStore, err = privilege.New(ctx, entityCli)
		if err != nil {
			slog.Error("failed to load privilege mapping from the Compliance Entity", "err", err)
			os.Exit(1)
		}
		slog.Info("privilege store loaded")

		// Grants are read from the entity on every request and never cached:
		// role→privilege changes only on a deploy (hence privStore's 15-minute
		// refresh above), but a revoked grant must take effect immediately.
		grantRepo = grant.NewRepository(entityCli)
	}

	hrClient := hrentity.NewClient(cfg.HREntity.GraphQLURL, cfg.HREntity.TokenURL, cfg.HREntity.ClientID, cfg.HREntity.ClientSecret)

	// The identity directory. Left nil when unconfigured, which the client
	// tolerates by answering "no such user" — see scim.Client.LookupByEmail.
	// Local development without credentials for this internal service then
	// provisions users without a uuid instead of failing.
	var scimClient *scim.Client
	if cfg.SCIM.Configured() {
		scimClient = scim.NewClient(cfg.SCIM.BaseURL, cfg.SCIM.TokenURL,
			cfg.SCIM.ClientID, cfg.SCIM.ClientSecret, cfg.SCIM.Scopes)
	} else {
		slog.Warn("SCIM is not configured; users will be provisioned without an Asgardeo uuid, " +
			"risk notifications will have no deliverable recipients, and the Risk Owner / " +
			"Management Approver pickers will return no candidates")
	}

	// One directory for the whole process, so its cache is shared across every
	// request rather than rebuilt per call site.
	dirSvc := directory.New(scimClient, directory.DefaultTTL)
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
		Users:     userentity.NewRepository(entityCli),
		HREntity:  hrClient,
		SCIM:      scimClient,
		Directory: dirSvc,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	userhandler.RegisterRoutes(mux, userDeps)
	riskDeps := buildRiskDeps(entityCli, fileSvc, hrClient, grantRepo, dirSvc, scimClient, cfg.Email)
	riskhandler.RegisterRoutes(mux, riskDeps)
	audithandler.RegisterRoutes(mux, buildAuditDeps(fileSvc, entityCli, cfg.AIValidation))

	// Daily overdue-risk escalation. This lives here rather than in the
	// compliance-entity because escalation now resolves line managers from the
	// HR entity and sends email — neither of which the entity has a client for.
	// It shares the handler's own notifier so an automatic escalation notifies
	// exactly as a manual one does.
	jobCtx, jobCancel := context.WithCancel(context.Background())
	defer jobCancel()
	go riskjob.NewEscalationJob(riskDeps.Risk, riskDeps.Escalation, riskDeps.NotifyEscalationSync).Start(jobCtx)

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
