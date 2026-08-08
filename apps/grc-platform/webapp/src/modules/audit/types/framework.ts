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

export interface PhaseCounts {
  complete: number;
  inProgress: number;
  needsClarification: number;
  notStarted: number;
}

export type BlockerReason = "overdue" | "needsClarification" | "dueSoon";

export interface BlockerRow {
  controlId: number;
  controlNumber: string;
  team: string;
  owner: string;
  reason: BlockerReason;
  dueDate: string | null;
  auditId: number;
  auditName: string;
}

export interface TeamAssigneeOverdue {
  name: string;
  overdueCount: number;
}

export interface TeamRollup {
  team: string;
  completed: number;
  total: number;
  phaseCounts: PhaseCounts;
  openCount: number;
  overdueCount: number;
  dueSoonCount: number;
  topAssignee: TeamAssigneeOverdue | null;
  /** Other assignees beyond topAssignee who also hold overdue work. */
  additionalAssigneeCount: number;
}

/**
 * Shared shape rendered by the three detail panels. A FrameworkRollup and each
 * entry in its `audits` both satisfy this, so "All audits" and a single
 * selected audit chip feed the same panel components (§3.2/3.3).
 */
export interface ScopeRollup {
  total: number;
  complete: number;
  completionPercent: number;
  overdueCount: number;
  /** Due within DUE_SOON_DAYS and not already overdue (§5 aggregation table). */
  dueSoonCount: number;
  /** YYYY-MM-DD of the binding deadline (nearest audit end date in scope). */
  deadline: string;
  phaseCounts: PhaseCounts;
  blockers: BlockerRow[];
  teams: TeamRollup[];
  /**
   * False until this scope's controls have been fetched — phaseCounts/blockers/
   * teams are then a coarse complete-vs-rest placeholder rather than the real
   * breakdown. See frameworkRollup.ts for why the rail can't always afford the
   * per-audit controls fetch up front.
   */
  hasDetail: boolean;
}

export interface AuditRollup extends ScopeRollup {
  id: number;
  name: string;
}

export interface FrameworkRollup extends ScopeRollup {
  id: number;
  name: string;
  auditCount: number;
  /** Drives rail sort order only (§4.3) — never rendered. */
  pace: number;
  audits: AuditRollup[];
}
