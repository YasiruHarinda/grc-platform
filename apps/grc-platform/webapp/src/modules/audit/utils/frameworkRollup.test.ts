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

import { describe, expect, it } from "vitest";
import type { Audit, AuditControl } from "@modules/audit/types/audit";
import { computeElapsedPercent, computeFrameworkRollups } from "./frameworkRollup";

const TODAY = new Date("2026-06-15T00:00:00");

function makeAudit(overrides: Partial<Audit> = {}): Audit {
  return {
    id: 1,
    name: "Asgardeo 2026",
    framework: { id: 10, name: "SOC 2" },
    product: { id: 1, name: "Asgardeo" },
    periodStart: "2026-01-01",
    periodEnd: "2026-12-31",
    status: "ACTIVE",
    scopeDescription: null,
    controlCounts: { total: 20, approved: 10, overdue: 0 },
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeControl(overrides: Partial<AuditControl> = {}): AuditControl {
  return {
    id: 1,
    auditId: 1,
    ownerId: 1,
    ownerName: "Jane Doe",
    teamId: 1,
    teamName: "SRE",
    auditorId: null,
    auditorName: null,
    auditorEmail: null,
    controlNumber: "CC1.1",
    description: "desc",
    evidenceRequirement: null,
    requirementType: "OE",
    controlType: "CONFIG",
    scope: "COMMON",
    dueDate: "2026-07-01",
    status: "EVIDENCE_PENDING",
    controlSource: "MANUAL",
    sampleReference: null,
    comments: null,
    isOverdue: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

// ── computeElapsedPercent ─────────────────────────────────────────────────────

describe("computeElapsedPercent", () => {
  it("returns 0 before the period starts", () => {
    expect(computeElapsedPercent("2026-07-01", "2026-12-31", TODAY)).toBe(0);
  });

  it("returns 100 after the period ends", () => {
    expect(computeElapsedPercent("2026-01-01", "2026-06-01", TODAY)).toBe(100);
  });

  it("returns the linear position inside the period", () => {
    // 2026 is not a leap-affecting range here: Jan 1 -> Dec 31 is 364 days;
    // Jun 15 is 165 days in, so ~45%.
    const pct = computeElapsedPercent("2026-01-01", "2026-12-31", TODAY);
    expect(pct).toBeGreaterThan(40);
    expect(pct).toBeLessThan(50);
  });
});

// ── computeFrameworkRollups: aggregation ─────────────────────────────────────

describe("computeFrameworkRollups", () => {
  it("returns one rollup per framework, only for frameworks with an active audit", () => {
    const audits = [
      makeAudit({ id: 1, framework: { id: 10, name: "SOC 2" } }),
      makeAudit({ id: 2, framework: { id: 20, name: "ISO 27001" }, status: "COMPLETED" }),
    ];
    const rollups = computeFrameworkRollups(audits, {}, TODAY);
    expect(rollups.map((r) => r.name)).toEqual(["SOC 2"]);
  });

  it("sums control counts and overdue across a framework's audits", () => {
    const audits = [
      makeAudit({ id: 1, controlCounts: { total: 20, approved: 10, overdue: 2 } }),
      makeAudit({ id: 2, controlCounts: { total: 30, approved: 20, overdue: 1 } }),
    ];
    const [rollup] = computeFrameworkRollups(audits, {}, TODAY);
    expect(rollup.total).toBe(50);
    expect(rollup.complete).toBe(30);
    expect(rollup.overdueCount).toBe(3);
    expect(rollup.completionPercent).toBeCloseTo(60);
    expect(rollup.auditCount).toBe(2);
  });

  it("weights elapsed percent by control count when computing pace", () => {
    // Audit A: 52 controls, fully elapsed (period already over) -> elapsed 100%
    // Audit B: 24 controls, period hasn't started -> elapsed 0%
    // Weighted elapsed = (100*52 + 0*24) / 76 ≈ 68.42
    const audits = [
      makeAudit({
        id: 1,
        controlCounts: { total: 52, approved: 52, overdue: 0 },
        periodStart: "2025-01-01",
        periodEnd: "2025-12-31",
      }),
      makeAudit({
        id: 2,
        controlCounts: { total: 24, approved: 0, overdue: 0 },
        periodStart: "2027-01-01",
        periodEnd: "2027-12-31",
      }),
    ];
    const [rollup] = computeFrameworkRollups(audits, {}, TODAY);
    // completionPercent = 52/76*100 = 68.42
    const expectedElapsed = (100 * 52 + 0 * 24) / 76;
    const expectedCompletion = (52 / 76) * 100;
    expect(rollup.pace).toBeCloseTo(expectedCompletion - expectedElapsed, 1);
  });

  it("uses the nearest audit end date as the framework deadline", () => {
    const audits = [
      makeAudit({ id: 1, periodEnd: "2026-12-31" }),
      makeAudit({ id: 2, periodEnd: "2026-08-01" }),
    ];
    const [rollup] = computeFrameworkRollups(audits, {}, TODAY);
    expect(rollup.deadline).toBe("2026-08-01");
  });

  it("sorts frameworks by overdue desc, then pace asc", () => {
    const audits = [
      makeAudit({
        id: 1,
        framework: { id: 1, name: "No Overdue" },
        controlCounts: { total: 20, approved: 20, overdue: 0 },
        periodStart: "2026-01-01",
        periodEnd: "2026-12-31",
      }),
      makeAudit({
        id: 2,
        framework: { id: 2, name: "High Overdue" },
        controlCounts: { total: 20, approved: 5, overdue: 5 },
        periodStart: "2026-01-01",
        periodEnd: "2026-12-31",
      }),
      makeAudit({
        id: 3,
        framework: { id: 3, name: "Mid Overdue" },
        controlCounts: { total: 20, approved: 5, overdue: 3 },
        periodStart: "2026-01-01",
        periodEnd: "2026-12-31",
      }),
      makeAudit({
        id: 4,
        framework: { id: 4, name: "Low Overdue" },
        controlCounts: { total: 20, approved: 5, overdue: 1 },
        periodStart: "2026-01-01",
        periodEnd: "2026-12-31",
      }),
    ];
    const rollups = computeFrameworkRollups(audits, {}, TODAY);
    expect(rollups.map((r) => r.name)).toEqual([
      "High Overdue",
      "Mid Overdue",
      "Low Overdue",
      "No Overdue",
    ]);
  });

  it("marks rail-only rollups as not having detail, with a coarse phase split", () => {
    const audits = [makeAudit({ controlCounts: { total: 20, approved: 8, overdue: 0 } })];
    const [rollup] = computeFrameworkRollups(audits, {}, TODAY);
    expect(rollup.hasDetail).toBe(false);
    expect(rollup.phaseCounts.complete).toBe(8);
    expect(rollup.blockers).toEqual([]);
    expect(rollup.teams).toEqual([]);
  });

  it("keeps a framework's hasDetail false while any of its audits are still awaiting their control fetch", () => {
    const audits = [
      makeAudit({ id: 1, controlCounts: { total: 20, approved: 8, overdue: 0 } }),
      makeAudit({ id: 2, controlCounts: { total: 10, approved: 5, overdue: 0 } }),
    ];
    // Only audit 1's controls have arrived; audit 2's query is still pending.
    const controls = [makeControl({ id: 1, auditId: 1, status: "COMPLETE" })];
    const [rollup] = computeFrameworkRollups(audits, { 1: controls }, TODAY);
    expect(rollup.hasDetail).toBe(false);
    // The still-pending audit's blockers/teams must not be silently dropped
    // from the framework total by treating the framework as fully detailed.
    expect(rollup.audits.find((a) => a.id === 2)?.hasDetail).toBe(false);
  });
});

// ── computeFrameworkRollups: detail (controls provided) ──────────────────────

describe("computeFrameworkRollups with controls", () => {
  it("computes the exact four-phase breakdown from control statuses", () => {
    const audit = makeAudit({ id: 1, controlCounts: { total: 4, approved: 1, overdue: 0 } });
    const controls = [
      makeControl({ id: 1, auditId: 1, status: "COMPLETE" }),
      makeControl({ id: 2, auditId: 1, status: "AWAITING_SAMPLE" }),
      makeControl({ id: 3, auditId: 1, status: "POPULATION_NEED_CLARIFICATION" }),
      makeControl({ id: 4, auditId: 1, status: "POPULATION_PENDING" }),
    ];
    const [rollup] = computeFrameworkRollups([audit], { 1: controls }, TODAY);
    expect(rollup.hasDetail).toBe(true);
    expect(rollup.phaseCounts).toEqual({
      complete: 1,
      inProgress: 1,
      needsClarification: 1,
      notStarted: 1,
    });
  });

  it("ranks blockers overdue first, then needs-clarification, then due-soon", () => {
    const audit = makeAudit({ id: 1 });
    const controls = [
      makeControl({ id: 1, auditId: 1, controlNumber: "DUE-SOON", status: "EVIDENCE_PENDING", dueDate: "2026-06-18", isOverdue: false }),
      makeControl({ id: 2, auditId: 1, controlNumber: "OVERDUE", status: "EVIDENCE_PENDING", dueDate: "2026-06-01", isOverdue: true }),
      makeControl({ id: 3, auditId: 1, controlNumber: "BLOCKED", status: "EVIDENCE_NEED_CLARIFICATION", dueDate: "2026-09-01", isOverdue: false }),
      makeControl({ id: 4, auditId: 1, controlNumber: "NOT-A-BLOCKER", status: "COMPLETE", dueDate: "2026-01-01", isOverdue: false }),
    ];
    const [rollup] = computeFrameworkRollups([audit], { 1: controls }, TODAY);
    expect(rollup.blockers.map((b) => b.controlNumber)).toEqual(["OVERDUE", "BLOCKED", "DUE-SOON"]);
    expect(rollup.blockers.map((b) => b.reason)).toEqual(["overdue", "needsClarification", "dueSoon"]);
    // §5: due-soon is summed as its own framework figure alongside overdue.
    expect(rollup.dueSoonCount).toBe(1);
  });

  it("prioritizes overdue over needs-clarification for a control that is both", () => {
    const audit = makeAudit({ id: 1 });
    const controls = [
      makeControl({ id: 1, auditId: 1, status: "EVIDENCE_NEED_CLARIFICATION", isOverdue: true }),
    ];
    const [rollup] = computeFrameworkRollups([audit], { 1: controls }, TODAY);
    expect(rollup.blockers).toHaveLength(1);
    expect(rollup.blockers[0].reason).toBe("overdue");
  });

  it("counts a blocked-and-due-soon control toward dueSoonCount even though its blocker row displays as needsClarification", () => {
    const audit = makeAudit({ id: 1 });
    const controls = [
      makeControl({
        id: 1,
        auditId: 1,
        status: "EVIDENCE_NEED_CLARIFICATION",
        dueDate: "2026-06-18", // 3 days from TODAY — inside the due-soon window
        isOverdue: false,
        teamName: "SRE",
      }),
    ];
    const [rollup] = computeFrameworkRollups([audit], { 1: controls }, TODAY);
    // List display still prioritizes needsClarification for the single row.
    expect(rollup.blockers.map((b) => b.reason)).toEqual(["needsClarification"]);
    // But the aggregate due-soon totals must not drop the control just
    // because its blocker row was classified as needsClarification —
    // audit-level and team-level figures must agree with each other.
    expect(rollup.audits[0].dueSoonCount).toBe(1);
    expect(rollup.dueSoonCount).toBe(1);
    expect(rollup.teams[0].dueSoonCount).toBe(1);
  });

  it("builds per-team rows with the assignee holding the most overdue work", () => {
    const audit = makeAudit({ id: 1 });
    const controls = [
      makeControl({ id: 1, auditId: 1, teamName: "SRE", ownerName: "Alice", isOverdue: true }),
      makeControl({ id: 2, auditId: 1, teamName: "SRE", ownerName: "Alice", isOverdue: true }),
      makeControl({ id: 3, auditId: 1, teamName: "SRE", ownerName: "Bob", isOverdue: true }),
      makeControl({ id: 4, auditId: 1, teamName: "SRE", ownerName: "Carol", isOverdue: false, status: "COMPLETE" }),
    ];
    const [rollup] = computeFrameworkRollups([audit], { 1: controls }, TODAY);
    const sre = rollup.teams.find((t) => t.team === "SRE");
    expect(sre).toBeDefined();
    expect(sre?.total).toBe(4);
    expect(sre?.overdueCount).toBe(3);
    expect(sre?.topAssignee).toEqual({ name: "Alice", overdueCount: 2 });
    // Bob also holds overdue work -> +1 beyond the named assignee.
    expect(sre?.additionalAssigneeCount).toBe(1);
  });

  it("merges teams and assignees across a framework's audits", () => {
    const audits = [makeAudit({ id: 1 }), makeAudit({ id: 2 })];
    const controlsByAuditId = {
      1: [makeControl({ id: 1, auditId: 1, teamName: "SRE", ownerName: "Alice", isOverdue: true })],
      2: [makeControl({ id: 2, auditId: 2, teamName: "SRE", ownerName: "Alice", isOverdue: true })],
    };
    const [rollup] = computeFrameworkRollups(audits, controlsByAuditId, TODAY);
    const sre = rollup.teams.find((t) => t.team === "SRE");
    expect(sre?.total).toBe(2);
    expect(sre?.overdueCount).toBe(2);
    expect(sre?.topAssignee).toEqual({ name: "Alice", overdueCount: 2 });
  });

  it("tags each merged blocker with its originating audit", () => {
    const audits = [makeAudit({ id: 1, name: "Audit A" }), makeAudit({ id: 2, name: "Audit B" })];
    const controlsByAuditId = {
      1: [makeControl({ id: 1, auditId: 1, controlNumber: "A-1", isOverdue: true })],
      2: [makeControl({ id: 2, auditId: 2, controlNumber: "B-1", isOverdue: true })],
    };
    const [rollup] = computeFrameworkRollups(audits, controlsByAuditId, TODAY);
    const labels = rollup.blockers.map((b) => `${b.auditName}:${b.controlNumber}`);
    expect(labels).toEqual(expect.arrayContaining(["Audit A:A-1", "Audit B:B-1"]));
  });
});
