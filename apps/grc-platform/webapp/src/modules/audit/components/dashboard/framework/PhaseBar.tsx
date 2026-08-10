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
import { DUE_SOON } from "@modules/audit/components/dashboard/dueDate";
import { PHASE_LABELS } from "@modules/audit/utils/controlStatus";

interface PhaseBarProps {
  counts: PhaseCounts;
  /** primary.main, or success.main once complete — decided by the caller. */
  completeColor: string;
  /** @default 8 */
  height?: number;
}

// Shared by the rail card and the team breakdown (§3.1/§3.3). No PHASE_COLORS
// (§8.1): complete takes the progress colour, needs-clarification takes the
// due-soon accent, and in-progress / not-started share a neutral grey — width
// alone tells the four segments apart, no legend needed.
export default function PhaseBar({ counts, completeColor, height = 8 }: PhaseBarProps): JSX.Element {
  const { complete, inProgress, needsClarification, notStarted } = counts;
  const total = complete + inProgress + needsClarification + notStarted;
  const pct = (n: number) => (total > 0 ? (n / total) * 100 : 0);

  const segments: { key: string; label: string; count: number; width: number; color: string; darkColor?: string }[] = [
    { key: "complete", label: PHASE_LABELS.COMPLETE, count: complete, width: pct(complete), color: completeColor },
    { key: "inProgress", label: PHASE_LABELS.IN_PROGRESS, count: inProgress, width: pct(inProgress), color: "rgba(0,0,0,0.24)", darkColor: "rgba(255,255,255,0.32)" },
    { key: "needsClarification", label: PHASE_LABELS.BLOCKED, count: needsClarification, width: pct(needsClarification), color: DUE_SOON },
    { key: "notStarted", label: PHASE_LABELS.NOT_STARTED, count: notStarted, width: pct(notStarted), color: "#E0E0E0" },
  ];
  // Width/color alone don't identify a segment to a screen reader (or to a
  // sighted user who can't distinguish the two greys) — role="img" collapses
  // the bar to one accessible element with this summary as its name; each
  // segment's native title gives the same name/count on hover.
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
            ...(s.darkColor && { "[data-color-scheme='dark'] &": { bgcolor: s.darkColor } }),
          }}
        />
      ))}
    </Box>
  );
}
