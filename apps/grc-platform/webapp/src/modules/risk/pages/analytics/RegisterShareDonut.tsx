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
import type { RegisterShare } from "../../api/riskApi";
import { buildRegisterColorMap } from "../dashboard/constants";

interface RegisterShareDonutProps {
  data: RegisterShare[] | null;
}

// Cross-register comparison donut — total risk volume (open + closed,
// all-time) per register. Only rendered by the parent when the page's
// register filter is "All", since it's the one chart whose job is comparing
// registers against each other.
export default function RegisterShareDonut({ data }: RegisterShareDonutProps): JSX.Element {
  if (!data || data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No risks recorded yet.
      </Typography>
    );
  }

  const colorMap = buildRegisterColorMap(data.map((d) => d.register_name));
  const rows = data.map((d) => ({ name: d.register_name, value: d.count }));

  return (
    <PieChart
      data={rows}
      height={320}
      colors={data.map((d) => colorMap.get(d.register_name)!)}
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
