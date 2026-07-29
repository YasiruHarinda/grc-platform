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

export type AuditStatus = "ACTIVE" | "COMPLETED" | "ARCHIVED" | "REMOVED";

export type ControlSource = "MANUAL" | "COPIED" | "CSV";

export type ControlStatus =
  // ── OE population phase ───────────────────────────────────────────────────
  | "POPULATION_PENDING"           // OE default — team must submit population
  | "POPULATION_INTERNAL_REVIEW"   // OE — team submitted, compliance reviewing
  | "POPULATION_UNDER_VALIDATION"  // OE — compliance approved, auditor reviewing
  | "POPULATION_NEED_CLARIFICATION"// OE — auditor rejected, team must resubmit
  | "POPULATION_COMPLETE"          // OE — auditor approved population
  | "AWAITING_SAMPLE"              // OE — auditor needs time to submit sample
  | "SUBMITTED_SAMPLE"             // OE — auditor submitted sample, team submits evidence
  // ── Evidence phase (Design default; OE after sample) ──────────────────────
  | "EVIDENCE_PENDING"             // Design default / OE after rejection cycle
  | "EVIDENCE_INTERNAL_REVIEW"     // Both — team submitted, compliance reviewing
  | "EVIDENCE_UNDER_VALIDATION"    // Both — compliance approved, auditor reviewing
  | "EVIDENCE_NEED_CLARIFICATION"  // Both — auditor rejected, team must resubmit
  | "COMPLETE";                    // Both — auditor approved, control closed

export type RequirementType = "DESIGN" | "OE";
export type ControlType = "CONFIG" | "NON_CONFIG";
export type ControlScope = "COMMON" | "PRODUCT_SPECIFIC";

export interface AuditFramework {
  id: number;
  name: string;
  status?: string;
}

export interface AuditFrameworkControl {
  id: number;
  frameworkId: number;
  controlNumber: string;
  description: string;
  evidenceRequirement: string | null;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
  version: number;
  isCurrent: boolean;
}

export interface FrameworkControlListResponse {
  controls: AuditFrameworkControl[];
  total: number;
}

export interface CreateFrameworkControlRequest {
  controlNumber: string;
  description: string;
  evidenceRequirement?: string | null;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
}

export interface AuditProduct {
  id: number;
  name: string;
}

export interface AuditTeam {
  id: number;
  name: string;
}

export interface ControlCounts {
  total: number;
  approved: number;
  overdue: number;
}

export interface Audit {
  id: number;
  name: string;
  framework: AuditFramework;
  product: AuditProduct;
  periodStart: string;
  periodEnd: string;
  status: AuditStatus;
  scopeDescription: string | null;
  controlCounts: ControlCounts;
  createdAt: string;
  updatedAt: string;
}

export interface PopulationSample {
  id: number;
  reference: string;
  description: string;
}

export interface AuditControl {
  id: number;
  auditId: number;
  ownerId: number | null;
  ownerName: string | null;
  teamId: number | null;
  teamName: string | null;
  auditorId: number | null;
  auditorName: string | null;
  // Backs the assigned-auditor gate: population validation, sample selection, and
  // evidence validation are shown only when this matches the signed-in user's email.
  auditorEmail: string | null;
  controlNumber: string;
  description: string;
  evidenceRequirement: string | null;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
  dueDate: string | null;
  status: ControlStatus;
  controlSource: ControlSource;
  sampleReference: string | null;
  comments: string | null;
  isOverdue: boolean;
  createdAt: string;
  updatedAt: string;
  samples?: PopulationSample[];
  // Population-phase fields (OE controls). Joined from the initial audit_population record.
  populationDescription?: string | null;
  populationComments?: string | null;
  populationDueDate?: string | null;
  populationOwnerName?: string | null;
  populationTeamName?: string | null;
}

export interface AuditListResponse {
  items: Audit[];
  total: number;
}

export interface ControlListResponse {
  items: AuditControl[];
  total: number;
}

// ── Request types (sent to backend) ──────────────────────────────────────────

export interface CreateAuditRequest {
  name: string;
  frameworkId: number;
  productId: number;
  periodStart: string;
  periodEnd: string;
  scopeDescription?: string | null;
}

export interface PopulationDetails {
  description: string;
  referenceNumber?: number | null;
  dueDate?: string | null;
  comments?: string | null;
  /** Population-phase process owner (may differ from the control's process owner). */
  ownerId?: number | null;
  /** Population-phase team (may differ from the control's team). */
  teamId?: number | null;
}

// Always creates a standalone control with full definition text — there is no
// framework-linked shape. pushToFramework optionally also writes the control
// into the framework's catalog as a side effect: a first version when
// sourceFrameworkControlId is unset, or a new version of that existing
// catalog control when it's set (edited-existing-control push-back). See
// docs/new/Audit-Control-Framework-Optional-Design.md §6.
export interface AddControlRequest {
  controlNumber: string;
  description: string;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
  evidenceRequirement?: string | null;
  dueDate?: string | null;
  ownerId?: number | null;
  teamId?: number | null;
  auditorId?: number | null;
  controlSource?: ControlSource;
  population?: PopulationDetails | null;
  pushToFramework?: boolean;
  sourceFrameworkControlId?: number;
}

export interface UpdateControlRequest {
  description?: string;
  controlType?: ControlType;
  scope?: ControlScope;
  evidenceRequirement?: string | null;
  dueDate?: string | null;
  ownerId?: number | null;
  teamId?: number | null;
  auditorId?: number | null;
}
