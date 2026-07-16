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
import { toDateOnlyString } from "@utils/dateTime";
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

// ── Risk Registers types ───────────────────────────────────────────────────────

export interface RiskListItem {
  id: number;
  risk_code: string;
  risk_title: string;
  source_register_name: string;
  risk_level: string;
  risk_level_color: string;
  owner_name: string;
  assigner_name: string;
  workflow_status: string;
  risk_type: string;
  implementation_date: string | null;
  rejection_comment: string | null;
  rejection_stage: string | null;
  created_at: string;
}

export interface RiskScoreInfo {
  id: number;
  likelihood: number;
  impact: number;
  risk_rating: number;
  risk_level: string;
  color_code: string;
}

export interface ActionPlanStep {
  id: number;
  plan_id: number;
  step_no: number;
  description: string;
  status: string;
  completed_date: string | null;
}

export interface ActionPlanDetail {
  id: number;
  action_owner_id: number | null;
  description: string | null;
  status: string;
  plan_type: string;
  steps: ActionPlanStep[];
}

export interface RiskAssessmentRecord {
  id: number;
  risk_id: number;
  score_id: number;
  progress: string;
  reassessment_date: string;
  assessed_by: string;
  created_at: string;
  residual_likelihood: number;
  residual_impact: number;
  residual_rating: number;
  residual_level: string;
  residual_color_code: string;
  // Marks a synthetic entry for the risk's gross score, added by the backend
  // so the log shows the full lineage even though it isn't a real reassessment.
  is_initial?: boolean;
}

export interface RiskDetail {
  id: number;
  risk_code: string;
  risk_year: number;
  risk_quarter: string;
  risk_title: string;
  risk_description: string;
  risk_identified_date: string | null;
  identified_by_type: string | null;
  identified_by_user_id: number | null;
  identified_by_name: string | null;
  assigner_id: number;
  owner_id: number;
  impact_description: string | null;
  treatment_strategy: string | null;
  assignment_team_id: number;
  progress: string | null;
  implementation_date: string | null;
  reassessment_date: string | null;
  git_issue_url: string | null;
  email_subject: string | null;
  remarks: string | null;
  workflow_status: string;
  risk_type: string;
  rejection_comment: string | null;
  rejection_stage: string | null;
  owner_first_approved_at: string | null;
  compliance_approval_date: string | null;
  created_at: string;
  updated_at: string;
  source_register_name: string;
  assignment_team_name: string;
  owner_name: string;
  assigner_name: string;
  identified_by_user_name: string | null;
  compliance_approver_name: string | null;
  // Original rating from creation; immutable once a risk owner has approved
  // the risk. Only EditRiskDialog should read this — for display, use
  // effective_score.
  gross_score: RiskScoreInfo | null;
  // Current residual score: the latest reassessment's score if one exists,
  // else gross_score. This is what headers/tables should display.
  effective_score: RiskScoreInfo | null;
  compliance_references: ComplianceReference[];
  action_plan: ActionPlanDetail | null;
  assessments: RiskAssessmentRecord[];
}

export interface ListRisksParams {
  statuses?: string[];
  team_id?: number[];
  level?: string[];
  search?: string;
  risk_type?: string[];
  owner_id?: number[];
  submitted_from?: string;
  submitted_to?: string;
  due_from?: string;
  due_to?: string;
  due_overdue?: boolean;
  offset?: number;
  limit?: number;
}

export interface RiskListPage {
  items: RiskListItem[];
  total: number;
  offset: number;
  limit: number;
}

export interface UpdateRiskPayload {
  risk_title: string;
  risk_description: string;
  risk_identified_date?: string;
  identified_by_type?: string;
  identified_by_user_id?: number;
  identified_by_name?: string;
  assigner_id?: number;
  owner_id?: number;
  impact_description?: string;
  compliance_reference_ids?: number[];
  progress?: string;
  git_issue_url?: string;
  email_subject?: string;
  remarks?: string;
  reassessment_date?: string;
  gross_score_id?: number;
  implementation_date?: string;
  treatment_strategy?: string;
  assignment_team_id?: number;
  action_plan_description?: string;
  action_owner_id?: number;
  action_steps?: { id?: number; description: string }[];
}

export interface CreateAssessmentPayload {
  likelihood: number;
  impact: number;
  progress: string;
  reassessment_date: string;
}

// ── Dashboard types (mirror model/dashboard.go) ────────────────────────────────

export interface RiskStatusSummary {
  total: number;
  open: number;
  closed: number;
  overdue: number;
}

export interface RegisterTreatmentCount {
  register_name: string;
  treatment_strategy: string;
  count: number;
}

export interface RiskLevelCount {
  risk_level: string;
  color_code: string;
  count: number;
}

export interface HeatmapCell {
  likelihood: number;
  impact: number;
  risk_level: string;
  color_code: string;
  count: number;
}

export interface RegisterCertShare {
  register_name: string;
  cert_name: string;
  count: number;
  percentage: number;
}

export interface RegisterLevelTreatmentCount {
  risk_level: string;
  treatment_strategy: string;
  count: number;
}

export interface RegisterAnalytics {
  register_id: number;
  register_name: string;
  open_count: number;
  heatmap: HeatmapCell[];
  level_counts: RiskLevelCount[];
  level_treatments: RegisterLevelTreatmentCount[];
}

export interface RepeatedRiskOccurrence {
  register_name: string;
  status: "OPEN" | "CLOSED";
  risk_level: string;
  color_code: string;
}

export interface RepeatedComplianceRisk {
  risk_title: string;
  occurrences: RepeatedRiskOccurrence[];
}

export interface HighRiskItem {
  id: number;
  risk_code: string;
  risk_description: string;
  register_name: string;
  owner_name: string;
  identified_date: string | null;
  treatment_strategy: string | null;
  implementation_date: string | null;
}

export interface DashboardSummary {
  summary: RiskStatusSummary;
  treatment_by_register: RegisterTreatmentCount[];
  level_counts: RiskLevelCount[];
  org_heatmap: HeatmapCell[];
  cert_distribution: RegisterCertShare[];
  registers: RegisterAnalytics[];
  repeated_compliance_risks: RepeatedComplianceRisk[];
  high_risks: HighRiskItem[];
}

// ── Analytics types (mirror model/analytics.go) ────────────────────────────────

export interface AnalyticsKPIs {
  new_risks_this_month: number;
  avg_days_to_close: number | null;
  avg_effective_score: number | null;
}

export interface TrendPoint {
  month: string;
  identified_count: number;
  closed_count: number;
  avg_score: number | null;
}

export interface MonthLevelCount {
  month: string;
  risk_level: string;
  color_code: string;
  count: number;
}

export interface MonthRegisterCount {
  month: string;
  register_name: string;
  count: number;
}

export interface RegisterShare {
  register_name: string;
  count: number;
}

export interface ComplianceShare {
  compliance_name: string;
  count: number;
}

export interface TreatmentShare {
  treatment_strategy: string;
  count: number;
}

export interface WorkflowStageCount {
  workflow_status: string;
  count: number;
}

export interface AgingRiskItem {
  id: number;
  risk_code: string;
  risk_title: string;
  register_name: string;
  owner_name: string;
  risk_level: string;
  color_code: string;
  identified_date: string | null;
  age_days: number;
}

export interface AnalyticsSummary {
  kpis: AnalyticsKPIs;
  trend: TrendPoint[];
  level_distribution: MonthLevelCount[];
  identified_by_register: MonthRegisterCount[] | null;
  closed_by_register: MonthRegisterCount[] | null;
  register_shares: RegisterShare[] | null;
  compliance_distribution: ComplianceShare[];
  treatment_mix: TreatmentShare[];
  workflow_funnel: WorkflowStageCount[];
  aging_risks: AgingRiskItem[];
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
  if (res.status === 204) {
    return undefined as T;
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
    source_register_id: data.sourceRegister !== "" ? data.sourceRegister : undefined,
    risk_title: data.riskTitle,
    risk_description: data.riskDescription,
    compliance_reference_ids: data.complianceReferences,
    identified_by_type: data.identifiedByType,
    ...(data.identifiedByType === "EMPLOYEE"
      ? { identified_by_user_id: data.identifiedByEmployee !== "" ? data.identifiedByEmployee : undefined }
      : { identified_by_name: data.identifiedByName !== "" ? data.identifiedByName : undefined }),
    assigner_id: data.assignedBy !== "" ? data.assignedBy : undefined,
    risk_identified_date: toDateOnlyString(data.riskIdentifiedDate),
    likelihood: data.likelihood,
    impact: data.impact,
    impact_description: data.impactDescription,
    implementation_date: toDateOnlyString(data.implementationDate),
    reassessment_date: toDateOnlyString(data.reassessmentDate),
    assignment_team_id: data.assignmentTeam !== "" ? data.assignmentTeam : undefined,
    owner_id: data.riskOwner !== "" ? data.riskOwner : undefined,
    action_owner_id: data.actionOwner !== "" ? data.actionOwner : undefined,
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

export async function fetchRisks(
  authFetch: AuthFetch,
  params: ListRisksParams = {},
): Promise<RiskListPage> {
  const q = new URLSearchParams();
  if (params.statuses?.length) q.set("statuses", params.statuses.join(","));
  if (params.team_id?.length) q.set("team_id", params.team_id.join(","));
  if (params.level?.length) q.set("level", params.level.join(","));
  if (params.search) q.set("search", params.search);
  if (params.risk_type?.length) q.set("risk_type", params.risk_type.join(","));
  if (params.owner_id?.length) q.set("owner_id", params.owner_id.join(","));
  if (params.submitted_from) q.set("submitted_from", params.submitted_from);
  if (params.submitted_to) q.set("submitted_to", params.submitted_to);
  if (params.due_from) q.set("due_from", params.due_from);
  if (params.due_to) q.set("due_to", params.due_to);
  if (params.due_overdue) q.set("due_overdue", "true");
  if (params.offset !== undefined) q.set("offset", String(params.offset));
  if (params.limit !== undefined) q.set("limit", String(params.limit));
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks?${q}`);
  return handleResponse<RiskListPage>(res);
}

export async function fetchRiskDetail(
  authFetch: AuthFetch,
  id: number,
): Promise<RiskDetail> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}`);
  return handleResponse<RiskDetail>(res);
}

export async function updateRisk(
  authFetch: AuthFetch,
  id: number,
  payload: UpdateRiskPayload,
): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
  return handleResponse<void>(res);
}

export async function approveRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/approve`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function rejectRisk(
  authFetch: AuthFetch,
  id: number,
  rejection_comment: string,
): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/reject`, {
    method: "POST",
    body: JSON.stringify({ rejection_comment }),
  });
  return handleResponse<void>(res);
}

export async function completeRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/complete`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function ownerApproveRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/owner-approve`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function closeRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/close`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function managementApproveRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/management-approve`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function cancelRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/cancel`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function resubmitRisk(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${id}/resubmit`, { method: "POST" });
  return handleResponse<void>(res);
}

export async function fetchDashboard(authFetch: AuthFetch): Promise<DashboardSummary> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/dashboard`);
  return handleResponse<DashboardSummary>(res);
}

export async function fetchAnalytics(
  authFetch: AuthFetch,
  registerId?: number,
): Promise<AnalyticsSummary> {
  const qs = registerId ? `?register_id=${registerId}` : "";
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/analytics/summary${qs}`);
  return handleResponse<AnalyticsSummary>(res);
}

export async function createAssessment(
  authFetch: AuthFetch,
  riskId: number,
  payload: CreateAssessmentPayload,
): Promise<RiskAssessmentRecord> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/${riskId}/assess`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return handleResponse<RiskAssessmentRecord>(res);
}
