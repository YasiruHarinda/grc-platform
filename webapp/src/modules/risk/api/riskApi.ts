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

import { BACKEND_BASE_URL } from "@config/apiConfig";
import type { AddRiskFormValues } from "../pages/add-risk/types";

// ── Response types (mirror Go models) ─────────────────────────────────────────

export interface RiskTeam {
  id: number;
  name: string;
  code: string | null;
  description: string | null;
  team_type: string;
  status: string;
}

export interface RiskScore {
  id: number;
  likelihood: number;
  impact: number;
  risk_rating: number;
  risk_level: "LOW" | "MEDIUM" | "HIGH";
  color_code: string;
}

export interface ComplianceReference {
  id: number;
  name: string;
  description: string | null;
}

export interface UserOption {
  id: number;
  display_name: string;
  email: string;
}

export interface CreateRiskResponse {
  id: number;
  risk_code: string;
}

export interface NextSequenceIDResponse {
  next_sequence_id: number;
}

// ── API functions ──────────────────────────────────────────────────────────────

type AuthFetch = (input: RequestInfo | URL, options?: RequestInit) => Promise<Response>;

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = Object.assign(new Error(body.message ?? res.statusText), {
      status: res.status,
      data: body,
    });
    throw err;
  }
  return res.json() as Promise<T>;
}

export async function fetchSourceRegisterTeams(authFetch: AuthFetch): Promise<RiskTeam[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/teams?type=SOURCE_REGISTER`);
  return handleResponse<RiskTeam[]>(res);
}

export async function fetchAssignmentTeams(authFetch: AuthFetch): Promise<RiskTeam[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/teams?type=ASSIGNMENT`);
  return handleResponse<RiskTeam[]>(res);
}

export async function fetchRiskScores(authFetch: AuthFetch): Promise<RiskScore[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risk-scores`);
  return handleResponse<RiskScore[]>(res);
}

export async function fetchComplianceReferences(authFetch: AuthFetch): Promise<ComplianceReference[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/compliance-references`);
  return handleResponse<ComplianceReference[]>(res);
}

export async function fetchUsers(authFetch: AuthFetch): Promise<UserOption[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/users`);
  return handleResponse<UserOption[]>(res);
}

export async function fetchNextSequenceID(
  authFetch: AuthFetch,
  sourceRegisterID: number,
  year: number,
  quarter: string,
): Promise<number> {
  const params = new URLSearchParams({
    source_register_id: String(sourceRegisterID),
    year: String(year),
    quarter,
  });
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/next-sequence-id?${params}`);
  const data = await handleResponse<NextSequenceIDResponse>(res);
  return data.next_sequence_id;
}

// ── Build the POST /api/v1/risks payload from the form values ──────────────────

export function buildCreateRiskPayload(data: AddRiskFormValues): Record<string, unknown> {
  return {
    year: data.year,
    quarter: data.quarter,
    source_register_id: data.sourceRegister,
    risk_title: data.riskTitle,
    risk_description: data.riskDescription,
    compliance_reference_ids: data.complianceReferences,
    identified_by_type: data.identifiedByType,
    ...(data.identifiedByType === "EMPLOYEE"
      ? { identified_by_user_id: data.identifiedByEmployee !== "" ? data.identifiedByEmployee : undefined }
      : { identified_by_name: data.identifiedByName !== "" ? data.identifiedByName : undefined }),
    assigner_id: data.assignedBy,
    risk_identified_date: data.riskIdentifiedDate
      ? formatDate(data.riskIdentifiedDate)
      : undefined,
    likelihood: data.likelihood,
    impact: data.impact,
    impact_description: data.impactDescription,
    implementation_date: data.implementationDate
      ? formatDate(data.implementationDate)
      : undefined,
    reassessment_date: data.reassessmentDate
      ? formatDate(data.reassessmentDate)
      : undefined,
    assignment_team_id: data.assignmentTeam,
    owner_id: data.riskOwner,
    action_owner_id: data.actionOwner,
    action_plan_description: data.actionPlanDescription,
    action_steps: data.actionSteps.map((s) => ({ description: s.description })),
    treatment_strategy: data.treatmentStrategy,
    progress: data.progress || undefined,
    git_issue_url: data.gitIssueUrl || undefined,
    email_subject: data.emailSubject,
    remarks: data.remarks || undefined,
  };
}

export async function createRisk(
  authFetch: AuthFetch,
  data: AddRiskFormValues,
): Promise<CreateRiskResponse> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks`, {
    method: "POST",
    body: JSON.stringify(buildCreateRiskPayload(data)),
  });
  return handleResponse<CreateRiskResponse>(res);
}

function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10); // YYYY-MM-DD
}
