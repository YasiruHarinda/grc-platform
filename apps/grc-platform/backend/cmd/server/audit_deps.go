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
	"log/slog"

	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository/entity"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aiagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
)

// buildAuditDeps wires Audit Hub dependencies. The audit module now reads/writes
// ALL data through the Compliance Entity (via ec) — no direct MySQL access.
//
// TriggerReminderJob is deliberately left unset here — it needs a reference to
// the reminder job (internal/audit/job), which itself needs Control/Audit/
// Notification from the Deps this function returns, so main.go wires it in
// after calling this function and before RegisterRoutes.
// hrClient is nil unless lead-escalation emails are enabled
// (LEAD_ESCALATION_EMAILS_ENABLED), which is what keeps the disabled build from
// resolving any lead (line manager) at all.
func buildAuditDeps(fileSvc *file.Service, ec *entityclient.Client, aiCfg config.AIValidationConfig, emailCfg config.EmailConfig, grantRepo grant.Repository, dirSvc *directory.Service, hrClient *hrentity.Client, activityLog *adminactivity.Client) audithandler.Deps {
	// ── Repositories (all Compliance Entity) ──────────────────────────────────
	auditRepo := auditentity.NewAuditRepository(ec)
	frameworkRepo := auditentity.NewFrameworkRepository(ec)
	frameworkControlRepo := auditentity.NewFrameworkControlRepository(ec)
	productRepo := auditentity.NewProductRepository(ec)
	userRepo := auditentity.NewUserRepository(ec)
	teamRepo := auditentity.NewTeamRepository(ec)
	commentRepo := auditentity.NewCommentRepository(ec)
	controlRepo := auditentity.NewControlRepository(ec)
	evidenceRepo := auditentity.NewEvidenceRepository(ec)
	populationRepo := auditentity.NewPopulationRepository(ec)
	trailRepo := auditentity.NewTrailRepository(ec)
	dashboardRepo := auditentity.NewDashboardRepository(ec)
	aiValidationRepo := auditentity.NewAIValidationRepository(ec)
	notificationRepo := auditentity.NewNotificationRepository(ec)

	// ── Services ──────────────────────────────────────────────────────────────
	// trailSvc is built first: both the audit and control services record
	// lifecycle events through it.
	trailSvc := auditservice.NewTrailService(trailRepo)
	auditSvc := auditservice.NewAuditService(auditRepo, frameworkRepo, productRepo, trailSvc)
	controlSvc := auditservice.NewControlService(controlRepo, populationRepo, trailSvc, dirSvc)
	frameworkSvc := auditservice.NewFrameworkService(frameworkRepo, productRepo, frameworkControlRepo)
	userSvc := auditservice.NewUserService(userRepo)
	teamSvc := auditservice.NewTeamService(teamRepo)
	dashboardSvc := auditservice.NewDashboardService(dashboardRepo, dirSvc)
	evidenceSvc := auditservice.NewEvidenceService(evidenceRepo, auditRepo, controlRepo, fileSvc)
	populationSvc := auditservice.NewPopulationService(populationRepo, fileSvc)
	commentSvc := auditservice.NewCommentService(commentRepo)
	aiValidationSvc := auditservice.NewAIValidationService(aiValidationRepo)
	notificationSvc := auditservice.NewNotificationService(notificationRepo)

	// AI validation trigger client — only when explicitly enabled per env.
	var aiAgent *aiagent.Client
	if aiCfg.Enabled {
		if aiCfg.AgentAPIKey == "" {
			slog.Error("AI_VALIDATION_ENABLED=true but AI_AGENT_API_KEY is not set — disabling AI validation to avoid per-request 401s")
		} else {
			aiAgent = aiagent.New(aiCfg.AgentBaseURL, aiCfg.AgentAPIKey)
		}
	}

	return audithandler.Deps{
		Audit:        auditSvc,
		Control:      controlSvc,
		Framework:    frameworkSvc,
		User:         userSvc,
		Team:         teamSvc,
		Dashboard:    dashboardSvc,
		Evidence:     evidenceSvc,
		Population:   populationSvc,
		Trail:        trailSvc,
		Comment:      commentSvc,
		Notification: notificationSvc,
		AIValidation: aiValidationSvc,
		AIAgent:      aiAgent,
		// Reuses the same email-service credentials already loaded for risk —
		// one email-service client for the whole backend, no new env vars.
		Email:           emailer.New(emailCfg.ServiceURL, emailCfg.FromAddress, emailCfg.TokenURL, emailCfg.ClientID, emailCfg.ClientSecret, emailCfg.Enabled),
		Users:           userRepo,
		Directory:       dirSvc,
		Grants:          grantRepo,
		FrontendBaseURL: emailCfg.FrontendBaseURL,
		HR:              hrClient,
		ActivityLog:     activityLog,
		// Review, Assignment are wired here as their implementations are added.
	}
}
