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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	riskentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository/entity"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/entity"
)

// buildRiskDeps wires the full Risk Hub dependency graph:
// repositories → services → handler Deps struct.
//
// Every repository is served by the Compliance Entity; the Risk Hub holds no
// database handle. fileSvc is the shared Azure Blob service used by evidence
// uploads, and hrClient talks to the HR entity's GraphQL service for employee
// lookups — neither is backed by the GRC platform's own database either.
//
// Dashboard and analytics take their payload already assembled: the entity runs
// the aggregate queries and the pivots, so these services pass it through,
// mirroring the audit module.
func buildRiskDeps(
	ec *entityclient.Client,
	fileSvc *file.Service,
	hrClient *hrentity.Client,
) riskhandler.Deps {
	userRepo := userentity.NewRepository(ec)
	actionPlanRepo := riskentity.NewActionPlanRepository(ec)
	return riskhandler.Deps{
		Risk:         riskservice.NewRiskService(riskentity.NewRiskRepository(ec), actionPlanRepo),
		Assessment:   riskservice.NewRiskAssessmentService(riskentity.NewAssessmentRepository(ec)),
		Team:         riskservice.NewTeamService(riskentity.NewTeamRepository(ec)),
		Score:        riskservice.NewRiskScoreService(riskentity.NewRiskScoreRepository(ec)),
		ActionPlan:   riskservice.NewActionPlanService(actionPlanRepo, userRepo),
		Evidence:     riskservice.NewEvidenceService(riskentity.NewRiskEvidenceRepository(ec), fileSvc),
		Escalation:   riskservice.NewEscalationService(riskentity.NewEscalationRepository(ec)),
		Notification: riskservice.NewNotificationService(riskentity.NewNotificationRepository(ec)),
		Compliance:   riskservice.NewComplianceReferenceService(riskentity.NewComplianceReferenceRepository(ec)),
		Analytics:    riskservice.NewAssembledAnalyticsService(riskentity.NewAnalyticsRepository(ec)),
		Dashboard:    riskservice.NewAssembledDashboardService(riskentity.NewDashboardRepository(ec)),
		Employee:     riskservice.NewEmployeeSearchService(hrClient),
		Users:        userRepo,
	}
}
