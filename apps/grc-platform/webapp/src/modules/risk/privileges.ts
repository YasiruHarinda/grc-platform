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

// Risk Hub privilege name constants. All prefixed RISK_ so they group together
// (visually and alphabetically) apart from the Audit Hub's privileges.
// Values must match privilege_name in the privilege table and the constants in
// backend/internal/shared/privilege/privilege.go exactly.
export const RiskPrivilege = {
  ViewRisks:             "RISK_VIEW_RISKS",
  // ViewAllRisks grants org-wide read visibility without edit/approve rights
  // — see backend/internal/shared/privilege/privilege.go for details.
  ViewAllRisks:          "RISK_VIEW_ALL_RISKS",
  // ViewRiskDashboard gates the Dashboard nav item/route specifically —
  // distinct from ViewRisks (which gates the Registers list) so an Action
  // Owner can hold list access without also getting the dashboard.
  ViewRiskDashboard:     "RISK_VIEW_DASHBOARD",
  CreateRisk:            "RISK_CREATE",
  UpdateRisk:            "RISK_UPDATE",
  SubmitRisk:            "RISK_SUBMIT",
  CancelRisk:            "RISK_CANCEL",
  OwnerApproveRisk:      "RISK_OWNER_APPROVE",
  ManagementApproveRisk: "RISK_MANAGEMENT_APPROVE",
  ComplianceApproveRisk: "RISK_COMPLIANCE_APPROVE",
  OwnerRejectRisk:       "RISK_OWNER_REJECT",
  ManagementRejectRisk:  "RISK_MANAGEMENT_REJECT",
  ComplianceRejectRisk:  "RISK_COMPLIANCE_REJECT",
  CompleteRisk:          "RISK_COMPLETE",
  CloseRisk:             "RISK_CLOSE",
  EscalateRisk:          "RISK_ESCALATE",
  AssessRisk:            "RISK_ASSESS",
  ManageTeams:           "RISK_MANAGE_TEAMS",
  ManageRiskScores:      "RISK_MANAGE_SCORES",
  ManageActionPlans:     "RISK_MANAGE_ACTION_PLANS",
  ManageComplianceRefs:  "RISK_MANAGE_COMPLIANCE_REFS",
  ViewAnalytics:         "RISK_VIEW_ANALYTICS",
  // RETIRED alongside the MANAGEMENT action plan itself: an escalation is now
  // answered with a comment, and additional plans are created by the Risk
  // Assigner under ManageActionPlans. Seeded INACTIVE server-side, so it
  // resolves for nobody — nothing should check it.
  CreateManagementActionPlan: "RISK_CREATE_MANAGEMENT_ACTION_PLAN",
  // RETIRED with the action-owner role. Completing a plan's steps is authorised
  // by being its action_owner_id — the identity, not a privilege — because an
  // Action Owner may be any employee and hold no role at all. Seeded INACTIVE
  // server-side, so it resolves for nobody: anything gating on it renders never.
  CompleteActionSteps:        "RISK_COMPLETE_ACTION_STEPS",
} as const;
