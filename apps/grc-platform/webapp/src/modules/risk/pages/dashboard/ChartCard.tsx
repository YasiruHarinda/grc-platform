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

import { Box, Paper, Typography } from "@wso2/oxygen-ui";
import type { JSX, ReactNode } from "react";
import { darkCardSx } from "../cardStyles";

interface ChartCardProps {
  title: string;
  // Optional line rendered under the title, e.g. describing the chart's scope.
  subtitle?: string;
  // Optional content (e.g. a summary pill) rendered at the header's right edge.
  headerRight?: ReactNode;
  children: ReactNode;
}

// Shared card shell for every dashboard chart and table.
export default function ChartCard({ title, subtitle, headerRight, children }: ChartCardProps): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ p: 2.5, height: "100%", ...darkCardSx }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1, mb: subtitle ? 0.25 : 1 }}>
        <Typography variant="subtitle1" fontWeight={600}>
          {title}
        </Typography>
        {headerRight}
      </Box>
      {subtitle && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {subtitle}
        </Typography>
      )}
      {children}
    </Paper>
  );
}
