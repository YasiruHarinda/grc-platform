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

// Matches risk.risk_quarter ENUM in risk_schema.sql
export type Quarter = "Q1" | "Q2" | "Q3" | "Q4";

// Matches risk.identified_by_type ENUM in risk_schema.sql
export type IdentifiedByType = "EMPLOYEE" | "EXTERNAL_PERSON" | "TOOL";

// Matches risk_score.likelihood / risk_score.impact (1–3 each).
export type LikelihoodLevel = 1 | 2 | 3;
export type ImpactLevel     = 1 | 2 | 3;

// Matches risk_score.risk_level ENUM in risk_schema.sql.
export type RiskLevel = "LOW" | "MEDIUM" | "HIGH";

// Matches risk.treatment_strategy ENUM in risk_schema.sql.
export type TreatmentStrategy = "MITIGATE" | "ACCEPT" | "TRANSFER" | "VOID";

export interface ActionStep {
  description: string;
}

import type { EvidenceAttachment } from "@components/evidence-attachments/EvidenceAttachments";
export type { EvidenceAttachment };

export interface AddRiskFormValues {
  // ── Step 1: Basic Information ─────────────────────────────────────────────
  year: number;
  quarter: Quarter;
  // Integer ID of the selected risk_team row (source register).
  // Fetched from GET /api/v1/teams?type=SOURCE_REGISTER.
  sourceRegister: number | "";
  riskTitle: string;
  riskDescription: string;
  // Array of risk_security_compliance_reference IDs.
  // Fetched from GET /api/v1/compliance-references.
  complianceReferences: number[];
  identifiedByType: IdentifiedByType;
  // User ID — used when identifiedByType = EMPLOYEE. Fetched from GET /api/v1/users.
  identifiedByEmployee: number | "";
  // Free-text name — used when identifiedByType = EXTERNAL_PERSON | TOOL.
  identifiedByName: string;
  // User ID of the risk assigner. Defaults to current user. Fetched from GET /api/v1/users.
  assignedBy: number | "";
  riskIdentifiedDate: Date | null;

  // ── Step 2: Risk Assessment ───────────────────────────────────────────────
  likelihood: LikelihoodLevel | null;
  impact: ImpactLevel | null;
  impactDescription: string;
  implementationDate: Date | null;
  reassessmentDate: Date | null;

  // ── Step 3: Action Plan ───────────────────────────────────────────────────
  // Integer ID of the assignment risk_team row. Fetched from GET /api/v1/teams?type=ASSIGNMENT.
  assignmentTeam: number | "";
  // User ID of the risk owner. Fetched from GET /api/v1/users.
  riskOwner: number | "";
  // User ID of the action owner. Fetched from GET /api/v1/users.
  actionOwner: number | "";
  actionPlanDescription: string;
  actionSteps: ActionStep[];
  treatmentStrategy: TreatmentStrategy | "";
  progress: string;
  gitIssueUrl: string;
  emailSubject: string;
  remarks: string;
  // TODO: POST attachments to /api/v1/risks/{id}/evidence after risk creation (backend endpoint not yet implemented)
  evidenceAttachments: EvidenceAttachment[];
}
