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

import { Settings, ShieldAlert, ShieldCheck, Users } from "@wso2/oxygen-ui-icons-react";
import type { NavSection } from "@components/side-nav-bar/types";
import { AdminPrivilege } from "./privileges";

// Admin Console sidebar section. Owned by the Admin module — add Admin nav
// items here without touching the shared SideBar component.
//
// Unlike Risk Hub (always visible, gated per-item), the WHOLE section is
// hidden from anyone lacking at least one of these three privileges — see
// hideSectionWithoutPrivilege's doc comment in side-nav-bar/types.ts for why
// this console's default is the opposite of Risk Hub's, and for why it's an
// any-of list rather than a single privilege.
//
// "Manage Audit Hub" routes to a stub page — its real reference-data screens
// (audit teams/frameworks/products) are a later phase; see
// ADMIN_CONSOLE_DESIGN.md and ManageAuditHubPage.tsx.
export const adminNav: NavSection = {
  id: "admin",
  label: "Admin Console",
  icon: Settings,
  hideSectionWithoutPrivilege: [
    AdminPrivilege.ManageUsers,
    AdminPrivilege.ManageRiskHub,
    AdminPrivilege.ManageAuditHub,
  ],
  items: [
    {
      id: "admin-users",
      label: "Users",
      path: "/admin/users",
      icon: Users,
      requiredPrivilege: AdminPrivilege.ManageUsers,
    },
    {
      id: "admin-audit-hub",
      label: "Manage Audit Hub",
      path: "/admin/audit-hub",
      // Same icon as the main Audit Hub section (modules/audit/nav.ts) so the
      // two read as related at a glance.
      icon: ShieldCheck,
      requiredPrivilege: AdminPrivilege.ManageAuditHub,
    },
    {
      id: "admin-risk-hub",
      label: "Manage Risk Hub",
      path: "/admin/risk-hub",
      // Same icon as the main Risk Hub section (modules/risk/nav.ts).
      icon: ShieldAlert,
      requiredPrivilege: AdminPrivilege.ManageRiskHub,
    },
  ],
};
