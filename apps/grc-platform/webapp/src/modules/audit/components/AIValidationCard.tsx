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

import { Alert, Box, Button, Chip, Collapse, LinearProgress, Paper, Typography } from "@wso2/oxygen-ui";
import { Bot, ChevronDown, ChevronRight, Sparkles } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useGetEvidence } from "@modules/audit/api/useGetEvidence";
import {
  isFreshPending,
  parseFeedback,
  parseGaps,
  useGetAIValidation,
  type AIGap,
  type AIValidationLog,
} from "@modules/audit/api/useGetAIValidation";

const AI_PURPLE = "#7c3aed";
const AI_PURPLE_BG = "#faf5ff";

// Severity → dot colour for the gap list.
const SEVERITY_COLOR: Record<AIGap["severity"], string> = {
  HIGH: "#dc2626",
  MEDIUM: "#b45309",
  LOW: "#6b7280",
};

// Terminal verdict → chip styling + label.
const VERDICT_STYLE: Record<string, { label: string; color: string; bg: string; darkBg: string }> = {
  PASS:      { label: "AI: Looks Complete",      color: "#16a34a", bg: "#f0fdf4", darkBg: "#16a34a33" },
  FAIL:      { label: "AI: Gaps Found",          color: "#dc2626", bg: "#fee2e2", darkBg: "#dc262633" },
  UNCERTAIN: { label: "AI: Needs Human Review",  color: "#b45309", bg: "#fff7ed", darkBg: "#b4530933" },
};

const ADVISORY_SUBMITTER = "AI-generated hint — does not affect review status.";
const ADVISORY_REVIEWER = "Advisory only — your decision is authoritative.";

// Hidden across every call site (evidence + population, submitter + reviewer)
// until the validation agent is rebuilt. Flip back to true to restore — the
// component and its call sites are otherwise untouched.
const AI_VALIDATION_ENABLED = false;

interface AIValidationCardProps {
  auditId: number;
  controlId: number;
  variant: "submitter" | "reviewer";
  /**
   * Which submission this card validates. "population" has no AI agent wired
   * up on the backend yet — it renders a permanent placeholder instead of
   * fetching anything, so the surface exists ahead of the agent landing later
   * (swap it for real data the same way "evidence" already works, once the
   * population AI endpoint exists). Defaults to "evidence", which is fully
   * wired for both control types.
   */
  phase?: "evidence" | "population";
}

const PHASE_TITLE: Record<"evidence" | "population", string> = {
  evidence: "AI Validation",
  population: "Population AI Validation",
};

const POPULATION_PLACEHOLDER_TEXT =
  "AI review for population submissions isn't wired up yet — this is where it'll appear once it's ready, flagging gaps before internal review.";

/**
 * AIValidationCard renders the advisory AI pre-review for a control's latest
 * evidence submission (phase="evidence", the default), or a placeholder for
 * the population phase (phase="population") until that agent exists. It
 * resolves the latest evidence id itself (react-query dedupes the shared
 * evidence query) and polls only while a job is in progress. Advisory only —
 * it never gates the workflow.
 */
export default function AIValidationCard({ auditId, controlId, variant, phase = "evidence" }: AIValidationCardProps): JSX.Element | null {
  const { data: submissions } = useGetEvidence(auditId, controlId, phase === "evidence" && AI_VALIDATION_ENABLED);
  const latestEvidenceId = phase === "evidence" ? (submissions?.[0]?.id ?? null) : null;
  const { data: validations, isLoading } = useGetAIValidation(AI_VALIDATION_ENABLED ? latestEvidenceId : null);

  if (!AI_VALIDATION_ENABLED) return null;

  const latest = phase === "evidence" ? validations?.[0] : undefined;

  // Population has nothing to show for the reviewer yet — only the
  // submitter-facing placeholder exists until the agent is wired up.
  if (phase === "population" && variant === "reviewer") {
    return null;
  }

  // Reviewer variant stays out of the way until there is something to show.
  if (variant === "reviewer" && (latestEvidenceId === null || !latest)) {
    return null;
  }

  const body = phase === "population"
    ? <NotValidated text={POPULATION_PLACEHOLDER_TEXT} />
    : renderBody(latest, latestEvidenceId, isLoading, variant);

  if (variant === "reviewer") {
    return (
      <Paper variant="outlined" sx={{ borderRadius: 2, p: 1.75, borderColor: "divider", bgcolor: AI_PURPLE_BG, "[data-color-scheme='dark'] &": { bgcolor: `${AI_PURPLE}33` } }}>
        {body}
      </Paper>
    );
  }

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <Box
        sx={{
          px: 2.5,
          py: 1.5,
          display: "flex",
          alignItems: "center",
          gap: 1.25,
          borderBottom: 1,
          borderColor: "divider",
          bgcolor: "action.hover",
        }}
      >
        <Box
          sx={{
            width: 30,
            height: 30,
            borderRadius: 1.5,
            color: "text.secondary",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          <Sparkles size={16} />
        </Box>
        <Typography variant="subtitle2" fontWeight={700} sx={{ flex: 1 }}>
          {PHASE_TITLE[phase]}
        </Typography>
        {latest && VERDICT_STYLE[latest.result] && (
          <Chip
            size="small"
            label={VERDICT_STYLE[latest.result].label}
            sx={{ bgcolor: VERDICT_STYLE[latest.result].bg, "[data-color-scheme='dark'] &": { bgcolor: VERDICT_STYLE[latest.result].darkBg }, color: VERDICT_STYLE[latest.result].color, fontWeight: 600 }}
          />
        )}
      </Box>
      <Box sx={{ p: 2.5 }}>{body}</Box>
    </Paper>
  );
}

// renderBody picks the visual for the current state.
function renderBody(
  latest: AIValidationLog | undefined,
  evidenceId: number | null,
  isLoading: boolean,
  variant: "submitter" | "reviewer",
): JSX.Element {
  // No submission yet, or no rows: nothing has run.
  if (evidenceId === null || (!latest && !isLoading)) {
    return <NotValidated />;
  }
  if (!latest) {
    return <MutedRow text="Loading AI review…" color="#9ca3af" />;
  }

  // Fresh PENDING: a job is genuinely in progress.
  if (isFreshPending(latest)) {
    return (
      <Box>
        <LinearProgress
          sx={{
            mb: 1.25,
            borderRadius: 1,
            "& .MuiLinearProgress-bar": { bgcolor: AI_PURPLE },
            bgcolor: AI_PURPLE_BG,
            "[data-color-scheme='dark'] &": { bgcolor: `${AI_PURPLE}33` },
          }}
        />
        <Typography variant="body2" color="text.secondary">
          Analyzing evidence…
        </Typography>
      </Box>
    );
  }

  // ERROR, or a PENDING row that never resolved (stale): unavailable.
  if (latest.result === "ERROR" || latest.result === "PENDING") {
    return (
      <Alert severity="warning" sx={{ py: 0.5 }}>
        AI validation unavailable — proceed as usual.
      </Alert>
    );
  }

  // Terminal verdict.
  return variant === "reviewer" ? <ReviewerVerdict latest={latest} /> : <SubmitterVerdict latest={latest} />;
}

function NotValidated({
  text = "AI review runs automatically after you submit evidence, and flags any gaps against the requirement.",
}: {
  text?: string;
}): JSX.Element {
  return (
    <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center", gap: 1.5, py: 0.5 }}>
      <Box
        sx={{
          width: 48,
          height: 48,
          borderRadius: "50%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: "text.secondary",
        }}
      >
        <Bot size={24} />
      </Box>
      <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.65 }}>
        {text}
      </Typography>
      <Button
        variant="outlined"
        fullWidth
        disabled
        startIcon={<Sparkles size={15} />}
        sx={{ textTransform: "none", fontWeight: 600 }}
      >
        Run AI Validation
      </Button>
    </Box>
  );
}

function MutedRow({ text, color }: { text: string; color: string }): JSX.Element {
  return (
    <Box sx={{ py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "action.hover", display: "flex", alignItems: "center", gap: 1 }}>
      <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: color, flexShrink: 0 }} />
      <Typography variant="body2" color="text.secondary">
        {text}
      </Typography>
    </Box>
  );
}

function Confidence({ score }: { score: number | null }): JSX.Element | null {
  if (score === null || score === undefined) return null;
  return (
    <Typography variant="caption" color="text.secondary">
      Confidence: {Math.round(score * 100)}%
    </Typography>
  );
}

function GapList({ gaps }: { gaps: AIGap[] }): JSX.Element {
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {gaps.map((g, i) => (
        <Box key={i} sx={{ display: "flex", gap: 1 }}>
          <Box sx={{ mt: 0.65, width: 8, height: 8, borderRadius: "50%", bgcolor: SEVERITY_COLOR[g.severity] ?? "#6b7280", flexShrink: 0 }} />
          <Box>
            <Typography variant="body2" sx={{ fontWeight: 600, lineHeight: 1.5 }}>
              {g.severity} · {g.requirementAspect}
              {g.fileName ? (
                <Typography component="span" variant="caption" color="text.secondary">
                  {" "}
                  ({g.fileName})
                </Typography>
              ) : null}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.55 }}>
              {g.issue}
            </Typography>
          </Box>
        </Box>
      ))}
    </Box>
  );
}

function SubmitterVerdict({ latest }: { latest: AIValidationLog }): JSX.Element {
  const gaps = parseGaps(latest.gapsFound);
  const feedback = parseFeedback(latest.feedback);
  const [showGaps, setShowGaps] = useState(true);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      {latest.summary && (
        <Typography variant="body2" sx={{ lineHeight: 1.7 }}>
          {latest.summary}
        </Typography>
      )}
      <Confidence score={latest.confidenceScore} />

      {gaps.length > 0 && (
        <Box>
          <Box
            component="button"
            onClick={() => setShowGaps((v) => !v)}
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 0.5,
              border: "none",
              background: "none",
              cursor: "pointer",
              p: 0,
              color: "text.primary",
            }}
          >
            {showGaps ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
            <Typography variant="body2" fontWeight={600}>
              {gaps.length} {gaps.length === 1 ? "gap" : "gaps"} found
            </Typography>
          </Box>
          <Collapse in={showGaps}>
            <Box sx={{ mt: 1 }}>
              <GapList gaps={gaps} />
            </Box>
          </Collapse>
        </Box>
      )}

      {feedback.length > 0 && (
        <Box>
          <Typography variant="body2" fontWeight={600} sx={{ mb: 0.75 }}>
            Suggested fixes before review:
          </Typography>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
            {feedback.map((f, i) => (
              <Box key={i} sx={{ display: "flex", gap: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.6 }}>
                  ☐ {f}
                </Typography>
              </Box>
            ))}
          </Box>
        </Box>
      )}

      <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
        ⓘ {ADVISORY_SUBMITTER}
      </Typography>
    </Box>
  );
}

function ReviewerVerdict({ latest }: { latest: AIValidationLog }): JSX.Element {
  const gaps = parseGaps(latest.gapsFound);
  const [showDetails, setShowDetails] = useState(false);
  const style = VERDICT_STYLE[latest.result];

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        {style && (
          <Chip size="small" label={style.label} sx={{ bgcolor: style.bg, "[data-color-scheme='dark'] &": { bgcolor: style.darkBg }, color: style.color, fontWeight: 600 }} />
        )}
        {latest.confidenceScore !== null && (
          <Typography variant="caption" color="text.secondary">
            {Math.round(latest.confidenceScore * 100)}% confidence
          </Typography>
        )}
        {gaps.length > 0 && (
          <>
            <Typography variant="caption" color="text.secondary">
              · {gaps.length} {gaps.length === 1 ? "gap" : "gaps"}
            </Typography>
            <Box
              component="button"
              onClick={() => setShowDetails((v) => !v)}
              sx={{ display: "inline-flex", alignItems: "center", gap: 0.25, border: "none", background: "none", cursor: "pointer", p: 0, color: AI_PURPLE }}
            >
              {showDetails ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
              <Typography variant="caption" sx={{ color: AI_PURPLE, fontWeight: 600 }}>
                details
              </Typography>
            </Box>
          </>
        )}
      </Box>

      {latest.summary && (
        <Typography variant="body2" sx={{ lineHeight: 1.6 }}>
          {latest.summary}
        </Typography>
      )}

      {gaps.length > 0 && (
        <Collapse in={showDetails}>
          <Box sx={{ mt: 0.5 }}>
            <GapList gaps={gaps} />
          </Box>
        </Collapse>
      )}

      <Typography variant="caption" color="text.secondary">
        ⓘ {ADVISORY_REVIEWER}
      </Typography>
    </Box>
  );
}
