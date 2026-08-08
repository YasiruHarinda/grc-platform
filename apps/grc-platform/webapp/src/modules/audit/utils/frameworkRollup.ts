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

import type { Audit, AuditControl } from "@modules/audit/types/audit";
import type {
  AuditRollup,
  BlockerReason,
  BlockerRow,
  FrameworkRollup,
  PhaseCounts,
  TeamRollup,
} from "@modules/audit/types/framework";
import { DUE_SOON_DAYS } from "@modules/audit/components/dashboard/dueDate";
import { STATUS_PHASE } from "@modules/audit/utils/controlStatus";

const DAY_MS = 86_400_000;

function daysBetween(from: Date, to: Date): number {
  return Math.round((to.getTime() - from.getTime()) / DAY_MS);
}

function startOfDay(date: Date): Date {
  const d = new Date(date);
  d.setHours(0, 0, 0, 0);
  return d;
}

// ── §4.2 Pace ──────────────────────────────────────────────────────────────

export function computeElapsedPercent(periodStart: string, periodEnd: string, today: Date = new Date()): number {
  const start = new Date(`${periodStart}T00:00:00`).getTime();
  const end = new Date(`${periodEnd}T00:00:00`).getTime();
  const now = startOfDay(today).getTime();
  if (now <= start) return 0;
  if (now >= end) return 100;
  return ((now - start) / (end - start)) * 100;
}

// ── Phase / blocker / team derivation from a control list ────────────────────

const EMPTY_PHASE_COUNTS: PhaseCounts = { complete: 0, inProgress: 0, needsClarification: 0, notStarted: 0 };

function tallyPhaseCounts(controls: AuditControl[]): PhaseCounts {
  const counts = { ...EMPTY_PHASE_COUNTS };
  for (const control of controls) {
    switch (STATUS_PHASE[control.status]) {
      case "COMPLETE": counts.complete += 1; break;
      case "IN_PROGRESS": counts.inProgress += 1; break;
      case "BLOCKED": counts.needsClarification += 1; break;
      case "NOT_STARTED": counts.notStarted += 1; break;
    }
  }
  return counts;
}

function mergePhaseCounts(a: PhaseCounts, b: PhaseCounts): PhaseCounts {
  return {
    complete: a.complete + b.complete,
    inProgress: a.inProgress + b.inProgress,
    needsClarification: a.needsClarification + b.needsClarification,
    notStarted: a.notStarted + b.notStarted,
  };
}

function blockerReason(control: AuditControl, today: Date): BlockerReason | null {
  if (control.isOverdue) return "overdue";
  if (STATUS_PHASE[control.status] === "BLOCKED") return "needsClarification";
  if (control.status === "COMPLETE") return null;
  if (!control.dueDate) return null;
  const due = new Date(`${control.dueDate}T00:00:00`);
  const days = daysBetween(startOfDay(today), due);
  if (days >= 0 && days <= DUE_SOON_DAYS) return "dueSoon";
  return null;
}

const REASON_RANK: Record<BlockerReason, number> = { overdue: 0, needsClarification: 1, dueSoon: 2 };

function dueDateSortKey(dueDate: string | null): number {
  return dueDate ? new Date(`${dueDate}T00:00:00`).getTime() : Number.POSITIVE_INFINITY;
}

function sortBlockers(blockers: BlockerRow[]): BlockerRow[] {
  return [...blockers].sort((a, b) => {
    if (REASON_RANK[a.reason] !== REASON_RANK[b.reason]) return REASON_RANK[a.reason] - REASON_RANK[b.reason];
    return dueDateSortKey(a.dueDate) - dueDateSortKey(b.dueDate);
  });
}

function buildBlockers(audit: Audit, controls: AuditControl[], today: Date): BlockerRow[] {
  const rows: BlockerRow[] = [];
  for (const control of controls) {
    const reason = blockerReason(control, today);
    if (!reason) continue;
    rows.push({
      controlId: control.id,
      controlNumber: control.controlNumber,
      team: control.teamName ?? "Unassigned",
      owner: control.ownerName ?? "Unassigned",
      reason,
      dueDate: control.dueDate,
      auditId: audit.id,
      auditName: audit.name,
    });
  }
  return sortBlockers(rows);
}

interface TeamDraft {
  team: string;
  completed: number;
  total: number;
  phaseCounts: PhaseCounts;
  openCount: number;
  overdueCount: number;
  dueSoonCount: number;
  assignees: Map<string, number>;
}

// Builds team rows straight from a flat control list. The framework-level
// rollup calls this with the *concatenation* of every audit's controls
// rather than merging already-collapsed per-audit TeamRollups, since
// collapsing to a single topAssignee per audit first would throw away the
// per-assignee counts needed to find the true cross-audit top holder (§5).
function buildTeams(controls: AuditControl[], today: Date): TeamRollup[] {
  const byTeam = new Map<string, TeamDraft>();
  for (const control of controls) {
    const teamName = control.teamName ?? "Unassigned";
    let draft = byTeam.get(teamName);
    if (!draft) {
      draft = {
        team: teamName,
        completed: 0,
        total: 0,
        phaseCounts: { ...EMPTY_PHASE_COUNTS },
        openCount: 0,
        overdueCount: 0,
        dueSoonCount: 0,
        assignees: new Map(),
      };
      byTeam.set(teamName, draft);
    }

    draft.total += 1;
    const phase = STATUS_PHASE[control.status];
    if (phase === "COMPLETE") draft.completed += 1;
    else draft.openCount += 1;
    draft.phaseCounts = mergePhaseCounts(draft.phaseCounts, tallyPhaseCounts([control]));

    if (control.isOverdue) {
      draft.overdueCount += 1;
      const owner = control.ownerName ?? "Unassigned";
      draft.assignees.set(owner, (draft.assignees.get(owner) ?? 0) + 1);
    } else if (phase !== "COMPLETE" && control.dueDate) {
      const days = daysBetween(startOfDay(today), new Date(`${control.dueDate}T00:00:00`));
      if (days >= 0 && days <= DUE_SOON_DAYS) draft.dueSoonCount += 1;
    }
  }

  return [...byTeam.values()]
    .map((draft): TeamRollup => {
      const ranked = [...draft.assignees.entries()]
        .filter(([, overdueCount]) => overdueCount > 0)
        .sort((a, b) => (b[1] - a[1]) || a[0].localeCompare(b[0]));
      const topAssignee = ranked.length > 0 ? { name: ranked[0][0], overdueCount: ranked[0][1] } : null;
      return {
        team: draft.team,
        completed: draft.completed,
        total: draft.total,
        phaseCounts: draft.phaseCounts,
        openCount: draft.openCount,
        overdueCount: draft.overdueCount,
        dueSoonCount: draft.dueSoonCount,
        topAssignee,
        additionalAssigneeCount: Math.max(ranked.length - 1, 0),
      };
    })
    .sort((a, b) => (b.overdueCount - a.overdueCount) || (b.openCount - a.openCount));
}

// ── Per-audit rollup ───────────────────────────────────────────────────────

export function computeAuditRollup(audit: Audit, controls: AuditControl[] | undefined, today: Date = new Date()): AuditRollup {
  const { total, approved, overdue } = audit.controlCounts;
  const completionPercent = total > 0 ? (approved / total) * 100 : 0;
  const hasDetail = controls !== undefined;
  const blockers = hasDetail ? buildBlockers(audit, controls, today) : [];

  return {
    id: audit.id,
    name: audit.name,
    total,
    complete: approved,
    completionPercent,
    overdueCount: overdue,
    dueSoonCount: blockers.filter((b) => b.reason === "dueSoon").length,
    deadline: audit.periodEnd,
    phaseCounts: hasDetail
      ? tallyPhaseCounts(controls)
      : { complete: approved, inProgress: 0, needsClarification: 0, notStarted: total - approved },
    blockers,
    teams: hasDetail ? buildTeams(controls, today) : [],
    hasDetail,
  };
}

// ── Framework rollup: sums audits, never stores figures separately (§5) ─────

export function computeFrameworkRollups(
  audits: Audit[],
  controlsByAuditId: Record<number, AuditControl[]> = {},
  today: Date = new Date(),
): FrameworkRollup[] {
  const byFramework = new Map<number, { name: string; audits: Audit[] }>();
  for (const audit of audits) {
    if (audit.status !== "ACTIVE") continue;
    let entry = byFramework.get(audit.framework.id);
    if (!entry) {
      entry = { name: audit.framework.name, audits: [] };
      byFramework.set(audit.framework.id, entry);
    }
    entry.audits.push(audit);
  }

  const rollups: FrameworkRollup[] = [];
  for (const [frameworkId, { name, audits: frameworkAudits }] of byFramework) {
    const auditRollups = frameworkAudits
      .map((audit) => computeAuditRollup(audit, controlsByAuditId[audit.id], today))
      .sort((a, b) => a.deadline.localeCompare(b.deadline));

    const total = auditRollups.reduce((s, a) => s + a.total, 0);
    const complete = auditRollups.reduce((s, a) => s + a.complete, 0);
    const overdueCount = auditRollups.reduce((s, a) => s + a.overdueCount, 0);
    const dueSoonCount = auditRollups.reduce((s, a) => s + a.dueSoonCount, 0);
    const completionPercent = total > 0 ? (complete / total) * 100 : 0;

    // §5: elapsed% is weighted by control count so a 52-control audit moves
    // the framework's pace more than a 24-control one.
    const weightedElapsed = frameworkAudits.reduce(
      (s, audit) => s + computeElapsedPercent(audit.periodStart, audit.periodEnd, today) * audit.controlCounts.total,
      0,
    );
    const elapsedPercent = total > 0 ? weightedElapsed / total : 0;
    const pace = completionPercent - elapsedPercent;

    const nearest = auditRollups[0]; // sorted by deadline ascending above
    const deadline = nearest ? nearest.deadline : "";

    const hasDetail = auditRollups.some((a) => a.hasDetail);
    const phaseCounts = auditRollups.reduce((acc, a) => mergePhaseCounts(acc, a.phaseCounts), { ...EMPTY_PHASE_COUNTS });
    const blockers = sortBlockers(auditRollups.flatMap((a) => a.blockers));
    // Built from the raw concatenated controls (not by merging per-audit
    // TeamRollups) so the cross-audit top assignee is correct — see buildTeams.
    const controlsInScope = frameworkAudits.flatMap((audit) => controlsByAuditId[audit.id] ?? []);
    const teams = buildTeams(controlsInScope, today);

    rollups.push({
      id: frameworkId,
      name,
      auditCount: auditRollups.length,
      pace,
      total,
      complete,
      completionPercent,
      overdueCount,
      dueSoonCount,
      deadline,
      phaseCounts,
      blockers,
      teams,
      hasDetail,
      audits: auditRollups,
    });
  }

  // §4.3 Rail sort order: overdue desc, then pace asc.
  return rollups.sort((a, b) => {
    if (b.overdueCount !== a.overdueCount) return b.overdueCount - a.overdueCount;
    return a.pace - b.pace;
  });
}
