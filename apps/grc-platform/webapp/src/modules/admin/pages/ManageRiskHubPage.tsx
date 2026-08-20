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

import { Box, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { type JSX, type SyntheticEvent, useState } from "react";
import ComplianceReferencesPage from "./ComplianceReferencesPage";
import RiskCategoriesPage from "./RiskCategoriesPage";
import RiskScoresPage from "./RiskScoresPage";
import RiskTeamsPage from "./RiskTeamsPage";

type SubTab = "teams" | "categories" | "compliance" | "scores";

// One route (/admin/risk-hub) with four sub-screens switched by tab, rather
// than four separate nav items — the design consolidated Users / Manage Risk
// Hub / Manage Audit Hub to three top-level Admin Console sections, with the
// Risk Hub's own reference-data screens living inside this one as tabs.
export default function ManageRiskHubPage(): JSX.Element {
  const [tab, setTab] = useState<SubTab>("teams");

  const handleChange = (_e: SyntheticEvent, value: SubTab) => setTab(value);

  return (
    <Box>
      <Typography variant="h4" fontWeight={700} sx={{ mb: 0.5 }}>
        Manage Risk Hub
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Reference data the Risk Hub's dropdowns are built from: teams, categories, compliance references, and the
        risk-scoring matrix.
      </Typography>

      <Tabs value={tab} onChange={handleChange} sx={{ mb: 2 }}>
        <Tab label="Risk Teams" value="teams" />
        <Tab label="Risk Categories" value="categories" />
        <Tab label="Compliance References" value="compliance" />
        <Tab label="Risk Scores" value="scores" />
      </Tabs>

      {tab === "teams" && <RiskTeamsPage />}
      {tab === "categories" && <RiskCategoriesPage />}
      {tab === "compliance" && <ComplianceReferencesPage />}
      {tab === "scores" && <RiskScoresPage />}
    </Box>
  );
}
