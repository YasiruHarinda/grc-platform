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

import type { HeatmapCell, RegisterAnalytics } from "../../api/riskApi";

// Weighted mean of each open risk's own residual rating (likelihood × impact),
// i.e. the average "residual risk value" per the dashboard's stated
// methodology — not the product of separately-averaged axes.
export function meanRating(cells: HeatmapCell[]): number | null {
  const totalCount = cells.reduce((sum, c) => sum + c.count, 0);
  if (totalCount === 0) return null;
  const weighted = cells.reduce((sum, c) => sum + c.likelihood * c.impact * c.count, 0);
  return weighted / totalCount;
}

// Weighted-average likelihood/impact, each axis averaged independently. Used
// only to decide which single cell a register's pill belongs in (rounded);
// the displayed score itself always comes from meanRating.
export function meanPosition(cells: HeatmapCell[]): { likelihood: number; impact: number } | null {
  const totalCount = cells.reduce((sum, c) => sum + c.count, 0);
  if (totalCount === 0) return null;
  const likelihood = cells.reduce((sum, c) => sum + c.likelihood * c.count, 0) / totalCount;
  const impact = cells.reduce((sum, c) => sum + c.impact * c.count, 0) / totalCount;
  return { likelihood, impact };
}

export function clampRound(n: number): number {
  return Math.min(3, Math.max(1, Math.round(n)));
}

// Methodology sentence shown above the Average Residual Risk Score matrix.
// Every value (overall score, register count/names, valid-open-risk count)
// is derived live from the current payload, not hardcoded, so it stays
// correct as risks are added/closed and as registers are added over time.
// Register count/list is limited to registers that actually plot a pill
// (meanRating != null) — a register with only closed or unscored risks
// wouldn't appear on the matrix, so it's left out of this sentence too.
export function residualScoreMethodologySentence(
  registers: RegisterAnalytics[],
  orgHeatmap: HeatmapCell[],
): string | null {
  const overall = meanRating(orgHeatmap);
  if (overall == null) return null;

  const openScoredCount = orgHeatmap.reduce((sum, c) => sum + c.count, 0);
  const plottedNames = registers
    .filter((r) => meanRating(r.heatmap) != null)
    .map((r) => r.register_name);

  return (
    `The overall average residual risk score of ${overall.toFixed(2)} was calculated by taking the mean of ` +
    `all residual risk values for open risks only (excluding closed risks and unscored entries) across all ` +
    `${plottedNames.length} risk register${plottedNames.length === 1 ? "" : "s"} (${plottedNames.join(", ")}), ` +
    `covering ${openScoredCount} valid open risk${openScoredCount === 1 ? "" : "s"}. Each register is plotted ` +
    `on the matrix below based on its mean score.`
  );
}
