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

import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { HistoryEntry } from "../../api/riskApi";
import { STATUS_CONFIG, formatDate } from "./utils";

// Tone drives the dot colour only — the sentence carries the meaning, so a
// missing tone degrades to neutral rather than hiding the entry.
type Tone = "created" | "approved" | "rejected" | "escalated" | "neutral";

const TONE_COLORS: Record<Tone, string> = {
  created: "#2563eb",
  approved: "#16a34a",
  rejected: "#dc2626",
  escalated: "#ea580c",
  neutral: "#94a3b8",
};

// statusLabel renders a workflow status the way the rest of the UI does, so the
// timeline and the status chips never disagree on wording.
function statusLabel(status?: string): string {
  if (!status) return "";
  return STATUS_CONFIG[status]?.label ?? status.replace(/_/g, " ").toLowerCase();
}

// fieldLabel turns a column name into the words the form uses.
const FIELD_LABELS: Record<string, string> = {
  risk_title: "title",
  risk_description: "description",
  impact_description: "impact description",
  implementation_date: "implementation date",
  reassessment_date: "reassessment date",
  treatment_strategy: "treatment strategy",
  email_subject: "email subject",
  git_issue_url: "Git issue URL",
  action_steps: "action steps",
  progress: "progress",
  remarks: "remarks",
};

function fieldLabel(field: string): string {
  return FIELD_LABELS[field] ?? field.replace(/_/g, " ");
}

// Values arrive as raw JSON strings (that is how they are stored), so unwrap a
// quoted scalar for display and fall back to the raw text if it isn't JSON.
function readValue(raw: string | null): string {
  if (!raw) return "";
  try {
    const v: unknown = JSON.parse(raw);
    return typeof v === "string" ? v : JSON.stringify(v);
  } catch {
    return raw;
  }
}

interface Rendered {
  title: string;
  body?: string;
  tone: Tone;
}

// entryToSentence maps an action plus its payload to one human line.
//
// The default case renders an unrecognised action generically rather than
// dropping it — the same choice the Audit Hub's ControlHistoryTimeline makes,
// so adding a new action server-side stays visible without a frontend release.
function entryToSentence(e: HistoryEntry): Rendered {
  const d = e.details ?? {};
  const transition =
    d.from && d.to ? `${statusLabel(d.from)} → ${statusLabel(d.to)}` : d.to ? statusLabel(d.to) : undefined;

  switch (e.action) {
    case "CREATE":
      return d.plan
        ? { title: `Action plan added — ${d.plan}`, tone: "created" }
        : { title: "Risk created", tone: "created" };
    case "SUBMIT":
      return { title: "Submitted for approval", body: transition, tone: "neutral" };
    case "APPROVE":
      return { title: `Approved by ${d.role ?? "approver"}`, body: transition, tone: "approved" };
    case "REJECT":
      return {
        title: `Rejected${d.stage ? ` at ${d.stage.replace(/_/g, " ").toLowerCase()}` : ""}`,
        body: [transition, d.comment].filter(Boolean).join(" · "),
        tone: "rejected",
      };
    case "ESCALATE":
      return {
        title: d.overdueDays ? `Escalated — overdue by ${d.overdueDays} days` : "Escalated — remediation overdue",
        body: transition,
        tone: "escalated",
      };
    case "COMMENT":
      return { title: "Escalation reviewed", body: d.comment, tone: "escalated" };
    case "ASSESS":
      return {
        title: d.previousLevel && d.level ? `Reassessed ${d.previousLevel} → ${d.level}` : `Reassessed${d.level ? ` — ${d.level}` : ""}`,
        tone: "neutral",
      };
    case "COMPLETE":
      return d.plan
        ? { title: `Action plan completed — ${d.plan}`, tone: "approved" }
        : { title: "Submitted for completion approval", body: transition, tone: "neutral" };
    case "CLOSE":
      return { title: "Risk closed", body: transition, tone: "approved" };
    case "CANCEL":
      return { title: "Risk cancelled", tone: "neutral" };
    case "UPDATE": {
      if (!e.field_changed) return { title: "Risk updated", tone: "neutral" };
      const from = readValue(e.old_value);
      const to = readValue(e.new_value);
      return {
        title: `Changed ${fieldLabel(e.field_changed)}`,
        // action_steps records no before/after — the steps are rows, not a
        // scalar — so it renders as a bare "changed" line.
        body: from || to ? `${from || "—"} → ${to || "—"}` : undefined,
        tone: "neutral",
      };
    }
    default:
      return { title: e.action.replace(/_/g, " ").toLowerCase(), tone: "neutral" };
  }
}

export default function RiskHistoryTimeline({ entries }: { entries: HistoryEntry[] }): JSX.Element {
  return (
    <Stack>
      {entries.map((e, i) => {
        const { title, body, tone } = entryToSentence(e);
        const last = i === entries.length - 1;
        return (
          <Box key={e.id} sx={{ display: "flex", gap: 1.5 }}>
            {/* Dot + connecting rail. The rail is omitted on the last entry so
                the timeline ends cleanly rather than trailing into nothing. */}
            <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", pt: 0.6 }}>
              <Box
                sx={{
                  width: 10,
                  height: 10,
                  borderRadius: "50%",
                  bgcolor: TONE_COLORS[tone],
                  flexShrink: 0,
                }}
              />
              {!last && <Box sx={{ width: "2px", flex: 1, bgcolor: "divider", mt: 0.5 }} />}
            </Box>

            <Box sx={{ pb: last ? 0 : 2, minWidth: 0, flex: 1 }}>
              <Typography variant="body2" fontWeight={600}>
                {title}
              </Typography>
              {body && (
                <Typography variant="body2" color="text.secondary" sx={{ wordBreak: "break-word" }}>
                  {body}
                </Typography>
              )}
              <Typography variant="caption" color="text.secondary">
                {formatDate(e.created_at)} · {e.created_by}
              </Typography>
            </Box>
          </Box>
        );
      })}
    </Stack>
  );
}
