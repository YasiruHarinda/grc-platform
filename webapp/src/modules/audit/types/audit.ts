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

export type ControlStatus =
  | "SAMPLING_REQUIRED"
  | "NOT_STARTED"
  | "EVIDENCE_SUBMITTED"
  | "COMPLIANCE_REVIEW"
  | "AUDITOR_REVIEW"
  | "APPROVED"
  | "RESUBMIT_REQUIRED";

export type RequirementType = "DESIGN" | "OE";
export type ControlType = "CONFIG" | "NON_CONFIG";
export type ControlScope = "COMMON" | "PRODUCT_SPECIFIC";

export interface AuditFramework {
  id: number;
  name: string;
  version: string | null;
}

export interface AuditProduct {
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

export interface AuditControl {
  id: number;
  auditId: number;
  ownerId: number | null;
  ownerName: string | null;
  teamId: number | null;
  teamName: string | null;
  auditorId: number | null;
  auditorName: string | null;
  controlNumber: string;
  description: string;
  evidenceRequirement: string | null;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
  dueDate: string | null;
  status: ControlStatus;
  sampleReference: string | null;
  comments: string | null;
  isManuallyAdded: boolean;
  isOverdue: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AuditListResponse {
  items: Audit[];
  total: number;
}

export interface ControlListResponse {
  items: AuditControl[];
  total: number;
}
