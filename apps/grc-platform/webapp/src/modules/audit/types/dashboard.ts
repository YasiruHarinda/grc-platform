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

export interface AuditStats {
  totalAudits: number;
  activeAudits: number;
  completedAudits: number;
  archivedAudits: number;
}

export interface DashboardStats {
  totalControls: number;
  completedControls: number;
  overdueControls: number;
  evidenceRequiredControls: number;
  completionPercent: number;
  /** Absent from older backends; treat as 0 if missing. */
  totalActionItems?: number;
}

export interface StatusCount {
  status: string;
  count: number;
}

export interface TeamCompletion {
  team: string;
  completed: number;
  total: number;
  /** Controls past due and not yet complete. May be absent from an older backend. */
  overdue?: number;
}

/** One team's control count for a single status — feeds the team drill-down. */
export interface TeamStatusCount {
  team: string;
  status: string;
  count: number;
}

export interface ActionItem {
  controlId: number;
  auditId: number;
  auditName: string;
  controlNumber: string;
  description: string;
  status: string;
  dueDate: string;
  team: string;
  processOwner: string;
  teamId?: number | null;
  ownerId?: number | null;
}

export interface OverdueControl {
  controlId: number;
  auditId: number;
  auditName: string;
  controlNumber: string;
  description: string;
  status: string;
  dueDate: string;
  team: string;
  processOwner: string;
  teamId?: number | null;
  ownerId?: number | null;
}

export interface DashboardData {
  auditStats: AuditStats;
  stats: DashboardStats;
  statusDistribution: StatusCount[];
  teamCompletion: TeamCompletion[];
  /** Optional: absent until the entity/backend ship the additive field. */
  teamStatusDistribution?: TeamStatusCount[];
  actionItems: ActionItem[];
  /** Action items due within 7 days — fetched independently so LIMIT on actionItems doesn't affect this. */
  dueSoonItems: ActionItem[];
  /** Count of controls awaiting evidence/population submission or resubmission (any role). */
  pendingCount: number;
  /** Count of controls with evidence/population submitted and awaiting auditor validation/sampling. */
  validationCount: number;
  overdueControls: OverdueControl[];
}
