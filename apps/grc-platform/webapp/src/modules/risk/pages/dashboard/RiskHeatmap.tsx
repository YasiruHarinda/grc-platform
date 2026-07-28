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

import { Box, Tooltip, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { HeatmapCell, RiskScore } from "../../api/riskApi";
import { LEVEL_LABELS } from "./constants";

interface RiskHeatmapProps {
  cells: HeatmapCell[];
  // The full 3×3 risk_score matrix, used to color cells that hold no risks.
  scores: RiskScore[];
}

const AXIS_VALUES = [1, 2, 3];
const ROW_LABEL_WIDTH = 22;
const LIKELIHOOD_TITLE_WIDTH = 20;

// Weighted-average likelihood/impact across a heatmap's open risks — the
// "center of mass" of the risk posture, drawn as the overall-residual-risk
// marker. Unlike the per-cell counts this is a continuous point (e.g.
// likelihood 1.9), so it can sit between cells rather than snapping to one.
function overallResidualRisk(cells: HeatmapCell[]): { likelihood: number; impact: number } | null {
  const totalCount = cells.reduce((sum, c) => sum + c.count, 0);
  if (totalCount === 0) return null;
  const likelihood = cells.reduce((sum, c) => sum + c.likelihood * c.count, 0) / totalCount;
  const impact = cells.reduce((sum, c) => sum + c.impact * c.count, 0) / totalCount;
  return { likelihood, impact };
}

// Converts an axis value in [1,3] to a 0–100% position within the 3-cell grid,
// matching each cell's center-to-center spacing (cell 1 center at 1/6, cell 3
// center at 5/6, and so on).
function axisValueToPercent(value: number): number {
  return (((value - 1) / 2) * (2 / 3) + 1 / 6) * 100;
}

// Custom 3×3 likelihood × impact heatmap (residual risk). Likelihood increases
// upward, impact rightward; each cell shows its open-risk count tinted with
// the matrix cell's severity color. A white marker overlays the count-weighted
// centroid of all open risks in this heatmap — the "overall residual risk".
export default function RiskHeatmap({ cells, scores }: RiskHeatmapProps): JSX.Element {
  const countAt = new Map(cells.map((c) => [`${c.likelihood}-${c.impact}`, c.count]));
  const scoreAt = new Map(scores.map((s) => [`${s.likelihood}-${s.impact}`, s]));
  const overall = overallResidualRisk(cells);

  return (
    <Box sx={{ maxWidth: 640, mx: "auto" }}>
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

          <Box sx={{ position: "relative", flex: 1 }}>
            <Box
              sx={{
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
                  const levelName = score
                    ? (LEVEL_LABELS[score.risk_level] ?? score.risk_level)
                    : "";
                  const cellDescription = `Likelihood ${likelihood} × Impact ${impact}${levelName ? ` ; ${levelName}` : ""}: ${count} open risk${count === 1 ? "" : "s"}`;
                  return (
                    <Tooltip key={`${likelihood}-${impact}`} title={cellDescription}>
                      <Box
                        tabIndex={0}
                        role="img"
                        aria-label={cellDescription}
                        sx={{
                          aspectRatio: "1.3",
                          minHeight: 84,
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                          borderRadius: 1,
                          bgcolor: `${color}26`,
                          border: `2px solid ${color}`,
                        }}
                      >
                        <Typography
                          variant="h6"
                          fontWeight={700}
                          color={count === 0 ? "text.disabled" : "text.primary"}
                        >
                          {count}
                        </Typography>
                      </Box>
                    </Tooltip>
                  );
                }),
              )}
            </Box>

            {overall && (
              <Tooltip
                title={`Overall residual risk — Likelihood ${overall.likelihood.toFixed(1)}, Impact ${overall.impact.toFixed(1)}`}
              >
                <Box
                  tabIndex={0}
                  role="img"
                  aria-label={`Overall residual risk — Likelihood ${overall.likelihood.toFixed(1)}, Impact ${overall.impact.toFixed(1)}`}
                  sx={{
                    position: "absolute",
                    left: `${axisValueToPercent(overall.impact)}%`,
                    top: `${100 - axisValueToPercent(overall.likelihood)}%`,
                    transform: "translate(-50%, -50%)",
                    width: 14,
                    height: 14,
                    borderRadius: "50%",
                    bgcolor: "#ffffff",
                    border: "2px solid #1a1a19",
                    boxShadow: "0 1px 4px rgba(0,0,0,0.5)",
                    cursor: "default",
                  }}
                />
              </Tooltip>
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

      {overall && (
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 0.75, mt: 1 }}>
          <Box
            component="span"
            sx={{
              width: 10,
              height: 10,
              borderRadius: "50%",
              bgcolor: "#ffffff",
              border: "2px solid #1a1a19",
              boxShadow: "0 1px 3px rgba(0,0,0,0.4)",
              flexShrink: 0,
            }}
          />
          <Typography variant="caption" color="text.secondary">
            Overall residual risk
          </Typography>
        </Box>
      )}
    </Box>
  );
}
