import { useEffect, useRef, useState } from "react";
import { getFileUrl } from "../api/client";
import { useQueryClient } from "@tanstack/react-query";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import Button from "@mui/material/Button";
import Alert from "@mui/material/Alert";
import CircularProgress from "@mui/material/CircularProgress";
import Chip from "@mui/material/Chip";
import Fab from "@mui/material/Fab";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import Divider from "@mui/material/Divider";
import Switch from "@mui/material/Switch";
import FormControlLabel from "@mui/material/FormControlLabel";
import Collapse from "@mui/material/Collapse";
import { BoltIcon, ArrowRightIcon, CircleCheckFilledIcon, LightbulbOnIcon, XMarkIcon } from "@oxygen-ui/react-icons";
import { agentApi, getAuthToken } from "../api/client";
import { BACKEND_BASE_URL } from "../config/apiConfig";
import ControlPicker from "../components/ControlPicker";
import ProductPicker from "../components/ProductPicker";
import FrameworkPicker from "../components/FrameworkPicker";
import "../index.css";

// ── Portal presets ────────────────────────────────────────────────────────

const PORTAL_PRESETS: { label: string; url: string }[] = [
  { label: "Azure Portal", url: "https://portal.azure.com" },
  { label: "AWS Console", url: "https://console.aws.amazon.com" },
  { label: "WSO2 Identity Server (Cloud)", url: "https://console.asgardeo.io" },
  { label: "Custom URL", url: "" },
];

// ── Session persistence ────────────────────────────────────────────────────

const SS_PREFIX = "compliance.agent.v2.";

// REST poll only acts as a fallback once the SSE stream has been silent this long.
const SSE_SILENCE_THRESHOLD_MS = 15_000;

// Max consecutive failed SSE reconnect attempts before giving up on the stream
// (REST polling fallback continues to cover the UI after this point).
const SSE_MAX_CONSECUTIVE_FAILURES = 5;

// Give up waiting for the local runner to open the browser after this long.
const LOGIN_POLL_TIMEOUT_MS = 60_000;

function useSessionState<T>(key: string, initial: T): [T, React.Dispatch<React.SetStateAction<T>>] {
  const fullKey = SS_PREFIX + key;
  const [value, setValue] = useState<T>(() => {
    try {
      const stored = sessionStorage.getItem(fullKey);
      return stored !== null ? (JSON.parse(stored) as T) : initial;
    } catch {
      return initial;
    }
  });
  useEffect(() => {
    try { sessionStorage.setItem(fullKey, JSON.stringify(value)); } catch { /* ignore */ }
  }, [fullKey, value]);
  return [value, setValue];
}

function clearSessionState(...keys: string[]) {
  keys.forEach((k) => sessionStorage.removeItem(SS_PREFIX + k));
}

// ── Prompt parsing ─────────────────────────────────────────────────────────

const SUBTASK_RE = /^\s*(?:\d+[.)\-:]?|[-*•►▶→])\s+(.+)$/;

function parseSubtasksClient(prompt: string): string[] {
  const lines = prompt.trim().split("\n");
  const tasks: string[][] = [];
  let current: string[] = [];
  for (const line of lines) {
    const m = line.match(SUBTASK_RE);
    if (m) {
      if (current.length) tasks.push(current);
      current = [m[1].trim()];
    } else if (current.length && line.trim()) {
      current.push(line.trim());
    }
  }
  if (current.length) tasks.push(current);
  const joined = tasks.map((t) => t.join("\n").trim()).filter(Boolean);
  return joined.length ? joined : prompt.trim() ? [prompt.trim()] : [];
}

// ── Types ──────────────────────────────────────────────────────────────────

type Usage = {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  llm_calls: number;
  cost_usd: number;
  model: string;
};

type SubtaskState = {
  index: number;
  text: string;
  status: "pending" | "running" | "completed";
  result?: string | null;
  screenshots?: { file_name: string; file_url: string; subtask: string; subtask_index: number; scroll_index?: number }[];
  evidence_id?: number | null;
  submission_id?: number | null;
  started_at?: number;
  completed_at?: number;
  usage?: Usage;
};

type RunState = {
  run_id: string;
  status: "starting" | "running" | "completed" | "error";
  current_index: number;
  subtasks: SubtaskState[];
  error?: string | null;
  started_at?: number;
  completed_at?: number | null;
  total_usage?: Usage;
};

type TaskOut = {
  id: number;
  user_email: string;
  prompt: string;
  region_hint: string | null;
  control_id: number | null;
  // Frozen at task creation, and NOT updated if the Control is later
  // renamed or deleted (see the backend's app/models/agent_task.py comment).
  // Present even after control_id goes null, so a run whose Control was
  // deleted can still be told apart from a login task or a Control-less
  // run, which have neither.
  control_ref_snapshot?: string | null;
  control_title_snapshot?: string | null;
  title: string | null;
  kind: "run" | "login" | "reset";
  status: "queued" | "running" | "completed" | "failed" | "cancelled";
  runner_id: string | null;
  progress: { subtasks?: SubtaskState[]; current_index?: number; total_usage?: Usage; paused?: boolean; pause_message?: string } | null;
  result: Record<string, unknown> | null;
  error: string | null;
  created_at: string;
  started_at: string | null;
  completed_at: string | null;
};

function buildRunState(task: TaskOut): RunState {
  const p = task.progress ?? {};
  const subtasks: SubtaskState[] = p.subtasks ?? [];
  const status: RunState["status"] =
    task.status === "completed" ? "completed" :
    task.status === "failed" || task.status === "cancelled" ? "error" :
    task.status === "running" ? "running" :
    "starting";
  return {
    run_id: String(task.id),
    status,
    current_index: p.current_index ?? subtasks.findIndex((s) => s.status === "running"),
    subtasks,
    error: task.error,
    started_at: task.started_at ? new Date(task.started_at).getTime() / 1000 : undefined,
    completed_at: task.completed_at ? new Date(task.completed_at).getTime() / 1000 : undefined,
    total_usage: p.total_usage,
  };
}

// ── Formatters ─────────────────────────────────────────────────────────────

function formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M";
  if (n >= 1_000) return (n / 1_000).toFixed(1) + "k";
  return String(n);
}

function formatCost(usd: number): string {
  if (usd === 0) return "free";
  if (usd < 0.01) return `$${usd.toFixed(5)}`;
  if (usd < 1) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}

// ── Main component ─────────────────────────────────────────────────────────

export default function AgentRunner() {
  const queryClient = useQueryClient();

  // Form state
  const [productId, setProductId] = useSessionState<number | "">("productId", "");
  const [frameworkId, setFrameworkId] = useSessionState<number | "">("frameworkId", "");
  const [controlId, setControlId] = useSessionState<number | "">("controlId", "");
  const [title, setTitle] = useSessionState<string>("title", "");
  const [prompt, setPrompt] = useSessionState<string>("prompt", "");
  const [regionHint, setRegionHint] = useSessionState<string>("regionHint", "");
  const [complexity, setComplexity] = useSessionState<"quick" | "standard" | "thorough">("complexity", "standard");

  // Agent settings
  const [useVision, setUseVision] = useSessionState<boolean>("useVision", false);
  const [maxActionsPerStep, setMaxActionsPerStep] = useSessionState<number>("maxActionsPerStep", 1);
  const [settingsOpen, setSettingsOpen] = useSessionState<boolean>("settingsOpen", false);

  // Step 1 — manual login
  const [portalPreset, setPortalPreset] = useSessionState<string>("portalPreset", "Azure Portal");
  const [portalUrl, setPortalUrl] = useSessionState<string>("portalUrl", "https://portal.azure.com");
  const [openingPortal, setOpeningPortal] = useState(false);
  const [browserUrl, setBrowserUrl] = useSessionState<string | null>("browserUrl", null);
  const [loginDone, setLoginDone] = useSessionState<boolean>("loginDone", false);
  const [portalError, setPortalError] = useState<string | null>(null);
  const [loginTaskId, setLoginTaskId] = useState<number | null>(null);

  // Task tracking (Step 2 — the real agent run)
  const [taskId, setTaskId] = useSessionState<number | null>("taskId", null);
  const [taskOut, setTaskOut] = useSessionState<TaskOut | null>("taskOut", null);
  const [queueing, setQueueing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastInvalidatedRef = useRef(0);
  const lastSseAtRef = useRef(0);

  // Runner status
  const [runnerOnline, setRunnerOnline] = useState(false);

  // Help dialog
  const [helpOpen, setHelpOpen] = useState(false);
  const [helpSeen, setHelpSeen] = useSessionState<boolean>("helpSeen", false);

  const parsedTasks = parseSubtasksClient(prompt);
  const maxStepsForComplexity = complexity === "quick" ? 15 : complexity === "thorough" ? 40 : 25;

  const isDone = taskOut ? ["completed", "failed", "cancelled"].includes(taskOut.status) : false;
  const isRunning = !!taskOut && !isDone;

  // Poll runner status every 10 s
  useEffect(() => {
    let cancel = false;
    const check = async () => {
      try {
        const data = await agentApi.runnerStatus();
        if (!cancel) setRunnerOnline(data.online);
      } catch { /* ignore */ }
    };
    check();
    const iv = setInterval(check, 10_000);
    return () => { cancel = true; clearInterval(iv); };
  }, []);

  // REST polling fallback — checks in every 8s while task is active, but only
  // actually fetches (and applies) a snapshot once the SSE stream has gone
  // quiet for a while. This avoids a slow REST response racing a fresher SSE
  // frame and making progress appear to jump backwards.
  useEffect(() => {
    if (!taskId || isDone) return;
    let cancel = false;
    const poll = async () => {
      // Start after 8s so SSE has a chance to deliver first
      await new Promise<void>((r) => setTimeout(r, 8_000));
      while (!cancel) {
        if (Date.now() - lastSseAtRef.current < SSE_SILENCE_THRESHOLD_MS) {
          // SSE is fresh — the poll stays dormant so it can't race SSE state.
          if (!cancel) await new Promise<void>((r) => setTimeout(r, 8_000));
          continue;
        }
        try {
          const data: TaskOut = await agentApi.getTask(taskId);
          if (!cancel) {
            setTaskOut(data);
            if (["completed", "failed", "cancelled"].includes(data.status)) {
              queryClient.invalidateQueries({ queryKey: ["evidence"] });
              queryClient.invalidateQueries({ queryKey: ["submissions"] });
              break;
            }
          }
        } catch { /* ignore */ }
        if (!cancel) await new Promise<void>((r) => setTimeout(r, 8_000));
      }
    };
    poll();
    return () => { cancel = true; };
  }, [taskId, isDone, queryClient, setTaskOut]);

  // Poll the login task until the runner has actually opened the browser
  useEffect(() => {
    if (!loginTaskId) return;
    let cancel = false;
    const startedAt = Date.now();
    const poll = async () => {
      while (!cancel) {
        if (Date.now() - startedAt > LOGIN_POLL_TIMEOUT_MS) {
          if (!cancel) setPortalError("Timed out opening the browser. Is the local runner running?");
          setOpeningPortal(false);
          setLoginTaskId(null);
          break;
        }
        try {
          const data: TaskOut = await agentApi.getTask(loginTaskId);
          if (cancel) break;
          if (data.status === "completed") {
            setBrowserUrl(data.prompt);
            setOpeningPortal(false);
            setLoginTaskId(null);
            break;
          }
          if (data.status === "failed" || data.status === "cancelled") {
            setPortalError(data.error || "Failed to open browser. Is the local runner running?");
            setOpeningPortal(false);
            setLoginTaskId(null);
            break;
          }
        } catch {
          if (!cancel) setPortalError("Lost connection to backend while opening the browser.");
          setOpeningPortal(false);
          setLoginTaskId(null);
          break;
        }
        await new Promise((r) => setTimeout(r, 1200));
      }
    };
    poll();
    return () => { cancel = true; };
  }, [loginTaskId, setBrowserUrl]);

  // SSE stream for live task progress — replaces 2s polling
  useEffect(() => {
    if (!taskId || isDone) return;
    const ctrl = new AbortController();

    const listen = async () => {
      // Consecutive failed reconnect attempts — capped so a permanently-down
      // backend doesn't get hammered forever (REST polling covers the UI once
      // we give up here).
      let consecutiveFailures = 0;
      while (!ctrl.signal.aborted) {
        let streamEnded = false;
        try {
          const token = await getAuthToken();
          // Bypasses axios (no interceptors for SSE), so it needs the same
          // dual-mode base client.ts uses — see issue #90.
          const resp = await fetch(`${BACKEND_BASE_URL}/api/agent/tasks/${taskId}/stream`, {
            headers: {
              ...(token ? { Authorization: `Bearer ${token}` } : {}),
            },
            signal: ctrl.signal,
          });
          if (!resp.ok || !resp.body) {
            // treat as a transient failure (e.g. 401 after token expiry, 502
            // gateway) — fall into the same catch/backoff path below instead
            // of permanently ending the stream.
            throw new Error(`SSE stream request failed with status ${resp.status}`);
          }
          consecutiveFailures = 0;

          const reader = resp.body.getReader();
          const dec = new TextDecoder();
          let buf = "";

          while (true) {
            const { done, value } = await reader.read();
            if (done) break; // gateway/server closed stream — don't assume task is done
            buf += dec.decode(value, { stream: true });
            const parts = buf.split("\n\n");
            buf = parts.pop() ?? "";
            for (const part of parts) {
              const line = part.trim();
              if (!line.startsWith("data: ")) continue;
              try {
                const data: TaskOut = JSON.parse(line.slice(6));
                setTaskOut(data);
                lastSseAtRef.current = Date.now();
                const completed = (data.progress?.subtasks ?? [])
                  .filter((s: any) => s.evidence_id).length;
                if (completed > lastInvalidatedRef.current) {
                  queryClient.invalidateQueries({ queryKey: ["evidence"] });
                  queryClient.invalidateQueries({ queryKey: ["submissions"] });
                  lastInvalidatedRef.current = completed;
                }
                if (["completed", "failed", "cancelled"].includes(data.status)) {
                  streamEnded = true;
                }
              } catch { /* ignore malformed events */ }
            }
            if (streamEnded) break;
          }

          // Stream closed without a terminal event — poll REST once to sync state
          if (!streamEnded) {
            try {
              const current: TaskOut = await agentApi.getTask(taskId!);
              setTaskOut(current);
              if (["completed", "failed", "cancelled"].includes(current.status)) {
                streamEnded = true;
              }
            } catch { /* ignore */ }
          }
        } catch (err: any) {
          if (err.name === "AbortError") return;
          consecutiveFailures += 1;
          // on network/transient error: fall back to a single fetch so UI stays up to date
          try {
            const data: TaskOut = await agentApi.getTask(taskId!);
            setTaskOut(data);
            if (["completed", "failed", "cancelled"].includes(data.status)) {
              streamEnded = true;
            }
          } catch { /* ignore */ }
          if (!streamEnded && consecutiveFailures >= SSE_MAX_CONSECUTIVE_FAILURES) {
            // Give up reconnecting after repeated back-to-back failures; the
            // REST polling fallback effect keeps the UI in sync from here.
            break;
          }
        }
        if (streamEnded) break;
        // wait 2s before reconnecting after an unexpected stream drop
        await new Promise<void>((resolve) => {
          const t = setTimeout(resolve, 2000);
          ctrl.signal.addEventListener("abort", () => { clearTimeout(t); resolve(); });
        });
      }
    };

    listen();
    return () => ctrl.abort();
  }, [taskId, isDone, queryClient, setTaskOut]);

  const handlePresetChange = (value: string) => {
    setPortalPreset(value);
    const preset = PORTAL_PRESETS.find((p) => p.label === value);
    if (preset && preset.url) setPortalUrl(preset.url);
    else setPortalUrl("");
  };

  const handleOpenPortal = async () => {
    if (!portalUrl.trim()) {
      setPortalError("Please enter a URL");
      return;
    }
    setOpeningPortal(true);
    setPortalError(null);
    setLoginDone(false);
    setBrowserUrl(null);
    try {
      const task: TaskOut = await agentApi.openLoginBrowser(portalUrl.trim());
      setLoginTaskId(task.id);
    } catch (err: any) {
      setPortalError(err.response?.data?.detail || "Failed to queue login task. Check backend logs.");
      setOpeningPortal(false);
    }
  };

  const handleResetBrowser = async () => {
    setPortalError(null);
    setLoginDone(false);
    setBrowserUrl(null);
    setOpeningPortal(false);
    setLoginTaskId(null);
    try {
      await agentApi.resetBrowser();
      setPortalError('Browser reset requested. Click "Open Browser & Login" to start a fresh session.');
    } catch (err: any) {
      setPortalError(err.response?.data?.detail || "Failed to reset browser.");
    }
  };

  const handleQueue = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!loginDone || !prompt.trim() || isRunning) return;
    setQueueing(true);
    setError(null);
    lastInvalidatedRef.current = 0;
    try {
      const task: TaskOut = await agentApi.createTask({
        prompt,
        control_id: controlId ? Number(controlId) : undefined,
        title: title || undefined,
        region_hint: regionHint || undefined,
        portal_url: browserUrl || undefined,
        max_steps: maxStepsForComplexity,
        use_vision: useVision,
        max_actions_per_step: maxActionsPerStep,
      });
      setTaskId(task.id);
      setTaskOut(task);
    } catch (err: any) {
      setError(err.response?.data?.detail || "Failed to queue task. Check backend logs.");
    } finally {
      setQueueing(false);
    }
  };

  const handleCancel = async () => {
    if (!taskId) return;
    try {
      await agentApi.cancelTask(taskId);
      setTaskOut((prev) => prev ? { ...prev, status: "cancelled" } : prev);
    } catch (err: any) {
      setError(err.response?.data?.detail || "Failed to cancel task.");
    }
  };

  const handleResume = async () => {
    if (!taskId) return;
    try {
      await agentApi.resumeTask(taskId);
      // Optimistically clear the paused flag so the banner hides at once; the
      // runner will confirm by posting progress with paused=false shortly after.
      setTaskOut((prev) =>
        prev && prev.progress
          ? { ...prev, progress: { ...prev.progress, paused: false } }
          : prev
      );
    } catch (err: any) {
      setError(err.response?.data?.detail || "Failed to resume task.");
    }
  };

  const handleNewTask = () => {
    clearSessionState("taskId", "taskOut", "prompt", "productId", "frameworkId", "controlId", "title", "useVision", "maxActionsPerStep");
    setTaskId(null);
    setTaskOut(null);
    setPrompt("");
    setProductId("");
    setFrameworkId("");
    setControlId("");
    setTitle("");
    setError(null);
    lastInvalidatedRef.current = 0;
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const runState = taskOut ? buildRunState(taskOut) : null;

  return (
    <Box sx={{ maxWidth: 760, mx: "auto" }}>
      {/* ── Header ─────────────────────────────────────────────────────── */}
      <Box sx={{ textAlign: "center", mb: 4 }}>
        <Box
          sx={{
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            width: 56, height: 56, borderRadius: "50%",
            backgroundColor: "rgba(255,115,0,0.10)", color: "primary.main", mb: 1.5,
          }}
        >
          <BoltIcon size={28} />
        </Box>
        <Typography variant="h4" gutterBottom>AI Agent Runner</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 560, mx: "auto" }}>
          Describe what to navigate and capture. Your local runner will control a real browser
          automatically. Optionally link the screenshot to a compliance control to auto-create
          an evidence record.
        </Typography>
      </Box>

      {/* ── Runner status indicator ────────────────────────────────────── */}
      <Stack direction="row" alignItems="center" spacing={1} justifyContent="center" sx={{ mb: 3 }}>
        <Box
          sx={{
            width: 10, height: 10, borderRadius: "50%",
            backgroundColor: runnerOnline ? "success.main" : "error.main",
            boxShadow: runnerOnline ? "0 0 6px rgba(76,175,80,0.6)" : "none",
          }}
        />
        <Typography variant="caption" color="text.secondary">
          {runnerOnline ? "Local runner is online" : "No runner connected — tasks will wait in queue"}
        </Typography>
      </Stack>

      {/* ── Step 1 — Open browser & log in manually ───────────────────── */}
      <Paper variant="outlined" sx={{ p: { xs: 3, sm: 4 }, mb: 3 }}>
        <Stack spacing={2.5}>
          <Stack direction="row" alignItems="center" spacing={1.25}>
            <Chip label="STEP 1" size="small" color="primary" sx={{ fontWeight: 700, height: 22 }} />
            <Typography variant="subtitle2" color="text.secondary"
              sx={{ textTransform: "uppercase", letterSpacing: "0.04em", fontSize: "0.72rem" }}>
              Open browser &amp; log in manually
            </Typography>
          </Stack>
          <Typography variant="body2" color="text.secondary">
            Pick the portal you want the agent to use. A browser window opens on your local runner's
            machine — log in with your credentials and MFA <strong>yourself</strong>. The agent never
            sees or stores your password.
          </Typography>

          <FormControl fullWidth>
            <InputLabel>Target Portal</InputLabel>
            <Select
              label="Target Portal"
              value={portalPreset}
              onChange={(e) => handlePresetChange(e.target.value as string)}
            >
              {PORTAL_PRESETS.map((p) => (
                <MenuItem key={p.label} value={p.label}>{p.label}</MenuItem>
              ))}
            </Select>
          </FormControl>

          <TextField
            label="URL"
            value={portalUrl}
            onChange={(e) => setPortalUrl(e.target.value)}
            placeholder="https://..."
            fullWidth
            disabled={portalPreset !== "Custom URL"}
          />

          <Stack direction={{ xs: "column", sm: "row" }} spacing={2} alignItems={{ xs: "stretch", sm: "center" }} flexWrap="wrap">
            <Button
              variant="contained"
              color="primary"
              onClick={handleOpenPortal}
              disabled={openingPortal}
              startIcon={openingPortal ? <CircularProgress size={16} color="inherit" /> : <ArrowRightIcon size={18} />}
            >
              {openingPortal ? "Opening browser..." : "Open Browser & Login"}
            </Button>
            {browserUrl && (
              <Button
                variant={loginDone ? "contained" : "outlined"}
                color={loginDone ? "success" : "inherit"}
                onClick={() => setLoginDone(true)}
                startIcon={loginDone ? <CircleCheckFilledIcon size={18} /> : undefined}
              >
                {loginDone ? "Login confirmed" : "I've logged in"}
              </Button>
            )}
            <Box sx={{ flex: 1, display: { xs: "none", sm: "block" } }} />
            <Button
              size="small"
              variant="text"
              color="inherit"
              onClick={handleResetBrowser}
              disabled={openingPortal}
              sx={{ fontSize: "0.78rem", textTransform: "none", color: "text.secondary" }}
            >
              Browser not opening? Reset session
            </Button>
          </Stack>

          {openingPortal && !runnerOnline && (
            <Alert severity="warning">
              No local runner detected yet. Make sure <code>wso2-runner start</code> is running on your machine.
            </Alert>
          )}

          {browserUrl && (
            <Alert severity={loginDone ? "success" : "info"}>
              Browser opened at <strong style={{ wordBreak: "break-all" }}>{browserUrl}</strong>. Complete login + MFA
              in that browser window (on your runner's machine), then click <strong>"I've logged in"</strong> above.
            </Alert>
          )}
          {portalError && (
            <Alert severity={portalError.startsWith("Browser reset") ? "info" : "error"}>{portalError}</Alert>
          )}

          <Divider sx={{ my: 1 }} />

          <Stack direction="row" alignItems="center" spacing={1.25}>
            <Typography variant="subtitle2" color="text.secondary"
              sx={{ textTransform: "uppercase", letterSpacing: "0.04em", fontSize: "0.72rem" }}>
              Environment context
            </Typography>
            <Chip label="Optional but recommended" size="small" variant="outlined" sx={{ height: 20, fontSize: "0.7rem" }} />
          </Stack>
          <Typography variant="body2" color="text.secondary">
            Tell the agent the right region / subscription / workspace. It switches there
            <strong> before</strong> searching, so it doesn't look in the wrong place.
          </Typography>
          <TextField
            label="Environment hint"
            value={regionHint}
            onChange={(e) => setRegionHint(e.target.value)}
            placeholder='e.g. "AWS region: Asia Pacific (Mumbai) ap-south-1" or "Azure subscription: WSO2-Prod"'
            multiline
            rows={2}
            fullWidth
          />
        </Stack>
      </Paper>

      {/* ── Step 2 — Run the AI agent (locked until login confirmed) ───── */}
      <Paper
        variant="outlined"
        sx={{
          p: { xs: 3, sm: 4 }, mb: 3,
          opacity: loginDone ? 1 : 0.55,
          pointerEvents: loginDone ? "auto" : "none",
        }}
        component="form"
        onSubmit={handleQueue}
      >
        <Stack spacing={2.5}>
          <Stack direction="row" alignItems="center" spacing={1.25}>
            <Chip label="STEP 2" size="small" color="primary" sx={{ fontWeight: 700, height: 22 }} />
            <Typography variant="subtitle2" color="text.secondary"
              sx={{ textTransform: "uppercase", letterSpacing: "0.04em", fontSize: "0.72rem" }}>
              Run the AI agent
            </Typography>
          </Stack>

          <Stack direction="row" alignItems="center" spacing={1.25}>
            <Typography variant="subtitle2" color="text.secondary"
              sx={{ textTransform: "uppercase", letterSpacing: "0.04em", fontSize: "0.72rem" }}>
              Link to compliance control
            </Typography>
            <Chip label="Optional" size="small" variant="outlined" sx={{ height: 20, fontSize: "0.7rem" }} />
          </Stack>

          <ProductPicker
            value={productId}
            onChange={(id) => { setProductId(id); setFrameworkId(""); setControlId(""); }}
            includeAll
            allLabel="— Just run, don't save as evidence —"
          />
          {productId !== "" && (
            <FrameworkPicker
              productId={productId}
              value={frameworkId}
              onChange={(id) => { setFrameworkId(id); setControlId(""); }}
              placeholderOption="— Select a framework —"
            />
          )}
          {frameworkId !== "" && (
            <ControlPicker
              frameworkId={frameworkId}
              controlId={controlId}
              onControlChange={(id) => setControlId(id)}
            />
          )}
          {controlId !== "" && (
            <TextField
              label="Evidence Title (optional)"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Defaults to start of prompt"
              fullWidth
            />
          )}

          <Divider sx={{ my: 0.5 }} />

          <FormControl fullWidth>
            <InputLabel>Task complexity</InputLabel>
            <Select
              label="Task complexity"
              value={complexity}
              onChange={(e) => setComplexity(e.target.value as "quick" | "standard" | "thorough")}
            >
              <MenuItem value="quick">Quick — 15 steps per task (simple navigation, single location)</MenuItem>
              <MenuItem value="standard">Standard — 25 steps per task (default, multi-step within one region)</MenuItem>
              <MenuItem value="thorough">Thorough — 40 steps per task (multi-region, deep search, complex)</MenuItem>
            </Select>
          </FormControl>

          {/* ── Agent Settings (collapsible) ──────────────────────────── */}
          <Box sx={{ opacity: (isRunning || isDone) ? 0.45 : 1, pointerEvents: (isRunning || isDone) ? "none" : "auto" }}>
            <Button
              size="small"
              variant="text"
              onClick={() => setSettingsOpen(!settingsOpen)}
              sx={{ textTransform: "none", color: "text.secondary", fontSize: "0.8rem", px: 0, mb: 0.5 }}
            >
              {settingsOpen ? "▾ Hide advanced settings" : "▸ Advanced settings"}
            </Button>
            <Collapse in={settingsOpen}>
              <Paper
                variant="outlined"
                sx={{ p: 2.5, backgroundColor: "rgba(255,115,0,0.03)", borderColor: "rgba(255,115,0,0.2)" }}
              >
                <Stack spacing={2.5}>
                  <Typography variant="caption" fontWeight={700}
                    sx={{ textTransform: "uppercase", letterSpacing: "0.06em", color: "text.secondary" }}>
                    Agent Behaviour Settings
                  </Typography>

                  {/* Vision Mode */}
                  <Box>
                    <FormControlLabel
                      control={
                        <Switch
                          checked={useVision}
                          onChange={(e) => setUseVision(e.target.checked)}
                          color="primary"
                        />
                      }
                      label={
                        <Box>
                          <Typography variant="body2" fontWeight={600}>
                            Agent Vision
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            {useVision
                              ? "ON — Agent takes a screenshot after every action. Use for complex or image-heavy pages."
                              : "OFF — Agent reads page structure directly. Faster, works for standard websites like AWS & Azure."}
                          </Typography>
                        </Box>
                      }
                      sx={{ alignItems: "flex-start", m: 0 }}
                    />
                  </Box>

                  <Divider />

                  {/* Actions per step */}
                  <Box>
                    <Typography variant="body2" fontWeight={600} gutterBottom>
                      Agent Speed
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1.5 }}>
                      How many actions the agent can take before checking in with the AI.
                      Higher = faster but less careful. Start at 1 for new tasks.
                    </Typography>
                    <Stack direction="row" spacing={1} flexWrap="wrap">
                      {[
                        { value: 1, label: "Careful", desc: "1 action" },
                        { value: 3, label: "Balanced", desc: "3 actions" },
                        { value: 5, label: "Fast", desc: "5 actions" },
                        { value: 10, label: "Maximum", desc: "10 actions" },
                      ].map((opt) => (
                        <Button
                          key={opt.value}
                          variant={maxActionsPerStep === opt.value ? "contained" : "outlined"}
                          size="small"
                          onClick={() => setMaxActionsPerStep(opt.value)}
                          sx={{
                            textTransform: "none",
                            flexDirection: "column",
                            lineHeight: 1.2,
                            px: 1.5,
                            py: 0.75,
                            minWidth: 80,
                          }}
                        >
                          <span style={{ fontWeight: 700 }}>{opt.label}</span>
                          <span style={{ fontSize: "0.68rem", opacity: 0.8 }}>{opt.desc}</span>
                        </Button>
                      ))}
                    </Stack>
                  </Box>

                  <Box sx={{ backgroundColor: "rgba(0,0,0,0.04)", borderRadius: 1, p: 1.25 }}>
                    <Typography variant="caption" color="text.secondary">
                      <strong>Current settings:</strong> Vision Mode {useVision ? "ON" : "OFF"} ·
                      Speed: {maxActionsPerStep === 1 ? "Careful" : maxActionsPerStep === 3 ? "Balanced" : maxActionsPerStep === 5 ? "Fast" : "Maximum"} ({maxActionsPerStep} action{maxActionsPerStep > 1 ? "s" : ""} per step) ·
                      Max steps: {maxStepsForComplexity} ({complexity})
                    </Typography>
                  </Box>
                </Stack>
              </Paper>
            </Collapse>
          </Box>

          <Alert severity="info" sx={{ "& .MuiAlert-message": { width: "100%" } }}>
            <Typography variant="body2" sx={{ mb: 0.5 }}>
              <strong>Tip:</strong> Use a numbered list to capture <strong>multiple screenshots in one run</strong>.
            </Typography>
            <Box component="pre" sx={{ m: 0, p: 1, fontSize: "0.78rem", backgroundColor: "rgba(0,0,0,0.04)", borderRadius: 1, whiteSpace: "pre-wrap" }}>
{`1. Go to S3, find bucket "cloudcare-k8s", screenshot the objects list
2. Go to EC2, find instance "cloud-care", screenshot the details page
3. Go to DynamoDB, find table "cloudcare-k8s", screenshot the items view`}
            </Box>
            <Typography variant="body2" sx={{ mt: 1.5, mb: 0.5 }}>
              <strong>Don't know the count in advance?</strong> Use <code>EACH:</code> to repeat a step for every
              item the agent finds, or <code>EACH-PAGE:</code> to capture the first and last page of a result set.
            </Typography>
            <Box component="pre" sx={{ m: 0, p: 1, fontSize: "0.78rem", backgroundColor: "rgba(0,0,0,0.04)", borderRadius: 1, whiteSpace: "pre-wrap" }}>
{`1. Go to Key Vaults (no filter)
2. EACH: Open Key Vault "{item}" and screenshot its Properties page`}
            </Box>
            <Box component="pre" sx={{ m: 0, mt: 1, p: 1, fontSize: "0.78rem", backgroundColor: "rgba(0,0,0,0.04)", borderRadius: 1, whiteSpace: "pre-wrap" }}>
{`1. Go to Key Vaults, filter by label "env:prod"
2. EACH-PAGE: Screenshot page {page} of the filtered results`}
            </Box>
            <Typography variant="body2" sx={{ mt: 1.5, mb: 0.5 }}>
              <strong>Need a PDF instead of a screenshot?</strong> Use <code>PDF:</code> — e.g. for a long
              GitHub issue thread. The agent expands any "Load more" content first, then the whole page is
              exported as a PDF instead of scrolling screenshots.
            </Typography>
            <Box component="pre" sx={{ m: 0, p: 1, fontSize: "0.78rem", backgroundColor: "rgba(0,0,0,0.04)", borderRadius: 1, whiteSpace: "pre-wrap" }}>
{`PDF: Open https://github.com/org/repo/issues/123, expand all comments, then export as PDF`}
            </Box>
          </Alert>

          <TextField
            label="Prompt"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={'Single task — e.g. "Go to Key Vault X and screenshot the access policy"\n\nOr a numbered list for multiple captures:\n1. Go to S3 cloud-care, screenshot objects\n2. Go to EC2 cloud-care, screenshot details'}
            multiline
            rows={6}
            required
            fullWidth
          />

          {prompt.trim() && (
            <Paper variant="outlined"
              sx={{ p: 1.5, backgroundColor: parsedTasks.length > 1 ? "rgba(76,175,80,0.07)" : "rgba(0,0,0,0.03)" }}>
              <Stack direction="row" alignItems="center" spacing={1} mb={parsedTasks.length > 1 ? 1 : 0}>
                <Chip
                  label={`Detected ${parsedTasks.length} task${parsedTasks.length !== 1 ? "s" : ""}`}
                  size="small"
                  color={parsedTasks.length > 1 ? "success" : "default"}
                  sx={{ fontWeight: 700 }}
                />
                <Typography variant="caption" color="text.secondary">
                  {parsedTasks.length > 1
                    ? "Each will produce one screenshot + one evidence record"
                    : "Single-task mode"}
                </Typography>
              </Stack>
              {parsedTasks.length > 1 && (
                <Stack spacing={0.5} sx={{ pl: 1 }}>
                  {parsedTasks.map((t, i) => (
                    <Typography key={i} variant="caption" sx={{ display: "block", fontFamily: "monospace" }}>
                      <strong>#{i + 1}</strong> {t.length > 100 ? t.slice(0, 100) + "..." : t}
                    </Typography>
                  ))}
                </Stack>
              )}
            </Paper>
          )}

          {error && <Alert severity="error">{error}</Alert>}

          <Button
            type="submit"
            variant="contained"
            size="large"
            disabled={!loginDone || queueing || !prompt.trim() || isRunning || isDone}
            startIcon={(queueing || isRunning) ? <CircularProgress size={16} color="inherit" /> : <ArrowRightIcon size={18} />}
            sx={{ py: 1.25 }}
          >
            {queueing ? "Queuing..." : isRunning ? (taskOut?.status === "queued" ? "Queued..." : "Agent running...") : "Queue Task for Runner"}
          </Button>

          <Typography variant="caption" color="text.secondary" sx={{ textAlign: "center" }}>
            {loginDone
              ? "The task is added to the queue. Your local runner picks it up and reuses your logged-in browser session."
              : 'Complete Step 1 and click "I\'ve logged in" above to unlock this form.'}
            <br />Max steps this run: <strong>{maxStepsForComplexity}</strong> ({complexity})
          </Typography>
        </Stack>
      </Paper>

      {/* ── Task status / run timeline ──────────────────────────────────── */}
      {taskOut && (
        <Box>
          <TaskStatusPanel task={taskOut} runnerOnline={runnerOnline} onCancel={handleCancel} onResume={handleResume} />

          {runState && runState.subtasks.length > 0 && (
            <Box sx={{ mt: 2 }}>
              <RunTimeline runState={runState} error={error} queryClient={queryClient} />
            </Box>
          )}

          <Paper variant="outlined" sx={{ mt: 3, p: 2.5, backgroundColor: isDone ? "rgba(255,115,0,0.05)" : "rgba(0,0,0,0.02)" }}>
            <Stack direction={{ xs: "column", sm: "row" }} alignItems={{ xs: "stretch", sm: "center" }} spacing={2}>
              <Box sx={{ flex: 1 }}>
                <Typography variant="subtitle1" fontWeight={700}>
                  {isDone ? "Task finished — queue another?" : "Start a new task?"}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {isDone
                    ? "Your login session and environment context are kept. Just edit the prompt above and run again."
                    : "Clear this task from the view and start fresh. The current task will keep running in the background."}
                </Typography>
              </Box>
              <Button
                variant={isDone ? "contained" : "outlined"}
                size="large"
                onClick={handleNewTask}
                startIcon={<ArrowRightIcon size={18} />}
                sx={{ minWidth: 200 }}
              >
                {isDone ? "New Task" : "Start Fresh"}
              </Button>
            </Stack>
          </Paper>
        </Box>
      )}

      {/* ── Help FAB ──────────────────────────────────────────────────── */}
      <Tooltip title="Quick guide" placement="left">
        <Fab
          color="primary"
          aria-label="Open quick guide"
          onClick={() => { setHelpOpen(true); setHelpSeen(true); }}
          sx={{
            position: "fixed", bottom: 28, right: 28, zIndex: 1200,
            boxShadow: "0 6px 16px rgba(255,115,0,0.35)",
            animation: helpSeen ? "none" : "pulseHelp 2.4s ease-in-out infinite",
            "@keyframes pulseHelp": {
              "0%, 100%": { transform: "scale(1)", boxShadow: "0 6px 16px rgba(255,115,0,0.35)" },
              "50%": { transform: "scale(1.08)", boxShadow: "0 10px 26px rgba(255,115,0,0.55)" },
            },
          }}
        >
          <LightbulbOnIcon size={22} />
        </Fab>
      </Tooltip>

      <HelpDialog open={helpOpen} onClose={() => setHelpOpen(false)} />
    </Box>
  );
}

// ── TaskStatusPanel ────────────────────────────────────────────────────────

function TaskStatusPanel({
  task,
  runnerOnline,
  onCancel,
  onResume,
}: {
  task: TaskOut;
  runnerOnline: boolean;
  onCancel: () => void;
  onResume: () => void;
}) {
  const canCancel = !["completed", "failed", "cancelled"].includes(task.status);
  const isPaused = !!task.progress?.paused && canCancel;

  const statusColor =
    task.status === "completed" ? "success" :
    task.status === "failed" || task.status === "cancelled" ? "error" :
    task.status === "running" ? "primary" :
    "default";

  const statusLabel =
    task.status === "queued" ? "Queued — waiting for runner" :
    task.status === "running" ? `Running (runner: ${task.runner_id ?? "unknown"})` :
    task.status === "completed" ? "Completed" :
    task.status === "failed" ? "Failed" :
    "Cancelled";

  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Stack spacing={1.5}>
        <Stack direction="row" alignItems="center" spacing={1.5} flexWrap="wrap">
          <Typography variant="subtitle1" fontWeight={700}>
            Task #{task.id}
          </Typography>
          <Chip
            label={statusLabel}
            color={statusColor as any}
            size="small"
            sx={{ fontWeight: 700 }}
            icon={task.status === "running" ? <CircularProgress size={12} color="inherit" /> : undefined}
          />
          {canCancel && (
            <Button size="small" color="error" variant="outlined" onClick={onCancel} sx={{ ml: "auto" }}>
              Cancel
            </Button>
          )}
        </Stack>

        {isPaused && (
          <Alert
            severity="warning"
            action={
              <Button color="warning" size="small" variant="contained" onClick={onResume} sx={{ fontWeight: 700 }}>
                ▶ Resume
              </Button>
            }
          >
            {task.progress?.pause_message ||
              "Paused — set up your filters in the browser, then click Resume."}
          </Alert>
        )}

        {task.status === "queued" && (
          <Alert
            severity={runnerOnline ? "info" : "warning"}
            icon={<CircularProgress size={18} />}
          >
            {runnerOnline
              ? "Runner is online — it will pick this up momentarily."
              : "No runner connected. Start the local runner on your machine to process this task."}
          </Alert>
        )}

        {task.error && (
          <Alert severity="error">{task.error}</Alert>
        )}

        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
          Prompt: {task.prompt.length > 120 ? task.prompt.slice(0, 120) + "…" : task.prompt}
        </Typography>

        <Stack direction="row" spacing={0.75} flexWrap="wrap" rowGap={0.5}>
          {task.control_id ? (
            <Chip label={`Control #${task.control_id}`} size="small" variant="outlined" />
          ) : task.control_title_snapshot || task.control_ref_snapshot ? (
            // control_id is null but a snapshot survives -- this run's
            // Control was deleted after the run happened, not a login task
            // or a Control-less run (those have no snapshot either).
            <Chip
              label={`${task.control_ref_snapshot ?? task.control_title_snapshot} (deleted)`}
              size="small"
              variant="outlined"
            />
          ) : null}
          {task.region_hint && (
            <Chip label={`Hint: ${task.region_hint.slice(0, 40)}`} size="small" variant="outlined" />
          )}
          <Chip
            label={new Date(task.created_at).toLocaleString()}
            size="small"
            variant="outlined"
            sx={{ color: "text.secondary" }}
          />
        </Stack>
      </Stack>
    </Paper>
  );
}

// ── RunTimeline ────────────────────────────────────────────────────────────

function RunTimeline({
  runState,
  error,
}: {
  runState: RunState;
  error: string | null;
  queryClient: ReturnType<typeof useQueryClient>;
}) {
  const total = runState.subtasks.length;
  const completedCount = runState.subtasks.reduce((acc, s) => acc + (s.screenshots?.length ?? 0), 0);
  const currentIdx = runState.current_index;
  const evidenceIds = runState.subtasks.map((s) => s.evidence_id).filter((x): x is number => !!x);
  const overallStatus = runState.status;
  const elapsed =
    runState.started_at
      ? Math.round((runState.completed_at || Date.now() / 1000) - runState.started_at)
      : 0;

  return (
    <Stack spacing={2}>
      <Paper variant="outlined" sx={{ p: 2 }}>
        <Stack direction="row" alignItems="center" spacing={2} flexWrap="wrap">
          <Chip
            label={
              overallStatus === "completed" ? "All tasks done" :
              overallStatus === "error" ? "Error" :
              overallStatus === "starting" ? "Starting agent..." :
              `Running task ${currentIdx + 1} of ${total}`
            }
            color={
              overallStatus === "completed" ? "success" :
              overallStatus === "error" ? "error" :
              "primary"
            }
            sx={{ fontWeight: 700 }}
            icon={overallStatus === "running" || overallStatus === "starting"
              ? <CircularProgress size={14} color="inherit" />
              : undefined}
          />
          <Typography variant="caption" color="text.secondary">
            {completedCount} screenshot{completedCount !== 1 ? "s" : ""} captured across {total} task{total !== 1 ? "s" : ""} · {elapsed}s elapsed
            {evidenceIds.length > 0 && ` · Evidence ${evidenceIds.map((id) => `#${id}`).join(", ")}`}
          </Typography>
        </Stack>

        {runState.total_usage && runState.total_usage.total_tokens > 0 && (
          <Stack direction="row" spacing={1.25} alignItems="center"
            sx={{ mt: 1.5, pt: 1.25, borderTop: "1px solid", borderColor: "divider", flexWrap: "wrap", rowGap: 0.75 }}>
            <Typography variant="caption" fontWeight={700}
              sx={{ textTransform: "uppercase", letterSpacing: "0.04em", color: "text.secondary" }}>
              LLM usage
            </Typography>
            <Chip size="small" variant="outlined" label={`${formatTokens(runState.total_usage.input_tokens)} in`} sx={{ height: 22, fontWeight: 600 }} />
            <Chip size="small" variant="outlined" label={`${formatTokens(runState.total_usage.output_tokens)} out`} sx={{ height: 22, fontWeight: 600 }} />
            <Chip size="small" variant="outlined" label={`${runState.total_usage.llm_calls} calls`} sx={{ height: 22, fontWeight: 600 }} />
            <Chip size="small" color="primary" label={`Total cost: ${formatCost(runState.total_usage.cost_usd)}`} sx={{ height: 22, fontWeight: 700 }} />
            <Typography variant="caption" color="text.disabled">({runState.total_usage.model})</Typography>
          </Stack>
        )}

        {overallStatus === "error" && error && (
          <Alert severity="error" sx={{ mt: 1.5 }}>{error}</Alert>
        )}
      </Paper>

      {runState.subtasks.map((task) => (
        <Paper key={task.index} variant="outlined" sx={{ overflow: "hidden" }}>
          <Box sx={{ px: 2, py: 1.25, backgroundColor: "rgba(0,0,0,0.03)", borderBottom: "1px solid", borderColor: "divider" }}>
            <Stack direction="row" alignItems="center" spacing={1.25} flexWrap="wrap">
              <Chip
                label={`Task ${task.index + 1}`}
                size="small"
                color={task.status === "completed" ? "success" : task.status === "running" ? "primary" : "default"}
                sx={{ fontWeight: 700 }}
                icon={
                  task.status === "completed" ? <CircleCheckFilledIcon size={14} /> :
                  task.status === "running" ? <CircularProgress size={12} color="inherit" /> :
                  undefined
                }
              />
              <Typography variant="body2" sx={{ fontWeight: 500, flex: 1, minWidth: 0 }}>
                {task.text}
              </Typography>
              {task.evidence_id && (
                <Chip label={`Evidence #${task.evidence_id}`} size="small" variant="outlined" />
              )}
            </Stack>
          </Box>

          {task.status === "pending" && (
            <Box sx={{ p: 2, textAlign: "center" }}>
              <Typography variant="caption" color="text.secondary">⏳ Waiting...</Typography>
            </Box>
          )}

          {task.status === "running" && !(task.screenshots?.length) && (
            <Box sx={{ p: 2, textAlign: "center" }}>
              <CircularProgress size={20} />
              <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
                Agent is working on this task...
              </Typography>
            </Box>
          )}

          {task.screenshots && task.screenshots.length > 0 && (
            <Box sx={{ p: 1, display: "flex", flexDirection: "column", gap: 1 }}>
              {task.screenshots.map((shot, i) => {
                const isPdf = shot.file_name.toLowerCase().endsWith(".pdf");
                return (
                  <Box key={shot.file_name}>
                    {task.screenshots!.length > 1 && (
                      <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 0.5, pl: 0.5 }}>
                        {isPdf ? "File" : "Screenshot"} {i + 1} of {task.screenshots!.length}
                      </Typography>
                    )}
                    {isPdf ? (
                      <Button
                        variant="outlined"
                        size="small"
                        href={getFileUrl(shot.file_url)}
                        target="_blank"
                        rel="noopener noreferrer"
                        sx={{ textTransform: "none" }}
                      >
                        View PDF — {shot.file_name.split("/").pop()}
                      </Button>
                    ) : (
                      <Box
                        component="img"
                        src={getFileUrl(shot.file_url)}
                        alt={`Screenshot ${i + 1} of task ${task.index + 1}`}
                        sx={{ width: "100%", display: "block", borderRadius: 1 }}
                      />
                    )}
                  </Box>
                );
              })}
            </Box>
          )}

          {task.result && task.status === "completed" && (
            <Box sx={{ px: 2, py: 1.5, borderTop: "1px solid", borderColor: "divider", backgroundColor: "rgba(0,0,0,0.02)" }}>
              <Typography variant="caption" color="text.secondary"
                sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.04em", display: "block", mb: 0.5 }}>
                Agent report
              </Typography>
              <Typography variant="body2" sx={{ fontFamily: "monospace", fontSize: "0.78rem", whiteSpace: "pre-wrap" }}>
                {task.result}
              </Typography>
            </Box>
          )}

          {task.usage && task.status === "completed" && task.usage.total_tokens > 0 && (
            <Box sx={{ px: 2, py: 1, borderTop: "1px solid", borderColor: "divider", backgroundColor: "rgba(255,115,0,0.04)" }}>
              <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" rowGap={0.5}>
                <Typography variant="caption" fontWeight={700}
                  sx={{ textTransform: "uppercase", letterSpacing: "0.04em", color: "text.secondary" }}>
                  Cost
                </Typography>
                <Chip size="small" variant="outlined" label={`${formatTokens(task.usage.input_tokens)} in`} sx={{ height: 20, fontSize: "0.7rem", fontWeight: 600 }} />
                <Chip size="small" variant="outlined" label={`${formatTokens(task.usage.output_tokens)} out`} sx={{ height: 20, fontSize: "0.7rem", fontWeight: 600 }} />
                <Chip size="small" variant="outlined" label={`${task.usage.llm_calls} calls`} sx={{ height: 20, fontSize: "0.7rem", fontWeight: 600 }} />
                <Chip size="small" color="primary" label={formatCost(task.usage.cost_usd)} sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700 }} />
              </Stack>
            </Box>
          )}
        </Paper>
      ))}
    </Stack>
  );
}

// ── HelpDialog ─────────────────────────────────────────────────────────────

function HelpDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ pr: 6, pb: 1.5 }}>
        <Stack direction="row" alignItems="center" spacing={1.5}>
          <Box sx={{ color: "primary.main", display: "flex" }}><LightbulbOnIcon size={24} /></Box>
          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.2 }}>Quick Guide</Typography>
            <Typography variant="caption" color="text.secondary">How to use the AI Agent</Typography>
          </Box>
        </Stack>
        <IconButton onClick={onClose} sx={{ position: "absolute", right: 12, top: 12 }} size="small">
          <XMarkIcon size={18} />
        </IconButton>
      </DialogTitle>

      <DialogContent dividers>
        <Stack spacing={2.75}>

          {/* Step 1 */}
          <Box>
            <Stack direction="row" alignItems="center" spacing={1} mb={1}>
              <Chip label="1" size="small" color="primary" sx={{ fontWeight: 700, minWidth: 26, height: 22 }} />
              <Typography variant="subtitle1" fontWeight={700}>Open browser &amp; log in manually</Typography>
            </Stack>
            <Stack spacing={0.4} sx={{ pl: 4.5 }}>
              <Typography variant="body2">• Pick a target portal — Azure, AWS, WSO2 Identity Server, or a custom URL</Typography>
              <Typography variant="body2">• Click <strong>"Open Browser &amp; Login"</strong> — a real Chrome window opens on your runner's machine</Typography>
              <Typography variant="body2">• Sign in yourself there — including MFA. The agent never sees your password</Typography>
              <Typography variant="body2">• Click <strong>"I've logged in"</strong> when done — this unlocks Step 2 below</Typography>
              <Typography variant="body2">• Use <strong>"Browser not opening? Reset session"</strong> if the browser gets stuck</Typography>
            </Stack>
          </Box>

          {/* Step 2 */}
          <Box>
            <Stack direction="row" alignItems="center" spacing={1} mb={1}>
              <Chip label="2" size="small" color="primary" sx={{ fontWeight: 700, minWidth: 26, height: 22 }} />
              <Typography variant="subtitle1" fontWeight={700}>Fill in the task form</Typography>
            </Stack>
            <Stack spacing={0.4} sx={{ pl: 4.5 }}>
              <Typography variant="body2">• (Optional) Pick a Product → Framework → Control to auto-save evidence</Typography>
              <Typography variant="body2">• Set <strong>Environment Hint</strong> — e.g. <em>"AWS region: Mumbai ap-south-1"</em> or <em>"Azure subscription: WSO2-Prod"</em></Typography>
              <Typography variant="body2">• Choose <strong>Task complexity</strong>: Quick (15 steps) / Standard (25 steps) / Thorough (40 steps)</Typography>
              <Typography variant="body2">• Type your prompt — one line for a single capture, or a numbered list for multiple screenshots in one run</Typography>
              <Typography variant="body2">
                • Don't know how many items there are? Prefix a line with <strong>EACH:</strong> to repeat it for
                every item the agent discovers (e.g. <em>"EACH: Open Key Vault "{"{item}"}" and screenshot its Properties page"</em>),
                or <strong>EACH-PAGE:</strong> to capture just the first and last page of a paginated result set
                (e.g. <em>"EACH-PAGE: Screenshot page {"{page}"} of the filtered results"</em>). The agent looks at the
                page first to find the real count, then runs one subtask per item/page automatically.
              </Typography>
              <Typography variant="body2">
                • Need a PDF instead of screenshots (e.g. a long GitHub issue thread)? Prefix a line with{" "}
                <strong>PDF:</strong> — the agent expands any "Load more"/"Show more comments" content first, then
                exports the whole page as a PDF you can download from the timeline.
              </Typography>
            </Stack>
          </Box>

          {/* Advanced settings */}
          <Box sx={{ backgroundColor: "rgba(0,0,0,0.03)", borderRadius: 1.5, p: 1.75, border: "1px solid", borderColor: "divider" }}>
            <Typography variant="caption" fontWeight={700} sx={{ textTransform: "uppercase", letterSpacing: "0.06em", color: "text.secondary", display: "block", mb: 1 }}>
              Advanced Settings (optional)
            </Typography>
            <Stack spacing={0.5}>
              <Typography variant="body2">
                <strong>Agent Vision (OFF by default)</strong> — when ON, the agent takes a screenshot after every action and uses it to decide the next step. Slower but better for image-heavy or unusual pages. Leave OFF for standard AWS / Azure portals.
              </Typography>
              <Typography variant="body2">
                <strong>Agent Speed</strong> — controls how many browser actions the agent takes before checking with the AI:
              </Typography>
              <Stack spacing={0.25} sx={{ pl: 2 }}>
                <Typography variant="body2">· <strong>Careful (1 action)</strong> — safest, best for new or tricky tasks</Typography>
                <Typography variant="body2">· <strong>Balanced (3 actions)</strong> — good default for most tasks</Typography>
                <Typography variant="body2">· <strong>Fast (5 actions)</strong> — quicker, for simple well-known pages</Typography>
                <Typography variant="body2">· <strong>Maximum (10 actions)</strong> — fastest, use only for very simple navigation</Typography>
              </Stack>
            </Stack>
          </Box>

          {/* Step 3 */}
          <Box>
            <Stack direction="row" alignItems="center" spacing={1} mb={1}>
              <Chip label="3" size="small" color="primary" sx={{ fontWeight: 700, minWidth: 26, height: 22 }} />
              <Typography variant="subtitle1" fontWeight={700}>Click "Queue Task for Runner"</Typography>
            </Stack>
            <Stack spacing={0.4} sx={{ pl: 4.5 }}>
              <Typography variant="body2">• Only enabled once you've confirmed login in Step 1</Typography>
              <Typography variant="body2">• Your local runner picks it up and reuses the browser session you just logged into</Typography>
              <Typography variant="body2">• Progress streams live to this page in real-time — no page refresh needed</Typography>
            </Stack>
          </Box>

          {/* Step 4 */}
          <Box>
            <Stack direction="row" alignItems="center" spacing={1} mb={1}>
              <Chip label="4" size="small" color="primary" sx={{ fontWeight: 700, minWidth: 26, height: 22 }} />
              <Typography variant="subtitle1" fontWeight={700}>Watch the live timeline</Typography>
            </Stack>
            <Stack spacing={0.4} sx={{ pl: 4.5 }}>
              <Typography variant="body2">• Each subtask moves: <strong>pending → running → completed</strong></Typography>
              <Typography variant="body2">• OS-level screenshots (showing URL bar, clock, browser chrome) appear as they're captured</Typography>
              <Typography variant="body2">• Token usage and cost are shown per subtask and as a total</Typography>
              <Typography variant="body2">• If linked to a control, an Evidence record and Submission are created automatically</Typography>
              <Typography variant="body2">• Use <strong>Cancel</strong> at any time to stop a running task</Typography>
            </Stack>
          </Box>

          {/* Pro Tips */}
          <Box sx={{ backgroundColor: "rgba(255,115,0,0.07)", borderRadius: 1.5, p: 1.75, borderLeft: "3px solid", borderColor: "primary.main" }}>
            <Stack direction="row" alignItems="center" spacing={1} mb={1}>
              <Box sx={{ color: "primary.main", display: "flex" }}><LightbulbOnIcon size={18} /></Box>
              <Typography variant="subtitle2" fontWeight={700} sx={{ textTransform: "uppercase", letterSpacing: "0.04em" }}>
                Pro Tips
              </Typography>
            </Stack>
            <Stack spacing={0.5}>
              <Typography variant="body2">★ Always set Environment Hint for cross-region or multi-subscription work</Typography>
              <Typography variant="body2">★ Use <strong>Thorough</strong> complexity when fuzzy-searching across services or regions</Typography>
              <Typography variant="body2">★ Use <strong>Agent Vision ON</strong> only if the agent struggles with a visually complex page — it's slower</Typography>
              <Typography variant="body2">★ Start with <strong>Careful (1 action)</strong> speed for new tasks; switch to Balanced once it works reliably</Typography>
              <Typography variant="body2">★ Use a numbered list in the prompt to capture multiple pages in a single run — each gets its own evidence record</Typography>
              <Typography variant="body2">★ You log in yourself in Step 1 — the agent reuses that session and never sees your password</Typography>
            </Stack>
          </Box>

        </Stack>
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 1.5 }}>
        <Button onClick={onClose} variant="contained">Got it</Button>
      </DialogActions>
    </Dialog>
  );
}
