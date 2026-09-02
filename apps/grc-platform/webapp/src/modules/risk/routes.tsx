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
import PrivilegeGuard from "./components/PrivilegeGuard";
import { RiskPrivilege } from "./privileges";

// Risk Hub routes, mounted under /risk by App.tsx. Owned by the Risk module —
// add Risk pages here without touching the shared App.tsx.
export const riskRoutes = (
  <Route path="risk">
    <Route index element={<Navigate to="dashboard" replace />} />
    <Route path="dashboard" element={<PrivilegeGuard privilege={RiskPrivilege.ViewRiskDashboard}><RiskDashboard /></PrivilegeGuard>} />
    {/*
      No PrivilegeGuard on registers: the Registers list is reachable by a
      named Action Owner too, who holds no RISK_VIEW_RISKS privilege. Guarding
      the route on that privilege would 403 an Action Owner out of the one tab that
      shows the risk they were handed, and would tie a direct /risk/registers
      link to the involvement fetch succeeding. The nav already hides this tab
      from anyone with no risk privilege and no Action Owner involvement
      (modules/risk/nav.ts); the backend GET /api/v1/risks scopes the response
      per caller, and RiskRegisters renders an empty list cleanly.
    */}
    <Route path="registers" element={<RiskRegisters />} />
    <Route path="add" element={<PrivilegeGuard privilege={RiskPrivilege.CreateRisk}><AddRisk /></PrivilegeGuard>} />
    <Route path="analytics" element={<PrivilegeGuard privilege={RiskPrivilege.ViewAnalytics}><RiskAnalytics /></PrivilegeGuard>} />
  </Route>
);
