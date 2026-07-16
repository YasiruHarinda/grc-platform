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

import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import type { JSX, ReactNode } from "react";
import type { AnalyticsSummary } from "../../api/riskApi";
import ChartCard from "../dashboard/ChartCard";
import { CLOSED_COLOR } from "../dashboard/constants";
import KpiTiles from "./KpiTiles";
import TrendChart from "./TrendChart";
import RegisterTrendChart from "./RegisterTrendChart";
import LevelDistributionChart from "./LevelDistributionChart";
import RegisterShareDonut from "./RegisterShareDonut";
import ComplianceDonut from "./ComplianceDonut";
import TreatmentRadial from "./TreatmentRadial";
import WorkflowFunnelChart from "./WorkflowFunnelChart";
import AgingRisksTable from "./AgingRisksTable";

interface AnalyticsViewProps {
  analytics: AnalyticsSummary;
  isAllRegisters: boolean;
}

// Small blue-bordered note, matching the callout style already used on the
// Risk Dashboard (methodology / compliance-cert notes).
function NoteBox({ children }: { children: ReactNode }): JSX.Element {
  return (
    <Box
      sx={{
        border: `1px solid ${CLOSED_COLOR}`,
        borderRadius: 1,
        bgcolor: `${CLOSED_COLOR}0d`,
        px: 1.5,
        py: 1,
        mb: 2,
      }}
    >
      <Typography variant="body2" color="text.secondary">
        {children}
      </Typography>
    </Box>
  );
}

// Pure layout of the Risk Analytics page; RiskAnalytics supplies fetched data
// and the register filter. Deliberately avoids re-showing what the Dashboard
// already covers as a point-in-time snapshot — every chart here either adds a
// time dimension or is an org-wide aggregate the Dashboard doesn't compute.
export default function AnalyticsView({ analytics, isAllRegisters }: AnalyticsViewProps): JSX.Element {
  return (
    <Stack spacing={3}>
      <KpiTiles loading={false} kpis={analytics.kpis} />

      <ChartCard title="Overall Risk Trend Over Time">
        <NoteBox>
          Monthly count of risks identified vs. closed{isAllRegisters ? " across all registers" : " for the selected register"}, with the average residual
          score of newly identified risks overlaid, over the trailing 12 months.
        </NoteBox>
        <TrendChart data={analytics.trend} />
      </ChartCard>

      {isAllRegisters && (
        <>
          <ChartCard title="Risks Identified by Source Register">
            <NoteBox>
              Monthly count of risks identified, broken down by source register, over the trailing 12
              months.
            </NoteBox>
            <RegisterTrendChart
              data={analytics.identified_by_register ?? []}
              emptyMessage="No risks identified in the last 12 months."
            />
          </ChartCard>

          <ChartCard title="Risks Closed by Source Register">
            <NoteBox>
              Monthly count of risks closed, broken down by source register, over the trailing 12 months.
            </NoteBox>
            <RegisterTrendChart
              data={analytics.closed_by_register ?? []}
              emptyMessage="No risks closed in the last 12 months."
            />
          </ChartCard>
        </>
      )}

      <ChartCard title="Risk Level Distribution Over Time">
        <NoteBox>
          Monthly count of risks identified, stacked by residual severity level (High/Medium/Low), over
          the trailing 12 months.
        </NoteBox>
        <LevelDistributionChart data={analytics.level_distribution} />
      </ChartCard>

      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            md: isAllRegisters ? "repeat(3, 1fr)" : "repeat(2, 1fr)",
          },
          gap: 3,
        }}
      >
        {isAllRegisters && (
          <ChartCard title="Risks by Register">
            <NoteBox>Total risk count (all time) per source register.</NoteBox>
            <RegisterShareDonut data={analytics.register_shares} />
          </ChartCard>
        )}
        <ChartCard title="Compliance Reference Distribution">
          <NoteBox>Total risk count (all time) tagged per compliance framework.</NoteBox>
          <ComplianceDonut data={analytics.compliance_distribution} />
        </ChartCard>
        <ChartCard title="Risk Treatment Strategies">
          <NoteBox>Current count of open risks by treatment strategy.</NoteBox>
          <TreatmentRadial data={analytics.treatment_mix} />
        </ChartCard>
      </Box>

      <ChartCard title="Workflow Status Funnel">
        <NoteBox>
          Current count of every non-cancelled risk by workflow stage (including Closed) showing where
          risks are sitting right now across the approval and remediation pipeline.
        </NoteBox>
        <WorkflowFunnelChart data={analytics.workflow_funnel} />
      </ChartCard>

      <ChartCard title="Aging Open Risks">
        <NoteBox>
          The 10 oldest currently open risks, ranked by days since they were identified.
        </NoteBox>
        <AgingRisksTable data={analytics.aging_risks} />
      </ChartCard>
    </Stack>
  );
}
