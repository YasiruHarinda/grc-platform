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

import { BarChart } from "@wso2/oxygen-ui-charts-react";
import { Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RiskLevelCount } from "../../api/riskApi";
import { LEVEL_FALLBACK_COLORS, LEVEL_LABELS, LEVEL_ORDER } from "./constants";

interface LevelCountChartProps {
  data: RiskLevelCount[];
}

// "Count vs. Risk Level" — one bar per residual level in its severity color.
// Each level is its own series sharing a stack so every category renders a
// single centered bar; the axis names the level, so the legend is hidden.
export default function LevelCountChart({ data }: LevelCountChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open risks.
      </Typography>
    );
  }

  const byLevel = new Map(data.map((d) => [d.risk_level, d]));
  const levels = LEVEL_ORDER.filter((level) => byLevel.has(level));
  const rows = levels.map((level) => ({
    level: LEVEL_LABELS[level],
    [level]: byLevel.get(level)!.count,
  }));

  const bars = levels.map((level) => ({
    dataKey: level,
    name: LEVEL_LABELS[level],
    fill: byLevel.get(level)!.color_code || LEVEL_FALLBACK_COLORS[level],
    stackId: "level",
    label: { position: "top", fontSize: 12, fill: "#888888" },
  }));

  return (
    <BarChart
      data={rows}
      xAxisDataKey="level"
      bars={bars}
      height={420}
      maxBarSize={64}
      legend={{ show: false }}
      isAnimationActive={false}
      margin={{ top: 8, right: 16, left: 0, bottom: 0 }}
    />
  );
}
