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

import { BarChart2, ClipboardList, LayoutDashboard, PlusCircle, ShieldAlert } from "@wso2/oxygen-ui-icons-react";
import type { NavSection } from "@components/side-nav-bar/types";
import { RiskPrivilege } from "./privileges";

// Risk Hub sidebar section. Owned by the Risk module — add Risk nav items
// here without touching the shared SideBar component.
//
// Gated like the Admin Console and Audit Hub: each item needs its route's
// privilege, and the whole section is withheld from anyone who can open none of
// the tabs — a hidden tab beats one that only 403s on click.
//
// The Registers item's requiredPrivilege (RISK_VIEW_RISKS) is also satisfied
// by being named on a risk: a grant-less Action Owner gets a synthetic
// RISK_VIEW_RISKS from useRiskPrivileges (GET /api/v1/risks/me/involvement), so
// they still see the section and the Registers tab — but nothing else — for the
// risk they were handed.
export const riskNav: NavSection = {
  id: "risk",
  label: "Risk Hub",
  icon: ShieldAlert,
  hideSectionWithoutPrivilege: [
    RiskPrivilege.ViewRisks,
    RiskPrivilege.ViewAllRisks,
    RiskPrivilege.ViewRiskDashboard,
    RiskPrivilege.CreateRisk,
    RiskPrivilege.ViewAnalytics,
  ],
  items: [
    {
      id: "risk-dashboard",
      label: "Dashboard",
      path: "/risk/dashboard",
      icon: LayoutDashboard,
      requiredPrivilege: RiskPrivilege.ViewRiskDashboard,
    },
    {
      id: "risk-registers",
      label: "Risk Registers",
      path: "/risk/registers",
      icon: ClipboardList,
      requiredPrivilege: RiskPrivilege.ViewRisks,
    },
    {
      id: "risk-add",
      label: "Add Risk",
      path: "/risk/add",
      icon: PlusCircle,
      requiredPrivilege: RiskPrivilege.CreateRisk,
    },
    {
      id: "risk-analytics",
      label: "Analytics",
      path: "/risk/analytics",
      icon: BarChart2,
      requiredPrivilege: RiskPrivilege.ViewAnalytics,
    },
  ],
};
