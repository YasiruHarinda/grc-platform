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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
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
	if cfg.Auth.TokenValidatorEnabled {
		privStore, err = privilege.New(ctx, entityCli)
		if err != nil {
			slog.Error("failed to load privilege mapping from the Compliance Entity", "err", err)
			os.Exit(1)
		}
		slog.Info("privilege store loaded")
	}

	hrClient := hrentity.NewClient(cfg.HREntity.GraphQLURL, cfg.HREntity.TokenURL, cfg.HREntity.ClientID, cfg.HREntity.ClientSecret)
	scimClient := scim.NewClient(cfg.SCIM.BaseURL, cfg.SCIM.TokenURL, cfg.SCIM.ClientID, cfg.SCIM.ClientSecret, cfg.SCIM.Scopes)

	userDeps := userhandler.Deps{
		Users:    userentity.NewRepository(entityCli),
		HREntity: hrClient,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	userhandler.RegisterRoutes(mux, userDeps)
	riskhandler.RegisterRoutes(mux, buildRiskDeps(entityCli, fileSvc, hrClient, scimClient, cfg.Email))
	audithandler.RegisterRoutes(mux, buildAuditDeps(fileSvc, entityCli, cfg.AIValidation))

	// Scope guard runs just inside Auth: an evidence-app-scoped token (IdP-2) is
	// confined to /api/v1/evidence-app/* — 403 on any other route.
	handler := middleware.SecurityHeaders(
		middleware.CORS(cfg.CORSAllowedOrigin)(
			middleware.CorrelationID(
				middleware.Logger(
					middleware.Auth(middleware.Config{
						IdPs:                  cfg.Auth.IdPs,
						ClockSkew:             cfg.Auth.ClockSkew,
						TokenValidatorEnabled: cfg.Auth.TokenValidatorEnabled,
						PrivilegeStore:        privStore,
					})(
						middleware.IssuerScope(mux),
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
