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

// The AI validation Validation Agent (threat model [04]). Project-internal
// Choreo component: triggered fire-and-forget by the GRC backend after an
// evidence submission, it runs an Anthropic tool loop against the MCP server
// and records an advisory result. Holds ANTHROPIC_API_KEY only.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/config"
)

func main() {
	cfg, err := config.LoadAgent()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if strings.EqualFold(cfg.LogLevel, "debug") {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	app := agent.New(cfg, logger)
	srv := &http.Server{
		Addr:              normalizePort(cfg.Port),
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("validation agent listening", "addr", srv.Addr, "model", cfg.AnthropicModel)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
	// Let in-flight validation jobs finish before exiting.
	app.Drain(30 * time.Second)
}

// normalizePort accepts "8090" or ":8090".
func normalizePort(p string) string {
	if strings.HasPrefix(p, ":") {
		return p
	}
	return ":" + p
}
