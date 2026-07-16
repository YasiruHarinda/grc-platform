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

import { PieChart } from "@wso2/oxygen-ui-charts-react";
import { Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { ComplianceShare } from "../../api/riskApi";
import { buildCertColorMap } from "../dashboard/constants";

interface ComplianceDonutProps {
  data: ComplianceShare[];
}

// Org-wide "Compliance Reference Distribution": total risk count tagged per
// compliance framework, all registers combined (or scoped to one register
// when filtered) — a single aggregate view, distinct from the Dashboard's
// per-register 100%-stacked cert chart.
export default function ComplianceDonut({ data }: ComplianceDonutProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No risks tagged to a compliance framework.
      </Typography>
    );
  }

  const colorMap = buildCertColorMap(data.map((d) => d.compliance_name));
  const rows = data.map((d) => ({ name: d.compliance_name, value: d.count }));

  return (
    <PieChart
      data={rows}
      height={320}
      colors={data.map((d) => colorMap.get(d.compliance_name)!)}
      isAnimationActive={false}
      pies={[
        {
          dataKey: "value",
          nameKey: "name",
          innerRadius: "45%",
          outerRadius: "75%",
          paddingAngle: 2,
          label: ({ percent }: { percent?: number }) => `${((percent ?? 0) * 100).toFixed(1)}%`,
        },
      ]}
      legend={{ show: true, align: "center", verticalAlign: "bottom" }}
    />
  );
}
