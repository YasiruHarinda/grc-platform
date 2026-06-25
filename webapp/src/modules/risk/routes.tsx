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

import { Route, Navigate } from "react-router";
import RiskDashboard from "@modules/risk/pages/RiskDashboard";
import RiskRegisters from "@modules/risk/pages/RiskRegisters";
import AddRisk from "@modules/risk/pages/AddRisk";
import RiskAnalytics from "@modules/risk/pages/RiskAnalytics";

// Risk Hub routes, mounted under /risk by App.tsx. Owned by the Risk module —
// add Risk pages here without touching the shared App.tsx.
export const riskRoutes = (
  <Route path="risk">
    <Route index element={<Navigate to="dashboard" replace />} />
    <Route path="dashboard" element={<RiskDashboard />} />
    <Route path="registers" element={<RiskRegisters />} />
    <Route path="add" element={<AddRisk />} />
    <Route path="analytics" element={<RiskAnalytics />} />
  </Route>
);
