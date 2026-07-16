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

import { BarChart, CartesianGrid, YAxis } from "@wso2/oxygen-ui-charts-react";
import { Typography, useColorScheme } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { WorkflowStageCount } from "../../api/riskApi";
import { WORKFLOW_FUNNEL_ORDER, WORKFLOW_STAGE_COLOR, WORKFLOW_STATUS_LABELS } from "./constants";

interface WorkflowFunnelChartProps {
  data: WorkflowStageCount[];
}

const ROW_HEIGHT = 40;
const MIN_HEIGHT = 240;
// Wide enough to fit the longest stage label ("Pending Management Approval")
// at 11px without truncating.
const LABEL_AXIS_WIDTH = 200;

// "Workflow Status Funnel": risk count per pipeline stage — surfaces where
// risks are piling up (e.g. stuck pending an approval), a bottleneck view
// nothing else on Dashboard or Analytics shows.
//
// Horizontal bars (layout="vertical" in recharts' naming) so long stage
// names read left-to-right on their own row instead of being squeezed
// under vertical bars, which clipped/overlapped for names like "Pending
// Management Approval". This also reads naturally as a top-to-bottom
// pipeline order.
export default function WorkflowFunnelChart({ data }: WorkflowFunnelChartProps): JSX.Element {
  // Resolve to a concrete hex per mode rather than theme.palette.text.secondary
  // (which returns a `var(--mui-...)` reference under CssVarsProvider) —
  // recharts renders `tick.fill`/`stroke` as plain SVG attributes, which
  // don't reliably re-render on a var() value the way styled DOM text does,
  // leaving the label the light-mode color even after switching to dark.
  const { mode, systemMode } = useColorScheme();
  const isDark = mode === "dark" || (mode === "system" && systemMode === "dark");
  const axisColor = isDark ? "#d4d4d4" : "#24292e";

  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No risks recorded yet.
      </Typography>
    );
  }

  const byStatus = new Map(data.map((d) => [d.workflow_status, d.count]));
  const rows = WORKFLOW_FUNNEL_ORDER.filter((s) => byStatus.has(s)).map((status) => ({
    stage: WORKFLOW_STATUS_LABELS[status] ?? status,
    count: byStatus.get(status)!,
  }));

  return (
    <BarChart
      data={rows}
      xAxisDataKey="stage"
      layout="vertical"
      bars={[{ dataKey: "count", name: "Risks", fill: WORKFLOW_STAGE_COLOR, radius: [0, 3, 3, 0] }]}
      height={Math.max(MIN_HEIGHT, rows.length * ROW_HEIGHT)}
      maxBarSize={28}
      isAnimationActive={false}
      legend={{ show: false }}
      margin={{ top: 8, right: 24, left: 8, bottom: 8 }}
      xAxis={{ show: true }}
      yAxis={{ show: false }}
      grid={{ show: false }}
    >
      <CartesianGrid horizontal={false} vertical strokeDasharray="3 3" stroke={axisColor} />
      <YAxis
        type="category"
        dataKey="stage"
        width={LABEL_AXIS_WIDTH}
        tick={{ fontSize: 11, fill: axisColor }}
        axisLine={false}
        tickLine={false}
      />
    </BarChart>
  );
}
