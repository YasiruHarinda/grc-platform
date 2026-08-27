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

// ── Response types (mirror internal/admin's Go models) ─────────────────────────

export interface DirectoryPerson {
  uuid: string;
  email: string;
  displayName: string;
}

export interface Grant {
  id: number;
  roleId: number;
  roleName: string;
  module: "RISK" | "AUDIT" | "SHARED";
  scopeType: "GLOBAL" | "RISK_TEAM" | "AUDIT_TEAM";
  scopeId: number;
  scopeBasis?: string;
  scopeTeamType?: string;
  scopeName?: string;
}

export interface AdminUser {
  id: number;
  uuid: string;
  displayName: string;
  email: string;
  userType: "INTERNAL" | "EXTERNAL";
  status: "ACTIVE" | "INACTIVE" | "REMOVED";
  createdOn: string;
  grants: Grant[];
}

// Role, as offered by the grant editor's picker — every module now (RISK,
// AUDIT, SHARED). assignableUserType governs which roles are offered once a
// person's user type (INTERNAL/EXTERNAL) is known; the backend enforces it
// regardless of what this picker offers.
export interface Role {
  id: number;
  roleName: string;
  description?: string;
  module: "RISK" | "AUDIT" | "SHARED";
  scopeBasis?: "SOURCE_REGISTER" | "ASSIGNMENT_TEAM";
  assignableUserType: "INTERNAL" | "EXTERNAL";
  status: string;
  // True if the backend would reject any non-GLOBAL grant of this role.
  globalOnly: boolean;
}

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

// searchDirectory looks up WSO2-org people by name/email substring, for the
// "Add User" typeahead. query shorter than 2 characters is not sent — the
// backend rejects it anyway, and there's no reason to round-trip a request
// that always 400s.
export async function searchDirectory(authFetch: AuthFetch, query: string): Promise<DirectoryPerson[]> {
  if (query.trim().length < 2) {
    return [];
  }
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/directory/search?q=${encodeURIComponent(query)}`);
  return handleResponse<DirectoryPerson[]>(res);
}

// searchExternalDirectory is searchDirectory's counterpart for the "Add User"
// typeahead once External is selected — a live call against the external
// Asgardeo org (no cached snapshot backs it, so each keystroke round-trips).
export async function searchExternalDirectory(authFetch: AuthFetch, query: string): Promise<DirectoryPerson[]> {
  if (query.trim().length < 2) {
    return [];
  }
  const res = await authFetch(
    `${BACKEND_BASE_URL}/api/v1/admin/directory/search-external?q=${encodeURIComponent(query)}`,
  );
  return handleResponse<DirectoryPerson[]>(res);
}

// fetchUsers lists platform users with their grants embedded. query is an
// optional name/email substring filter; omitted or empty returns everyone.
export async function fetchAdminUsers(authFetch: AuthFetch, query?: string): Promise<AdminUser[]> {
  const q = query?.trim() ? `?q=${encodeURIComponent(query.trim())}` : "";
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/users${q}`);
  return handleResponse<AdminUser[]>(res);
}

// createAdminUser provisions a platform user by uuid alone — the uuid-identity
// migration is why no email or display name is sent. userType defaults
// server-side to INTERNAL when omitted.
export async function createAdminUser(
  authFetch: AuthFetch,
  uuid: string,
  userType?: "INTERNAL" | "EXTERNAL",
): Promise<{ id: number; uuid: string; displayName: string; email: string }> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/users`, {
    method: "POST",
    body: JSON.stringify({ uuid, userType }),
  });
  return handleResponse(res);
}

// updateUserStatus sets a platform user's status. The server rejects a
// caller trying to change their own status (self-lockout guard) with a 422.
export async function updateUserStatus(
  authFetch: AuthFetch,
  userId: number,
  status: "ACTIVE" | "INACTIVE" | "REMOVED",
): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/users/${userId}/status`, {
    method: "PATCH",
    body: JSON.stringify({ status }),
  });
  await handleResponse(res);
}

// fetchRoles returns the grant editor's role picker options — every module.
export async function fetchRoles(authFetch: AuthFetch): Promise<Role[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/roles`);
  return handleResponse<Role[]>(res);
}

export interface CreateGrantPayload {
  roleId: number;
  scopeType: "GLOBAL" | "RISK_TEAM" | "AUDIT_TEAM";
  scopeId: number;
}

// createGrant grants a (role, scope) pair to a user. Idempotent on the
// server: re-granting something already held reactivates it rather than
// failing.
export async function createGrant(authFetch: AuthFetch, userId: number, payload: CreateGrantPayload): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/users/${userId}/grants`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  await handleResponse(res);
}

// revokeGrant deactivates a specific grant row (never deletes it).
export async function revokeGrant(authFetch: AuthFetch, userId: number, grantId: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/admin/users/${userId}/grants/${grantId}`, {
    method: "DELETE",
  });
  await handleResponse(res);
}

export interface ScopeTeam {
  id: number;
  name: string;
}

// fetchScopeTeams populates the grant editor's scope picker for a
// SOURCE_REGISTER- or ASSIGNMENT_TEAM-scoped RISK role. Calls the same
// GET /api/v1/risks/teams the Risk module's own pickers use directly (rather than
// importing modules/risk/api/riskApi.ts's equivalent) — the two modules stay
// independent of each other's internals even though they happen to read the
// same backend route today (see useAdminPrivileges's doc comment for the same
// tradeoff made the same way).
export async function fetchScopeTeams(
  authFetch: AuthFetch,
  type: "SOURCE_REGISTER" | "ASSIGNMENT",
): Promise<ScopeTeam[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/teams?type=${type}`);
  return handleResponse<ScopeTeam[]>(res);
}

// fetchAuditScopeTeams is fetchScopeTeams' counterpart for an AUDIT role's
// scope picker — a different backend route (audit_team, not risk_team), same
// ScopeTeam shape the picker already renders either kind through.
export async function fetchAuditScopeTeams(authFetch: AuthFetch): Promise<ScopeTeam[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audit/teams`);
  return handleResponse<ScopeTeam[]>(res);
}

// ── Manage Risk Hub — Risk Teams ────────────────────────────────────────────────

export interface AdminTeam {
  id: number;
  name: string;
  code: string | null;
  description: string | null;
  team_type: "SOURCE_REGISTER" | "ASSIGNMENT" | "BOTH";
  status: "ACTIVE" | "INACTIVE" | "REMOVED";
}

export interface TeamPayload {
  name: string;
  code: string | null;
  description: string;
  // The form's own Team Type selector only ever offers "BOTH" | "ASSIGNMENT"
  // (see teamTypeOptions in RiskTeamsPage.tsx) — but editing a pre-existing
  // SOURCE_REGISTER team must still round-trip that value unchanged, since
  // the update endpoint doesn't support partial updates and would otherwise
  // silently upgrade it to a full register.
  team_type: "BOTH" | "ASSIGNMENT" | "SOURCE_REGISTER";
  status: "ACTIVE" | "INACTIVE";
}

// fetchAllTeams lists every team regardless of type (no ?type= filter) or
// status (?includeInactive=true), unlike fetchScopeTeams, which the grant
// picker uses for a single semantic type and implicitly ACTIVE only. The Risk
// Teams admin table needs to show every row — including ASSIGNMENT-only teams
// a scope picker would never offer, and inactive ones so they can be
// reactivated rather than disappearing the moment they're deactivated.
export async function fetchAllTeams(authFetch: AuthFetch): Promise<AdminTeam[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/teams?includeInactive=true`);
  return handleResponse<AdminTeam[]>(res);
}

export async function createTeam(authFetch: AuthFetch, payload: TeamPayload): Promise<AdminTeam> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/teams`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return handleResponse<AdminTeam>(res);
}

export async function updateTeam(authFetch: AuthFetch, id: number, payload: TeamPayload): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/teams/${id}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
  await handleResponse(res);
}

// ── Manage Risk Hub — Risk Categories ───────────────────────────────────────────

export interface AdminRiskCategory {
  id: number;
  name: string;
  description: string | null;
}

export interface RiskCategoryPayload {
  name: string;
  description: string;
}

export async function fetchRiskCategories(authFetch: AuthFetch): Promise<AdminRiskCategory[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/categories`);
  return handleResponse<AdminRiskCategory[]>(res);
}

export async function createRiskCategory(
  authFetch: AuthFetch,
  payload: RiskCategoryPayload,
): Promise<AdminRiskCategory> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/categories`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return handleResponse<AdminRiskCategory>(res);
}

export async function updateRiskCategory(
  authFetch: AuthFetch,
  id: number,
  payload: RiskCategoryPayload,
): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/categories/${id}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
  await handleResponse(res);
}

// Refused (409) server-side when the category is still tagged on any risk —
// see the entity's DeleteRiskCategory doc comment for why that can't just be
// a DB constraint error. The caller shows whatever message comes back.
export async function deleteRiskCategory(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/categories/${id}`, { method: "DELETE" });
  await handleResponse(res);
}

// ── Manage Risk Hub — Compliance References ─────────────────────────────────────

export interface AdminComplianceReference {
  id: number;
  name: string;
  description: string | null;
}

export interface ComplianceReferencePayload {
  name: string;
  description: string;
}

export async function fetchComplianceReferences(authFetch: AuthFetch): Promise<AdminComplianceReference[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/compliance-references`);
  return handleResponse<AdminComplianceReference[]>(res);
}

export async function createComplianceReference(
  authFetch: AuthFetch,
  payload: ComplianceReferencePayload,
): Promise<AdminComplianceReference> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/compliance-references`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return handleResponse<AdminComplianceReference>(res);
}

export async function updateComplianceReference(
  authFetch: AuthFetch,
  id: number,
  payload: ComplianceReferencePayload,
): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/compliance-references/${id}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
  await handleResponse(res);
}

// Refused (409) server-side when the reference is still tagged on any risk —
// same reasoning as deleteRiskCategory.
export async function deleteComplianceReference(authFetch: AuthFetch, id: number): Promise<void> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/compliance-references/${id}`, { method: "DELETE" });
  await handleResponse(res);
}

// ── Manage Risk Hub — Risk Scores (read-only) ───────────────────────────────────

export interface RiskScore {
  id: number;
  likelihood: number;
  impact: number;
  risk_rating: number;
  risk_level: "LOW" | "MEDIUM" | "HIGH";
  color_code: string;
}

// No add/edit UI at all, not even for color — a locked decision (the 3x3
// matrix is a fixed set of load-bearing constants). This console only reads it.
export async function fetchRiskScores(authFetch: AuthFetch): Promise<RiskScore[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/risks/scores`);
  return handleResponse<RiskScore[]>(res);
}

// ── Manage Audit Hub — Audit Teams ──────────────────────────────────────────────
// Deliberately not the same shape as Risk's AdminTeam — audit_team has no
// code or team_type, just a name and a status.

export interface AdminAuditTeam {
  id: number;
  name: string;
  status: "ACTIVE" | "INACTIVE";
}

export interface AuditTeamPayload {
  name: string;
  status: "ACTIVE" | "INACTIVE";
}

// fetchAllAuditTeams lists every team regardless of status (?includeInactive=true),
// unlike fetchAuditScopeTeams, which the grant picker uses and which implicitly
// means ACTIVE only. Same reasoning as fetchAllTeams vs fetchScopeTeams.
export async function fetchAllAuditTeams(authFetch: AuthFetch): Promise<AdminAuditTeam[]> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audit/teams?includeInactive=true`);
  return handleResponse<AdminAuditTeam[]>(res);
}

export async function createAuditTeam(authFetch: AuthFetch, payload: AuditTeamPayload): Promise<AdminAuditTeam> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audit/teams`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return handleResponse<AdminAuditTeam>(res);
}

export async function updateAuditTeam(
  authFetch: AuthFetch,
  id: number,
  payload: AuditTeamPayload,
): Promise<AdminAuditTeam> {
  const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audit/teams/${id}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
  return handleResponse<AdminAuditTeam>(res);
}
