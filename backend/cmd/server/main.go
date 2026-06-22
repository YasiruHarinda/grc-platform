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

	audithandler "github.com/wso2-open-operations/grc-platform/backend/internal/audit/handler"
	"github.com/wso2-open-operations/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-platform/backend/internal/db"
	"github.com/wso2-open-operations/grc-platform/backend/internal/middleware"
	riskhandler "github.com/wso2-open-operations/grc-platform/backend/internal/risk/handler"
	"github.com/wso2-open-operations/grc-platform/backend/internal/shared/privilege"
	userhandler "github.com/wso2-open-operations/grc-platform/backend/internal/user/handler"
)

func main() {
	middleware.ConfigureLogger()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "err", err)
		os.Exit(1)
	}

	sqlDB, err := db.Connect(cfg.DB.DSN)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	// Load the role→privilege mapping from the database.
	// When TokenValidatorEnabled=false (local dev), skip loading — HasPrivilege returns true for all checks.
	// When TokenValidatorEnabled=true (production), load is required — exit if it fails.
	var privStore *privilege.Store
	if cfg.Auth.TokenValidatorEnabled {
		loadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		privStore, err = privilege.New(loadCtx, sqlDB)
		if err != nil {
			slog.Error("failed to load privilege mapping from database", "err", err)
			os.Exit(1)
		}
		slog.Info("privilege store loaded")
	}

	// TODO: wire up the three layers in order:
	//   1. Repositories  → internal/risk/repository/mysql/  + internal/audit/repository/mysql/ + internal/user/mysql/
	//   2. Services      → internal/risk/service/           + internal/audit/service/
	//   3. Handler Deps  → pass services into riskhandler.Deps{} and audithandler.Deps{} below

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	userhandler.RegisterRoutes(mux)
	riskhandler.RegisterRoutes(mux, riskhandler.Deps{})
	audithandler.RegisterRoutes(mux, audithandler.Deps{})

	handler := middleware.CorrelationID(
		middleware.Logger(
			middleware.Auth(middleware.Config{
				JWKSEndpoint:          cfg.Auth.JWKSEndpoint,
				Issuer:                cfg.Auth.Issuer,
				Audience:              cfg.Auth.Audience,
				ClockSkew:             cfg.Auth.ClockSkew,
				TokenValidatorEnabled: cfg.Auth.TokenValidatorEnabled,
				PrivilegeStore:        privStore,
			})(mux),
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
