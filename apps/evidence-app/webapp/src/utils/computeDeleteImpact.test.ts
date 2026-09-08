import { describe, expect, test } from "vitest";
import {
  computeDeleteImpact,
  type DeleteImpactControl,
  type DeleteImpactEvidence,
  type DeleteImpactFramework,
  type DeleteImpactSubmission,
  type DeleteImpactTask,
} from "./computeDeleteImpact";
import { timeAgo } from "./timeAgo";

// A handful of frameworks, controls, evidence and submissions shared across
// tests that need a small catalogue rather than a single row. Kept as plain
// objects, built fresh per test where a test needs to control exactly what's
// in scope.

describe("computeDeleteImpact", () => {
  test("product level counts roll up across more than one framework and more than one control", () => {
    const frameworks: DeleteImpactFramework[] = [
      { id: 1, product_id: 100 },
      { id: 2, product_id: 100 },
    ];
    const controls: DeleteImpactControl[] = [
      { id: 10, framework_id: 1 },
      { id: 11, framework_id: 1 },
      { id: 12, framework_id: 2 },
    ];
    const evidence: DeleteImpactEvidence[] = [
      { id: 1000, control_id: 10 },
      { id: 1001, control_id: 11 },
      { id: 1002, control_id: 12 },
    ];
    const submissions: DeleteImpactSubmission[] = [
      { id: 5000, evidence_id: 1000, status: "submitted" },
      { id: 5001, evidence_id: 1001, status: "approved" },
      { id: 5002, evidence_id: 1002, status: "submitted" },
    ];

    const { impact, warnings } = computeDeleteImpact({
      level: "product",
      targetId: 100,
      frameworks,
      controls,
      evidence,
      submissions,
      tasks: [],
    });

    expect(impact).toEqual([
      { label: "frameworks", count: 2 },
      { label: "controls", count: 3 },
      { label: "evidence records", count: 3 },
      { label: "submission records", count: 3 },
      { label: "approved submissions", count: 1 },
    ]);
    expect(warnings).toEqual([]);
  });

  test("framework level counts cover only that framework's controls, not a sibling framework's rows", () => {
    const controls: DeleteImpactControl[] = [
      { id: 10, framework_id: 1 },
      { id: 11, framework_id: 1 },
      { id: 12, framework_id: 2 }, // sibling framework, must not be counted
    ];
    const evidence: DeleteImpactEvidence[] = [
      { id: 1000, control_id: 10 },
      { id: 1001, control_id: 11 },
      { id: 1002, control_id: 12 }, // belongs to the sibling, must not be counted
    ];
    const submissions: DeleteImpactSubmission[] = [
      { id: 5000, evidence_id: 1000, status: "submitted" },
      { id: 5001, evidence_id: 1001, status: "submitted" },
      { id: 5002, evidence_id: 1002, status: "approved" },
    ];

    const { impact, warnings } = computeDeleteImpact({
      level: "framework",
      targetId: 1,
      frameworks: [],
      controls,
      evidence,
      submissions,
      tasks: [],
    });

    expect(impact).toEqual([
      { label: "controls", count: 2 },
      { label: "evidence records", count: 2 },
      { label: "submission records", count: 2 },
      { label: "approved submissions", count: 0 },
    ]);
    expect(warnings).toEqual([]);
  });

  test("control level counts cover only that control", () => {
    const evidence: DeleteImpactEvidence[] = [
      { id: 1000, control_id: 10 },
      { id: 1001, control_id: 11 }, // a different control, must not be counted
    ];
    const submissions: DeleteImpactSubmission[] = [
      { id: 5000, evidence_id: 1000, status: "submitted" },
      { id: 5001, evidence_id: 1001, status: "submitted" },
    ];

    const { impact, warnings } = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks: [],
      controls: [],
      evidence,
      submissions,
      tasks: [],
    });

    expect(impact).toEqual([
      { label: "evidence records", count: 1 },
      { label: "submission records", count: 1 },
      { label: "approved submissions", count: 0 },
    ]);
    expect(warnings).toEqual([]);
  });

  test("approved submissions are counted separately and are also inside the total submission count", () => {
    const evidence: DeleteImpactEvidence[] = [{ id: 1000, control_id: 10 }];
    const submissions: DeleteImpactSubmission[] = [
      { id: 5000, evidence_id: 1000, status: "approved" },
      { id: 5001, evidence_id: 1000, status: "submitted" },
    ];

    const { impact } = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks: [],
      controls: [],
      evidence,
      submissions,
      tasks: [],
    });

    const submissionRecords = impact.find((i) => i.label === "submission records");
    const approved = impact.find((i) => i.label === "approved submissions");
    expect(submissionRecords?.count).toBe(2);
    expect(approved?.count).toBe(1);
  });

  test("only a task with status running warns; queued, completed and failed tasks produce no warning", () => {
    const tasks: DeleteImpactTask[] = [
      { status: "queued", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
      { status: "completed", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
      { status: "failed", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
      { status: "running", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "b@example.com" },
    ];

    const { warnings } = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks: [],
      controls: [],
      evidence: [],
      submissions: [],
      tasks,
    });

    // Header line + one line for the single running run + the closing line.
    expect(warnings).toHaveLength(3);
    expect(warnings[0]).toContain("1 agent run marked as in progress");
    expect(warnings[1]).toBe(`Started ${timeAgo("2026-09-08T00:00:00Z")} by b@example.com.`);
  });

  test("a running task pointing at a control outside the deletion path produces no warning, at every level", () => {
    const frameworks: DeleteImpactFramework[] = [
      { id: 1, product_id: 100 },
      { id: 2, product_id: 200 }, // a different product entirely
    ];
    const controls: DeleteImpactControl[] = [
      { id: 10, framework_id: 1 },
      { id: 20, framework_id: 2 }, // outside product 100 and outside framework 1
    ];
    const outsideTask: DeleteImpactTask[] = [
      { status: "running", control_id: 20, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
    ];

    const productResult = computeDeleteImpact({
      level: "product",
      targetId: 100,
      frameworks,
      controls,
      evidence: [],
      submissions: [],
      tasks: outsideTask,
    });
    const frameworkResult = computeDeleteImpact({
      level: "framework",
      targetId: 1,
      frameworks,
      controls,
      evidence: [],
      submissions: [],
      tasks: outsideTask,
    });
    const controlResult = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks,
      controls,
      evidence: [],
      submissions: [],
      tasks: outsideTask,
    });

    expect(productResult.warnings).toEqual([]);
    expect(frameworkResult.warnings).toEqual([]);
    expect(controlResult.warnings).toEqual([]);
  });

  test("a running task with a null control_id produces no warning and does not throw", () => {
    const tasks: DeleteImpactTask[] = [
      { status: "running", control_id: null, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
    ];

    expect(() =>
      computeDeleteImpact({
        level: "control",
        targetId: 10,
        frameworks: [],
        controls: [],
        evidence: [],
        submissions: [],
        tasks,
      })
    ).not.toThrow();

    const { warnings } = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks: [],
      controls: [],
      evidence: [],
      submissions: [],
      tasks,
    });
    expect(warnings).toEqual([]);
  });

  test("empty lists produce an impact list of zeroes and no warnings", () => {
    const { impact, warnings } = computeDeleteImpact({
      level: "product",
      targetId: 1,
      frameworks: [],
      controls: [],
      evidence: [],
      submissions: [],
      tasks: [],
    });

    expect(impact).toEqual([
      { label: "frameworks", count: 0 },
      { label: "controls", count: 0 },
      { label: "evidence records", count: 0 },
      { label: "submission records", count: 0 },
      { label: "approved submissions", count: 0 },
    ]);
    expect(warnings).toEqual([]);
  });

  test("warning wording is singular for one run and plural for two", () => {
    const oneRun: DeleteImpactTask[] = [
      { status: "running", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
    ];
    const twoRuns: DeleteImpactTask[] = [
      { status: "running", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "a@example.com" },
      { status: "running", control_id: 10, started_at: "2026-09-08T00:00:00Z", user_email: "b@example.com" },
    ];

    const singular = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks: [],
      controls: [],
      evidence: [],
      submissions: [],
      tasks: oneRun,
    });
    const plural = computeDeleteImpact({
      level: "control",
      targetId: 10,
      frameworks: [],
      controls: [],
      evidence: [],
      submissions: [],
      tasks: twoRuns,
    });

    expect(singular.warnings[0]).toBe("1 agent run marked as in progress against this control.");
    expect(plural.warnings[0]).toBe("2 agent runs marked as in progress against this control.");
  });
});
