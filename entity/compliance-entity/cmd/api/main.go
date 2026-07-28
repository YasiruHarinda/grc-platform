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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/config"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/db"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/job"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/server"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/storage"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	pool, err := db.New(cfg)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	// Azure Blob storage — the entity is the only component holding the account
	// key. Disabled (nil) when unconfigured (local dev / metadata-only tests).
	var store *storage.Service
	if cfg.AzureConfigured() {
		store = storage.NewService(storage.Config{
			AccountName:   cfg.AzureAccountName,
			AccountKey:    cfg.AzureAccountKey,
			ContainerName: cfg.AzureContainerName,
		})
		log.Printf("Azure Blob storage enabled (container %q)", cfg.AzureContainerName)
	} else {
		log.Printf("Azure Blob storage NOT configured — file endpoints disabled")
	}

	addr := ":" + cfg.ServerPort
	srv := server.New(addr, pool, store)

	go func() {
		log.Printf("Compliance Entity REST Service started on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Daily overdue-risk escalation job. Runs against its own repo/service
	// instances (cheap — thin wrappers over the same pool) rather than
	// reaching into NewRouter's, which aren't exposed outside server.New.
	jobCtx, jobCancel := context.WithCancel(context.Background())
	defer jobCancel()
	jobRiskSvc := service.NewRiskService(repository.NewRiskRepository(pool))
	escalationJob := job.NewEscalationJob(
		jobRiskSvc,
		service.NewRiskEscalationService(repository.NewRiskEscalationRepository(pool), jobRiskSvc),
	)
	go escalationJob.Start(jobCtx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Println("server stopped")
}
