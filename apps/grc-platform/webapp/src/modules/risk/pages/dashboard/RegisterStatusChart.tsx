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
import { Box, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RegisterStatusLevelCount } from "../../api/riskApi";
import {
  LEVEL_FALLBACK_COLORS,
  LEVEL_LABELS,
  LEVEL_ORDER,
  STATUS_BUCKET_LABELS,
  STATUS_BUCKET_ORDER,
  labelColorOn,
  stackedSegmentAccessor,
} from "./constants";

interface RegisterStatusChartProps {
  data: RegisterStatusLevelCount[];
}

const CHART_HEIGHT = 280;

// Per-register risk-status chart: every non-cancelled risk bucketed by status
// (closed, or an open risk's treatment strategy) on the x-axis, stacked by
// residual level so the severity mix within each status is visible. All 5
// buckets and all 3 levels are always shown, even at zero, for a consistent
// axis/legend across registers.
export default function RegisterStatusChart({ data }: RegisterStatusChartProps): JSX.Element {
  const rows = new Map<string, Record<string, string | number>>();
  const colorOf: Record<string, string> = { ...LEVEL_FALLBACK_COLORS };
  for (const d of data) {
    if (!rows.has(d.bucket)) rows.set(d.bucket, { bucket: STATUS_BUCKET_LABELS[d.bucket] ?? d.bucket });
    rows.get(d.bucket)![d.risk_level] = d.count;
    colorOf[d.risk_level] = d.color_code;
  }

  const bars = LEVEL_ORDER.map((level) => ({
    dataKey: level,
    name: LEVEL_LABELS[level] ?? level,
    fill: colorOf[level],
    stackId: "status",
    radius: 0,
    label: {
      position: "center",
      fontSize: 11,
      fill: labelColorOn(colorOf[level]),
      valueAccessor: stackedSegmentAccessor,
      formatter: (value: unknown) => (Number(value) > 0 ? Number(value) : ""),
    },
  }));

  const chartRows = STATUS_BUCKET_ORDER.map(
    (bucket) => rows.get(bucket) ?? { bucket: STATUS_BUCKET_LABELS[bucket] },
  );

  return (
    <Box sx={{ display: "flex", alignItems: "stretch", gap: 0.5 }}>
      <Box
        sx={{
          height: CHART_HEIGHT,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Typography
          variant="caption"
          color="text.secondary"
          noWrap
          sx={{ writingMode: "vertical-rl", transform: "rotate(180deg)" }}
        >
          Number of Risks
        </Typography>
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <BarChart
          data={chartRows}
          xAxisDataKey="bucket"
          bars={bars}
          height={CHART_HEIGHT}
          maxBarSize={56}
          isAnimationActive={false}
          margin={{ top: 8, right: 16, left: 8, bottom: 0 }}
          yAxis={{ show: true }}
        />
      </Box>
    </Box>
  );
}
