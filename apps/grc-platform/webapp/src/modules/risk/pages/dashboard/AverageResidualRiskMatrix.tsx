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

import { Box, Chip, Tooltip, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { HeatmapCell, RegisterAnalytics, RiskScore } from "../../api/riskApi";
import { LEVEL_LABELS } from "./constants";
import { clampRound, meanPosition, meanRating } from "./residualRiskMath";

// Cells can hold up to 8 registers' pills; cap how many render directly so
// the count badge (top-right) never gets crowded out, the grid keeps a
// uniform cell height instead of one cell growing much taller than the
// rest, and the whole matrix (and its dashboard row) stays compact.
const MAX_VISIBLE_PILLS = 2;

interface AverageResidualRiskMatrixProps {
  registers: RegisterAnalytics[];
  // Org-wide (all registers combined) heatmap — backs the per-cell risk-count
  // badge and the overall-average pill.
  orgHeatmap: HeatmapCell[];
  // The full 3×3 risk_score matrix, used to color cells and label ratings.
  scores: RiskScore[];
}

const AXIS_VALUES = [1, 2, 3];
const ROW_LABEL_WIDTH = 22;
const LIKELIHOOD_TITLE_WIDTH = 20;

interface RegisterPoint {
  name: string;
  rating: number;
}

// 3×3 likelihood × impact matrix comparing registers' average residual risk.
// Each cell shows the fixed risk rating (top-right), the org-wide open-risk
// count at that cell (top-left), and a pill per register whose own rounded
// average likelihood/impact lands there, labeled with its unrounded average
// rating. The all-registers overall average is shown separately, as a pill
// in the card header (see ChartCard's headerRight), not inside the grid.
export default function AverageResidualRiskMatrix({
  registers,
  orgHeatmap,
  scores,
}: AverageResidualRiskMatrixProps): JSX.Element {
  const scoreAt = new Map(scores.map((s) => [`${s.likelihood}-${s.impact}`, s]));
  const countAt = new Map(orgHeatmap.map((c) => [`${c.likelihood}-${c.impact}`, c.count]));

  const registersAt = new Map<string, RegisterPoint[]>();
  for (const register of registers) {
    const pos = meanPosition(register.heatmap);
    const rating = meanRating(register.heatmap);
    if (!pos || rating == null) continue;
    const key = `${clampRound(pos.likelihood)}-${clampRound(pos.impact)}`;
    const list = registersAt.get(key) ?? [];
    list.push({ name: register.register_name, rating });
    registersAt.set(key, list);
  }
  for (const list of registersAt.values()) {
    list.sort((a, b) => b.rating - a.rating);
  }

  return (
    <Box sx={{ maxWidth: 560, mx: "auto" }}>
      <Box sx={{ display: "flex", alignItems: "stretch", gap: 1 }}>
        <Box
          sx={{
            width: LIKELIHOOD_TITLE_WIDTH,
            flexShrink: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            writingMode: "vertical-rl",
            transform: "rotate(180deg)",
          }}
        >
          <Typography variant="caption" color="text.secondary" noWrap>
            Likelihood (Residual Risk)
          </Typography>
        </Box>

        <Box sx={{ flex: 1, display: "flex", gap: 1 }}>
          <Box
            sx={{
              width: ROW_LABEL_WIDTH,
              display: "flex",
              flexDirection: "column",
              gap: "2px",
            }}
          >
            {[...AXIS_VALUES].reverse().map((likelihood) => (
              <Box
                key={likelihood}
                sx={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "flex-end" }}
              >
                <Typography variant="caption" color="text.secondary">
                  {likelihood}
                </Typography>
              </Box>
            ))}
          </Box>

          <Box
            sx={{
              flex: 1,
              display: "grid",
              gridTemplateColumns: "repeat(3, 1fr)",
              gridTemplateRows: "repeat(3, 1fr)",
              gap: "2px",
            }}
          >
            {[...AXIS_VALUES].reverse().map((likelihood) =>
              AXIS_VALUES.map((impact) => {
                const score = scoreAt.get(`${likelihood}-${impact}`);
                const count = countAt.get(`${likelihood}-${impact}`) ?? 0;
                const color = score?.color_code ?? "#9e9e9e";
                const points = registersAt.get(`${likelihood}-${impact}`) ?? [];
                const rating = score?.risk_rating ?? likelihood * impact;
                const levelName = score ? (LEVEL_LABELS[score.risk_level] ?? score.risk_level) : "";
                const cellDescription = `Likelihood ${likelihood} × Impact ${impact} = ${rating}${levelName ? ` ; ${levelName}` : ""}: ${count} open risk${count === 1 ? "" : "s"}`;

                const pillSx = {
                  bgcolor: "rgba(255,255,255,0.92)",
                  color,
                  fontWeight: 700,
                  height: 20,
                  maxWidth: 96,
                  "& .MuiChip-label": {
                    fontSize: "0.65rem",
                    px: 0.75,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  },
                };

                return (
                  <Tooltip key={`${likelihood}-${impact}`} title={cellDescription}>
                    <Box
                      tabIndex={0}
                      role="img"
                      aria-label={cellDescription}
                      sx={{
                        position: "relative",
                        minHeight: 64,
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        justifyContent: "center",
                        gap: 0.5,
                        pt: 2.5,
                        pb: 0.75,
                        px: 0.75,
                        borderRadius: 1,
                        bgcolor: `${color}26`,
                        border: `2px solid ${color}`,
                      }}
                    >
                      <Typography
                        variant="subtitle2"
                        fontWeight={700}
                        sx={{ position: "absolute", top: 2, right: 6 }}
                        color="text.secondary"
                      >
                        {count}
                      </Typography>
                      {points.slice(0, MAX_VISIBLE_PILLS).map((p) => (
                        <Tooltip key={p.name} title={`${p.name}  ${p.rating.toFixed(2)}`}>
                          <Chip label={`${p.name}  ${p.rating.toFixed(2)}`} size="small" sx={pillSx} />
                        </Tooltip>
                      ))}
                      {points.length > MAX_VISIBLE_PILLS && (
                        <Tooltip
                          title={points
                            .slice(MAX_VISIBLE_PILLS)
                            .map((p) => `${p.name} ${p.rating.toFixed(2)}`)
                            .join(", ")}
                        >
                          <Chip
                            label={`+${points.length - MAX_VISIBLE_PILLS} more`}
                            size="small"
                            sx={{ ...pillSx, bgcolor: "rgba(255,255,255,0.75)" }}
                          />
                        </Tooltip>
                      )}
                    </Box>
                  </Tooltip>
                );
              }),
            )}
          </Box>
        </Box>
      </Box>

      <Box sx={{ display: "flex", gap: 1 }}>
        <Box sx={{ width: LIKELIHOOD_TITLE_WIDTH, flexShrink: 0 }} />
        <Box sx={{ flex: 1, display: "flex", gap: 1 }}>
          <Box sx={{ width: ROW_LABEL_WIDTH }} />
          <Box sx={{ flex: 1, display: "grid", gridTemplateColumns: "repeat(3, 1fr)", pt: 0.5 }}>
            {AXIS_VALUES.map((impact) => (
              <Box key={impact} sx={{ textAlign: "center" }}>
                <Typography variant="caption" color="text.secondary">
                  {impact}
                </Typography>
              </Box>
            ))}
          </Box>
        </Box>
      </Box>

      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ display: "block", textAlign: "center", mt: 0.5 }}
      >
        Impact (Residual Risk)
      </Typography>
    </Box>
  );
}
