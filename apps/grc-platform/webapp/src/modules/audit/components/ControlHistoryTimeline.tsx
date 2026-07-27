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

import { Alert, Box, Chip, Skeleton, Typography } from "@wso2/oxygen-ui";
import {
  ArrowRight,
  Bot,
  CheckCircle2,
  FilePlus2,
  History,
  MessageSquare,
  RotateCcw,
  Upload,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { useMemo, type JSX } from "react";
import { useQueries } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { useGetTrail, type TrailEntry, type TrailDetails } from "@modules/audit/api/useGetTrail";
import { useGetEvidence } from "@modules/audit/api/useGetEvidence";
import { commentsQueryKey, type AuditComment } from "@modules/audit/api/useComments";
import { aiValidationQueryKey, type AIValidationLog } from "@modules/audit/api/useGetAIValidation";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import { formatTimestamp, relativeTime } from "@modules/audit/utils/format";
import type { ControlStatus } from "@modules/audit/types/audit";

// ─── Event model ──────────────────────────────────────────────────────────────

type EventTone = "created" | "uploaded" | "resubmitted" | "approved" | "rejected" | "comment" | "ai";

interface TimelineEvent {
  id: string;
  tone: EventTone;
  at: string; // ISO
  actor: string;
  title: string;
  from?: ControlStatus;
  to?: ControlStatus;
  body?: string;
  badge?: { label: string; color: string };
}

// Tone → icon + colour. Colours mirror the status palette in controlStatus.ts so
// the timeline reads as one system with the status chips.
const TONE: Record<EventTone, { color: string; icon: JSX.Element }> = {
  created:     { color: "#94A3B8", icon: <FilePlus2 size={15} /> },
  uploaded:    { color: "#6366F1", icon: <Upload size={15} /> },
  resubmitted: { color: "#F59E0B", icon: <RotateCcw size={15} /> },
  approved:    { color: "#10B981", icon: <CheckCircle2 size={15} /> },
  rejected:    { color: "#EF4444", icon: <XCircle size={15} /> },
  comment:     { color: "#0EA5E9", icon: <MessageSquare size={15} /> },
  ai:          { color: "#8B5CF6", icon: <Bot size={15} /> },
};

// ─── Detail helpers ───────────────────────────────────────────────────────────

/** Normalizes the trail `details` field, which may arrive as an object or a
 *  JSON string depending on how the store round-trips the JSON column. */
function readDetails(d: TrailDetails | null | undefined): TrailDetails {
  if (!d) return {};
  if (typeof d === "string") {
    try {
      const parsed = JSON.parse(d) as unknown;
      return typeof parsed === "object" && parsed !== null ? (parsed as TrailDetails) : {};
    } catch {
      return {};
    }
  }
  return d;
}

/** Returns the status only when it's a known control status (else undefined), so
 *  we never hand ControlStatusChip an unmapped key. */
function asStatus(v: unknown): ControlStatus | undefined {
  return typeof v === "string" && v in CONTROL_STATUS_LABELS ? (v as ControlStatus) : undefined;
}

const VIA_LABELS: Record<string, string> = {
  "web-app": "Web App",
  "evidence-app": "Evidence Portal",
};

// ─── Trail → event mapping ────────────────────────────────────────────────────

function trailToEvent(e: TrailEntry): TimelineEvent | null {
  const d = readDetails(e.details);
  const from = asStatus(d.from);
  const to = asStatus(d.to);
  const viaNote = typeof d.via === "string" && VIA_LABELS[d.via] ? `via ${VIA_LABELS[d.via]}` : undefined;
  const base = { id: `t-${e.id}`, at: e.createdAt, actor: e.createdBy };

  switch (e.action) {
    case "CREATED":
      return { ...base, tone: "created", title: "Control created" };
    case "UPLOADED":
      return { ...base, tone: "uploaded", title: "Evidence submitted for internal review", body: viaNote };
    case "RESUBMITTED":
      return { ...base, tone: "resubmitted", title: "Evidence resubmitted", body: viaNote };
    case "APPROVED": {
      const title =
        to === "COMPLETE"
          ? "Approved by auditor"
          : to?.endsWith("UNDER_VALIDATION")
            ? "Passed internal review"
            : to?.endsWith("SAMPLE") || to === "SUBMITTED_SAMPLE"
              ? "Sample selected"
              : "Advanced";
      return { ...base, tone: "approved", title, from, to, body: readComment(d) };
    }
    case "REJECTED": {
      const title = to?.endsWith("NEED_CLARIFICATION")
        ? "Auditor requested clarification"
        : "Sent back for changes";
      return { ...base, tone: "rejected", title, from, to, body: readComment(d) };
    }
    case "COMMENTED":
      return { ...base, tone: "comment", title: "Comment added", body: readComment(d) };
    case "AI_VALIDATED":
      return { ...base, tone: "ai", title: "AI validation completed", body: readComment(d) };
    default:
      // ESCALATED / EXPORTED etc. — show generically rather than dropping.
      return { ...base, tone: "created", title: e.action.toLowerCase().replace(/_/g, " ") };
  }
}

function readComment(d: TrailDetails): string | undefined {
  return typeof d.comment === "string" && d.comment.trim() !== "" ? d.comment : undefined;
}

// ─── Comment / AI → event mapping ─────────────────────────────────────────────

function commentToEvent(c: AuditComment): TimelineEvent {
  return {
    id: `c-${c.id}`,
    tone: "comment",
    at: c.createdAt,
    actor: c.createdBy,
    title: c.isInternal ? "Internal note" : "Comment",
    body: c.content,
    badge: c.isInternal ? { label: "Internal", color: "#b45309" } : undefined,
  };
}

const AI_TITLE: Record<AIValidationLog["result"], string | null> = {
  PASS: "AI validation passed",
  FAIL: "AI validation flagged gaps",
  UNCERTAIN: "AI validation inconclusive",
  ERROR: "AI validation could not complete",
  PENDING: null, // in-progress, not a historical event
};

const AI_BADGE: Partial<Record<AIValidationLog["result"], { label: string; color: string }>> = {
  PASS: { label: "Pass", color: "#10B981" },
  FAIL: { label: "Gaps", color: "#EF4444" },
  UNCERTAIN: { label: "Uncertain", color: "#F59E0B" },
};

function aiToEvent(a: AIValidationLog): TimelineEvent | null {
  const title = AI_TITLE[a.result];
  if (!title) return null;
  return {
    id: `a-${a.id}`,
    tone: "ai",
    at: a.createdOn,
    actor: "AI reviewer",
    title,
    body: a.summary ?? undefined,
    badge: AI_BADGE[a.result],
  };
}

// ─── Component ────────────────────────────────────────────────────────────────

export default function ControlHistoryTimeline({
  auditId,
  controlId,
  currentStatus,
}: {
  auditId: number;
  controlId: number;
  currentStatus: ControlStatus;
}): JSX.Element {
  const authFetch = useAuthApiClient();
  const trail = useGetTrail(auditId, controlId, true);
  // Evidence rounds give us the evidence ids to pull per-round comments + AI runs,
  // which are woven into the same timeline as the lifecycle events.
  const evidence = useGetEvidence(auditId, controlId, true);
  const evidenceIds = useMemo(() => (evidence.data ?? []).map((e) => e.id), [evidence.data]);

  const commentResults = useQueries({
    queries: evidenceIds.map((id) => ({
      queryKey: commentsQueryKey(id),
      queryFn: async (): Promise<AuditComment[]> => {
        const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/evidence/${id}/comments`);
        if (!res.ok) throw new Error(String(res.status));
        return ((await res.json()) as { items?: AuditComment[] }).items ?? [];
      },
    })),
  });

  const aiResults = useQueries({
    queries: evidenceIds.map((id) => ({
      queryKey: aiValidationQueryKey(id),
      queryFn: async (): Promise<AIValidationLog[]> => {
        const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/evidence/${id}/ai-validations`);
        if (!res.ok) throw new Error(String(res.status));
        return ((await res.json()) as { validations?: AIValidationLog[] }).validations ?? [];
      },
    })),
  });

  const events = useMemo<TimelineEvent[]>(() => {
    const out: TimelineEvent[] = [];
    for (const e of trail.data ?? []) {
      const ev = trailToEvent(e);
      if (ev) out.push(ev);
    }
    for (const r of commentResults) {
      for (const c of r.data ?? []) out.push(commentToEvent(c));
    }
    for (const r of aiResults) {
      for (const a of r.data ?? []) {
        const ev = aiToEvent(a);
        if (ev) out.push(ev);
      }
    }
    // Oldest first: the tab reads as the control's journey from creation onward.
    return out.sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime());
  }, [trail.data, commentResults, aiResults]);

  if (trail.isLoading) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        {[0, 1, 2].map((i) => (
          <Box key={i} sx={{ display: "flex", gap: 1.5 }}>
            <Skeleton variant="circular" width={30} height={30} />
            <Skeleton variant="rounded" height={48} sx={{ flex: 1 }} />
          </Box>
        ))}
      </Box>
    );
  }

  if (trail.isError) {
    return <Alert severity="error" sx={{ fontSize: "0.8rem" }}>Couldn’t load this control’s history.</Alert>;
  }

  if (events.length === 0) {
    return (
      <Box sx={{ py: 6, display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center", gap: 1.25 }}>
        <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "action.hover", display: "flex", alignItems: "center", justifyContent: "center", color: "text.secondary" }}>
          <History size={24} />
        </Box>
        <Typography variant="subtitle2" fontWeight={700}>No history yet</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 320, lineHeight: 1.6 }}>
          Events appear here as the control moves through submission, internal review, and auditor validation.
        </Typography>
      </Box>
    );
  }

  const firstAt = events[0].at;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {/* Journey summary */}
      <Box
        sx={{
          p: 2,
          borderRadius: 2,
          border: "1px solid",
          borderColor: "divider",
          bgcolor: "action.hover",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
          gap: 1.5,
        }}
      >
        <Box>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block" }}>
            {events.length} event{events.length === 1 ? "" : "s"} · started {formatTimestamp(firstAt)}
          </Typography>
          <Typography variant="body2" sx={{ mt: 0.25 }}>Current status</Typography>
        </Box>
        <ControlStatusChip status={currentStatus} size="medium" />
      </Box>

      {/* Timeline */}
      <Box sx={{ position: "relative", pl: 0.5 }}>
        {/* connecting rail behind the nodes */}
        <Box
          sx={{
            position: "absolute",
            left: 19,
            top: 14,
            bottom: 14,
            width: "2px",
            bgcolor: "divider",
          }}
        />
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2.25 }}>
          {events.map((ev) => (
            <TimelineRow key={ev.id} event={ev} />
          ))}
        </Box>
      </Box>
    </Box>
  );
}

// ─── Row ──────────────────────────────────────────────────────────────────────

function TimelineRow({ event }: { event: TimelineEvent }): JSX.Element {
  const tone = TONE[event.tone];
  return (
    <Box sx={{ display: "flex", gap: 1.75, position: "relative" }}>
      {/* node */}
      <Box
        sx={{
          width: 30,
          height: 30,
          borderRadius: "50%",
          flexShrink: 0,
          zIndex: 1,
          bgcolor: (theme) => (theme.palette.mode === "dark" ? "background.paper" : "#fff"),
          border: "2px solid",
          borderColor: tone.color,
          color: tone.color,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {tone.icon}
      </Box>

      {/* content */}
      <Box sx={{ flex: 1, minWidth: 0, pt: 0.25 }}>
        <Box sx={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 1 }}>
          <Typography variant="body2" fontWeight={600} sx={{ lineHeight: 1.4 }}>
            {event.title}
          </Typography>
          <Typography
            variant="caption"
            color="text.secondary"
            title={formatTimestamp(event.at)}
            sx={{ flexShrink: 0, whiteSpace: "nowrap" }}
          >
            {relativeTime(event.at)}
          </Typography>
        </Box>

        {/* status transition chips */}
        {event.from && event.to && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, mt: 0.75, flexWrap: "wrap" }}>
            <ControlStatusChip status={event.from} size="small" />
            <ArrowRight size={13} style={{ opacity: 0.5, flexShrink: 0 }} />
            <ControlStatusChip status={event.to} size="small" />
          </Box>
        )}

        {event.badge && (
          <Chip
            label={event.badge.label}
            size="small"
            variant="outlined"
            sx={{ mt: 0.75, height: 20, color: event.badge.color, borderColor: event.badge.color, "& .MuiChip-label": { px: 1, fontSize: "0.68rem", fontWeight: 600 } }}
          />
        )}

        {event.body && (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{
              mt: 0.75,
              lineHeight: 1.6,
              p: event.tone === "comment" || event.tone === "ai" ? 1.25 : 0,
              borderRadius: 1.25,
              bgcolor: event.tone === "comment" || event.tone === "ai" ? "action.hover" : "transparent",
            }}
          >
            {event.body}
          </Typography>
        )}

        <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5, display: "block" }}>
          {event.actor}
        </Typography>
      </Box>
    </Box>
  );
}
