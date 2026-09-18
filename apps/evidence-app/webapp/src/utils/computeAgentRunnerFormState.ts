// The Agent Task statuses the backend ever sets on a queued run. Kept
// structural rather than imported from the page, so this module carries no
// dependency on AgentRunner.tsx or its wider TaskOut shape.
export type AgentRunnerTaskStatus = "queued" | "running" | "completed" | "failed" | "cancelled";

// What the primary button on the run form is offering right now. "queue" is
// the resting state — everything else is either something already in flight
// (which is also, today, exactly when the button shows a spinner instead of
// the arrow icon) or, for "newTask", a finished task waiting to be cleared —
// see ticket chala2001/grc-tools#139.
export type AgentRunnerPrimaryAction = "queue" | "queuing" | "waitingForRunner" | "runningAgent" | "newTask";

export type ComputeAgentRunnerFormStateArgs = {
  // Whether the Engineer has clicked "I've logged in" for Step 2.
  loginDone: boolean;
  // The current Agent Task's status, or null when there is no task yet
  // (before the first queue, or after New Task has cleared one).
  taskStatus: AgentRunnerTaskStatus | null;
  // Whether the createTask request is in flight.
  queueing: boolean;
  // Whether the prompt field is blank (after trimming).
  promptEmpty: boolean;
  // Whether the prompt has a step that looks like it changes something
  // (detectChangingSteps found one) that the Engineer has not yet
  // acknowledged for the CURRENT prompt text. Ignored once a task is
  // finished — a locked prompt has nothing left for the warning to govern.
  // See chala2001/grc-tools#140.
  unacknowledgedChangingSteps: boolean;
};

export type AgentRunnerFormState = {
  // Whether the Control link picker (Product, Framework, Control, Evidence
  // Title) accepts input. Always true — the picker is about which Control
  // the Engineer is here for, and that has nothing to do with whether a
  // browser session exists yet. Named as a constant rather than left out of
  // this type, so "the Control panel is never locked" is a decision this
  // function states and a test can assert, not a fact someone has to notice
  // by its absence. See chala2001/grc-tools#150.
  controlLinkEditable: boolean;
  // Whether the run panel (prompt, complexity, Advanced settings, Queue)
  // accepts input. Mirrors loginDone directly: nothing can run without a
  // logged in browser session, so the whole panel stays faded and
  // unclickable until login is confirmed. This used to be tested inline in
  // AgentRunner.tsx's markup; moved here so the page has one place to read
  // "what's locked" from instead of re-deriving it against loginDone
  // itself. See chala2001/grc-tools#150.
  runSectionEditable: boolean;
  // Whether the prompt field accepts typing. False once a task has finished
  // — there is nothing left to queue against, so the box says so rather than
  // accepting text it can't submit. True the rest of the time, including
  // while a task is queued or running: a run takes minutes and drafting the
  // next prompt during one is worth keeping. See chala2001/grc-tools#139.
  promptEditable: boolean;
  // Whether the Advanced settings panel can be opened and its controls
  // changed. False while a task is running or has finished, so a setting
  // can't be changed mid run or against a result that's no longer live.
  advancedSettingsEditable: boolean;
  // What the primary button offers.
  primaryAction: AgentRunnerPrimaryAction;
  // Whether pressing the primary button right now would do anything.
  primaryActionEnabled: boolean;
  // What the panel below a task's result offers: a way to clear a still
  // running task and start over, or a genuine new task once one has
  // finished.
  resultPanelAction: "startFresh" | "newTask";
};

/**
 * Works out what the Agent Runner's run form should look like: whether the
 * prompt can be typed into, what the primary button offers and whether it's
 * enabled, whether Advanced settings can be touched, and what the panel
 * below a result offers. Plain data in, plain data out — no React, no
 * knowledge of how any of this is drawn.
 *
 * Used to be five separate ad hoc checks against two booleans ("a task is
 * running", "a task has finished"), each restated at its own call site in
 * AgentRunner.tsx, roughly ten of them. Pulled out here so the rule for each
 * lives in one place — see ticket chala2001/grc-tools#138.
 *
 * Deliberately does not cover the effects that poll the Agent Task and
 * subscribe to its live updates. Those decide whether to keep talking to
 * the backend, not what the form looks like, and stay exactly as they were.
 */
export function computeAgentRunnerFormState({
  loginDone,
  taskStatus,
  queueing,
  promptEmpty,
  unacknowledgedChangingSteps,
}: ComputeAgentRunnerFormStateArgs): AgentRunnerFormState {
  const isDone = taskStatus !== null && ["completed", "failed", "cancelled"].includes(taskStatus);
  const isRunning = taskStatus !== null && !isDone;

  // A finished task takes over the primary button's face before "in flight"
  // is even considered — the two can't be true together (queueing only
  // happens with no task, or a fresh one, in view), but isDone wins if it
  // ever did.
  const primaryAction: AgentRunnerPrimaryAction = isDone
    ? "newTask"
    : queueing
      ? "queuing"
      : isRunning
        ? taskStatus === "queued"
          ? "waitingForRunner"
          : "runningAgent"
        : "queue";

  // "newTask" skips the queue guards that have nothing to do with it — an
  // empty prompt, a request already in flight and an unacknowledged
  // changing step don't stop a finished task being cleared: there's no
  // prompt left to queue against, so nothing about its text still matters.
  // It still needs loginDone, because the whole form is made unclickable
  // without it: an enabled button inside an unclickable form looks
  // pressable and isn't, which is worse than a disabled one. The button
  // below the result stays available either way. See
  // chala2001/grc-tools#139 and chala2001/grc-tools#140.
  const primaryActionEnabled = isDone
    ? loginDone
    : loginDone && !promptEmpty && !queueing && !isRunning && !unacknowledgedChangingSteps;

  return {
    controlLinkEditable: true,
    runSectionEditable: loginDone,
    promptEditable: !isDone,
    advancedSettingsEditable: !isRunning && !isDone,
    primaryAction,
    primaryActionEnabled,
    resultPanelAction: isDone ? "newTask" : "startFresh",
  };
}
