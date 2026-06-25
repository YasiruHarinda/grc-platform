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
	"database/sql"

	riskhandler "github.com/wso2-open-operations/grc-platform/backend/internal/risk/handler"
	riskmysql "github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository/mysql"
	riskservice "github.com/wso2-open-operations/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-platform/backend/internal/shared/file"
)

// buildRiskDeps wires the full Risk Hub dependency graph:
// MySQL repositories → services → handler Deps struct.
// file is the shared Azure Blob service used by evidence uploads.
func buildRiskDeps(db *sql.DB, fileSvc *file.Service) riskhandler.Deps {
	riskRepo       := riskmysql.NewRiskRepository(db)
	teamRepo       := riskmysql.NewTeamRepository(db)
	scoreRepo      := riskmysql.NewRiskScoreRepository(db)
	actionPlanRepo := riskmysql.NewActionPlanRepository(db)
	evidenceRepo   := riskmysql.NewRiskEvidenceRepository(db)
	escalationRepo := riskmysql.NewEscalationRepository(db)
	notifRepo      := riskmysql.NewNotificationRepository(db)
	complianceRepo := riskmysql.NewComplianceReferenceRepository(db)
	analyticsRepo  := riskmysql.NewAnalyticsRepository(db)

	return riskhandler.Deps{
		Risk:         riskservice.NewRiskService(riskRepo),
		Team:         riskservice.NewTeamService(teamRepo),
		Score:        riskservice.NewRiskScoreService(scoreRepo),
		ActionPlan:   riskservice.NewActionPlanService(actionPlanRepo),
		Evidence:     riskservice.NewEvidenceService(evidenceRepo, fileSvc),
		Escalation:   riskservice.NewEscalationService(escalationRepo),
		Notification: riskservice.NewNotificationService(notifRepo),
		Compliance:   riskservice.NewComplianceReferenceService(complianceRepo),
		Analytics:    riskservice.NewAnalyticsService(analyticsRepo),
	}
}
