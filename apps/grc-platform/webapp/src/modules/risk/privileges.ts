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

// Risk Hub privilege name constants.
// Values must match privilege_name in the privilege table and the constants in
// backend/internal/shared/privilege/privilege.go exactly.
export const RiskPrivilege = {
  ViewRisks:             "VIEW_RISKS",
  CreateRisk:            "CREATE_RISK",
  UpdateRisk:            "UPDATE_RISK",
  SubmitRisk:            "SUBMIT_RISK",
  CancelRisk:            "CANCEL_RISK",
  OwnerApproveRisk:      "OWNER_APPROVE_RISK",
  ManagementApproveRisk: "MANAGEMENT_APPROVE_RISK",
  ComplianceApproveRisk: "COMPLIANCE_APPROVE_RISK",
  OwnerRejectRisk:       "OWNER_REJECT_RISK",
  ManagementRejectRisk:  "MANAGEMENT_REJECT_RISK",
  ComplianceRejectRisk:  "COMPLIANCE_REJECT_RISK",
  CompleteRisk:          "COMPLETE_RISK",
  CloseRisk:             "CLOSE_RISK",
  EscalateRisk:          "ESCALATE_RISK",
  AssessRisk:            "ASSESS_RISK",
  ManageTeams:           "MANAGE_TEAMS",
  ManageRiskScores:      "MANAGE_RISK_SCORES",
  ManageActionPlans:     "MANAGE_ACTION_PLANS",
  ManageComplianceRefs:  "MANAGE_COMPLIANCE_REFS",
  ViewAnalytics:         "VIEW_ANALYTICS",
  CreateManagementActionPlan: "CREATE_MANAGEMENT_ACTION_PLAN_RISK",
  CompleteActionSteps:        "COMPLETE_ACTION_STEPS_RISK",
} as const;
