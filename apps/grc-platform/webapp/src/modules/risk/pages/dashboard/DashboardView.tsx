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

import { Box, Chip, Stack, Typography } from "@wso2/oxygen-ui";
import { Info } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import type { DashboardSummary, RiskScore } from "../../api/riskApi";
import ChartCard from "./ChartCard";
import SummaryCards from "./SummaryCards";
import StatusPieChart from "./StatusPieChart";
import TreatmentByRegisterChart from "./TreatmentByRegisterChart";
import LevelCountChart from "./LevelCountChart";
import AverageResidualRiskMatrix from "./AverageResidualRiskMatrix";
import { meanRating, residualScoreMethodologySentence } from "./residualRiskMath";
import CertDistributionChart from "./CertDistributionChart";
import { certListSentence, CLOSED_COLOR } from "./constants";
import RegisterSection from "./RegisterSection";
import RepeatedRisksTable from "./RepeatedRisksTable";
import HighRisksTable from "./HighRisksTable";

interface DashboardViewProps {
  dashboard: DashboardSummary;
  scores: RiskScore[];
  // false when the page is scoped to one register — hides charts that only
  // make sense comparing across registers (their x-axis or plotted points
  // *are* the register comparison).
  isAllRegisters: boolean;
}

// Pure layout of the risk dashboard; RiskDashboard supplies fetched data.
export default function DashboardView({
  dashboard,
  scores,
  isAllRegisters,
}: DashboardViewProps): JSX.Element {
  return (
    <Stack spacing={3}>
      <SummaryCards summary={dashboard.summary} />

      {isAllRegisters ? (
        <>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "5fr 7fr" }, gap: 3 }}>
            <ChartCard title="Overall Risk Status Distribution">
              <StatusPieChart summary={dashboard.summary} />
            </ChartCard>
            <ChartCard title="Risk Treatment Strategy on Open Risks">
              <TreatmentByRegisterChart data={dashboard.treatment_by_register} />
            </ChartCard>
          </Box>

          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 3 }}>
            <ChartCard title="Count vs. Risk Level (Open Risks)">
              <LevelCountChart data={dashboard.level_counts} />
            </ChartCard>
            <ChartCard
              title="Average Residual Risk Score (Open Risks)"
              headerRight={
                (() => {
                  const overall = meanRating(dashboard.org_heatmap);
                  return overall == null ? null : (
                    <Chip label={`Overall Avg  ${overall.toFixed(2)}`} size="small" color="primary" />
                  );
                })()
              }
            >
              {(() => {
                const methodology = residualScoreMethodologySentence(dashboard.registers, dashboard.org_heatmap);
                return (
                  methodology && (
                    <Box
                      sx={{
                        border: `1px solid ${CLOSED_COLOR}`,
                        borderRadius: 1,
                        bgcolor: `${CLOSED_COLOR}0d`,
                        px: 1.5,
                        py: 1,
                        mb: 1.5,
                      }}
                    >
                      <Typography variant="body2" color="text.secondary">
                        {methodology}
                      </Typography>
                    </Box>
                  )
                );
              })()}
              <AverageResidualRiskMatrix
                registers={dashboard.registers}
                orgHeatmap={dashboard.org_heatmap}
                scores={scores}
              />
            </ChartCard>
          </Box>
        </>
      ) : (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 3 }}>
          <ChartCard title="Overall Risk Status Distribution">
            <StatusPieChart summary={dashboard.summary} />
          </ChartCard>
          <ChartCard title="Count vs. Risk Level (Open Risks)">
            <LevelCountChart data={dashboard.level_counts} />
          </ChartCard>
        </Box>
      )}

      {isAllRegisters && (
        <ChartCard
          title="Number of Open Risks against Compliance Certifications"
          subtitle={certListSentence(dashboard.cert_distribution.map((d) => d.cert_name))}
        >
          <CertDistributionChart data={dashboard.cert_distribution} />
          <Box
            sx={{
              display: "flex",
              alignItems: "flex-start",
              gap: 1,
              border: `1px solid ${CLOSED_COLOR}`,
              borderRadius: 1,
              bgcolor: `${CLOSED_COLOR}0d`,
              px: 1.5,
              py: 1,
              mt: 2,
            }}
          >
            <Box sx={{ flexShrink: 0, display: "flex", alignItems: "center", color: CLOSED_COLOR, mt: "1px" }}>
              <Info size={16} />
            </Box>
            <Typography variant="body2" color="text.secondary">
              <Typography component="span" variant="body2" fontWeight={700} color="text.primary">
                Note:
              </Typography>{" "}
              A single risk may appear across multiple compliance certifications. Fixing one risk can
              simultaneously reduce risk exposure in several certifications (making cross-certification
              remediation a high-leverage activity).
            </Typography>
          </Box>
        </ChartCard>
      )}

      {dashboard.registers.map((register) => (
        <RegisterSection key={register.register_id} register={register} scores={scores} />
      ))}

      {isAllRegisters && (
        <ChartCard title="Repeated Risks Potentially Impacting Compliance Certs">
          <RepeatedRisksTable data={dashboard.repeated_compliance_risks} />
        </ChartCard>
      )}

      <ChartCard
        title="High Severity Open Risks"
        headerRight={
          <Chip
            label={`${dashboard.high_risks.length} High Risks`}
            size="small"
            sx={{ bgcolor: "#e34948", color: "#fff", fontWeight: 600 }}
          />
        }
      >
        <HighRisksTable data={dashboard.high_risks} />
      </ChartCard>
    </Stack>
  );
}
