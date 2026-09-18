import { describe, expect, test } from "vitest";
import { computeAgentRunnerFormState } from "./computeAgentRunnerFormState";

describe("computeAgentRunnerFormState", () => {
  test("signed out — form stays locked even with a prompt typed", () => {
    const state = computeAgentRunnerFormState({
      loginDone: false,
      taskStatus: null,
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(true);
    expect(state.advancedSettingsEditable).toBe(true);
    expect(state.primaryAction).toBe("queue");
    expect(state.primaryActionEnabled).toBe(false);
    expect(state.resultPanelAction).toBe("startFresh");
  });

  test("no task, prompt empty — primary action is refused for lack of a prompt", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: null,
      queueing: false,
      promptEmpty: true,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(true);
    expect(state.advancedSettingsEditable).toBe(true);
    expect(state.primaryAction).toBe("queue");
    expect(state.primaryActionEnabled).toBe(false);
  });

  test("no task, prompt typed — the form is ready to queue", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: null,
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.advancedSettingsEditable).toBe(true);
    expect(state.primaryAction).toBe("queue");
    expect(state.primaryActionEnabled).toBe(true);
    expect(state.resultPanelAction).toBe("startFresh");
  });

  test("a queue request in flight — refused until it resolves", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: null,
      queueing: true,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.primaryAction).toBe("queuing");
    expect(state.primaryActionEnabled).toBe(false);
    expect(state.advancedSettingsEditable).toBe(true);
  });

  test("task queued — waiting for the runner to pick it up", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "queued",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(true);
    expect(state.advancedSettingsEditable).toBe(false);
    expect(state.primaryAction).toBe("waitingForRunner");
    expect(state.primaryActionEnabled).toBe(false);
    expect(state.resultPanelAction).toBe("startFresh");
  });

  test("task running — the prompt stays editable so the next one can be drafted", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "running",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(true);
    expect(state.advancedSettingsEditable).toBe(false);
    expect(state.primaryAction).toBe("runningAgent");
    expect(state.primaryActionEnabled).toBe(false);
    expect(state.resultPanelAction).toBe("startFresh");
  });

  test("task completed — the primary action flips to new task, enabled, prompt locks, and the result panel offers a new task", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "completed",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(false);
    expect(state.advancedSettingsEditable).toBe(false);
    expect(state.primaryAction).toBe("newTask");
    expect(state.primaryActionEnabled).toBe(true);
    expect(state.resultPanelAction).toBe("newTask");
  });

  test("task failed — same finished shape as completed", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "failed",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(false);
    expect(state.advancedSettingsEditable).toBe(false);
    expect(state.primaryAction).toBe("newTask");
    expect(state.primaryActionEnabled).toBe(true);
    expect(state.resultPanelAction).toBe("newTask");
  });

  test("task cancelled — same finished shape as completed", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "cancelled",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(false);
    expect(state.advancedSettingsEditable).toBe(false);
    expect(state.primaryAction).toBe("newTask");
    expect(state.primaryActionEnabled).toBe(true);
    expect(state.resultPanelAction).toBe("newTask");
  });

  test("task completed with an empty prompt — new task is still enabled, since an empty prompt has nothing to do with clearing a finished task", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "completed",
      queueing: false,
      promptEmpty: true,
      unacknowledgedChangingSteps: false,
    });

    expect(state.promptEditable).toBe(false);
    expect(state.primaryAction).toBe("newTask");
    expect(state.primaryActionEnabled).toBe(true);
  });

  test("task completed but sign in unconfirmed — new task is offered but disabled, because the whole form is unclickable without it", () => {
    const state = computeAgentRunnerFormState({
      loginDone: false,
      taskStatus: "completed",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.primaryAction).toBe("newTask");
    expect(state.primaryActionEnabled).toBe(false);
  });

  test("the primary action flips to new task exactly when a task reaches a finished status", () => {
    const statuses: Array<{ status: "queued" | "running" | "completed" | "failed" | "cancelled" | null; expectNewTask: boolean }> = [
      { status: null, expectNewTask: false },
      { status: "queued", expectNewTask: false },
      { status: "running", expectNewTask: false },
      { status: "completed", expectNewTask: true },
      { status: "failed", expectNewTask: true },
      { status: "cancelled", expectNewTask: true },
    ];

    for (const { status, expectNewTask } of statuses) {
      const state = computeAgentRunnerFormState({
        loginDone: true,
        taskStatus: status,
        queueing: false,
        promptEmpty: false,
        unacknowledgedChangingSteps: false,
      });
      expect(state.primaryAction === "newTask").toBe(expectNewTask);
      expect(state.promptEditable).toBe(!expectNewTask);
    }
  });

  test("an unacknowledged changing step refuses the primary action even though everything else is ready", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: null,
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: true,
    });

    expect(state.primaryAction).toBe("queue");
    expect(state.primaryActionEnabled).toBe(false);
  });

  test("an acknowledged changing step does not refuse the primary action", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: null,
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.primaryAction).toBe("queue");
    expect(state.primaryActionEnabled).toBe(true);
  });

  test("a finished task ignores an unacknowledged changing step entirely — new task stays enabled", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "completed",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: true,
    });

    expect(state.primaryAction).toBe("newTask");
    expect(state.primaryActionEnabled).toBe(true);
  });

  test("signed out with nothing typed — the Control panel is unlocked but the run panel is not", () => {
    const state = computeAgentRunnerFormState({
      loginDone: false,
      taskStatus: null,
      queueing: false,
      promptEmpty: true,
      unacknowledgedChangingSteps: false,
    });

    expect(state.controlLinkEditable).toBe(true);
    expect(state.runSectionEditable).toBe(false);
  });

  test("signed out with a prompt typed — the Control panel stays unlocked but the Queue button is still refused", () => {
    const state = computeAgentRunnerFormState({
      loginDone: false,
      taskStatus: null,
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.controlLinkEditable).toBe(true);
    expect(state.runSectionEditable).toBe(false);
    expect(state.primaryActionEnabled).toBe(false);
  });

  test("logged in with no Agent Task — both panels are unlocked", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: null,
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.controlLinkEditable).toBe(true);
    expect(state.runSectionEditable).toBe(true);
  });

  test("logged in with an Agent Task queued or running — the run in progress does not re-lock either panel", () => {
    for (const status of ["queued", "running"] as const) {
      const state = computeAgentRunnerFormState({
        loginDone: true,
        taskStatus: status,
        queueing: false,
        promptEmpty: false,
        unacknowledgedChangingSteps: false,
      });

      expect(state.controlLinkEditable).toBe(true);
      expect(state.runSectionEditable).toBe(true);
    }
  });

  test("logged in with an Agent Task finished — both panels stay unlocked even though the prompt itself has locked", () => {
    const state = computeAgentRunnerFormState({
      loginDone: true,
      taskStatus: "completed",
      queueing: false,
      promptEmpty: false,
      unacknowledgedChangingSteps: false,
    });

    expect(state.controlLinkEditable).toBe(true);
    expect(state.runSectionEditable).toBe(true);
  });
});
