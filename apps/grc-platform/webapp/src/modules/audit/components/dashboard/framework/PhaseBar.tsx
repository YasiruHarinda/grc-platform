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

import { Box } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { PhaseCounts } from "@modules/audit/types/framework";
import { PHASE_COLORS, PHASE_LABELS, PHASE_ORDER, type ControlPhase } from "@modules/audit/utils/controlStatus";

interface PhaseBarProps {
  counts: PhaseCounts;
  /** primary.main, or success.main once complete — decided by the caller. */
  completeColor: string;
  /** @default 8 */
  height?: number;
}

const PHASE_COUNT_KEY: Record<ControlPhase, keyof PhaseCounts> = {
  NOT_STARTED: "notStarted",
  IN_PROGRESS: "inProgress",
  BLOCKED: "needsClarification",
  COMPLETE: "complete",
};

// Shared by the rail card and the team breakdown. Segment order
// and color follow the same PHASE_ORDER/PHASE_COLORS contract PhaseDonut uses,
// so a phase reads as the same color everywhere on the dashboard. completeColor
// is the one deliberate override: callers pass primary.main while an audit is
// still in progress and success.main once it's fully complete — a dynamic
// progress signal that a static PHASE_COLORS.COMPLETE can't express.
export default function PhaseBar({ counts, completeColor, height = 8 }: PhaseBarProps): JSX.Element {
  const total = counts.complete + counts.inProgress + counts.needsClarification + counts.notStarted;
  const pct = (n: number) => (total > 0 ? (n / total) * 100 : 0);

  const segments = PHASE_ORDER.map((phase) => {
    const count = counts[PHASE_COUNT_KEY[phase]];
    return {
      key: phase,
      label: PHASE_LABELS[phase],
      count,
      width: pct(count),
      color: phase === "COMPLETE" ? completeColor : PHASE_COLORS[phase],
    };
  });
  // Width/color alone don't identify a segment to a screen reader — role="img"
  // collapses the bar to one accessible element with this summary as its name;
  // each segment's native title gives the same name/count on hover.
  const summary = segments.map((s) => `${s.count} ${s.label}`).join(", ");

  return (
    <Box
      role="img"
      aria-label={`Phase breakdown: ${summary}`}
      sx={{
        display: "flex",
        width: "100%",
        height,
        borderRadius: height / 2,
        overflow: "hidden",
        bgcolor: "#E0E0E0",
        "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
      }}
    >
      {segments.map((s) => s.width > 0 && (
        <Box
          key={s.key}
          title={`${s.label}: ${s.count}`}
          sx={{
            width: `${s.width}%`,
            bgcolor: s.color,
          }}
        />
      ))}
    </Box>
  );
}
