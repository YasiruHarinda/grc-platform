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

import { ClipboardList, LayoutDashboard, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import type { NavSection } from "@components/side-nav-bar/types";
import { AuditPrivilege } from "./privileges";

// Audit Hub sidebar section. Owned by the Audit module — add Audit nav items
// here without touching the shared SideBar component.
//
// Gated like the Admin Console, not like Risk Hub: each item needs its route's
// privilege, and the whole section is withheld from anyone holding none of
// them — a hidden tab beats one that only 403s on click.
export const auditNav: NavSection = {
  id: "audit",
  label: "Audit Hub",
  icon: ShieldCheck,
  hideSectionWithoutPrivilege: [AuditPrivilege.ViewAudits],
  items: [
    {
      id: "audit-dashboard",
      label: "Dashboard",
      path: "/audit/dashboard",
      icon: LayoutDashboard,
      requiredPrivilege: AuditPrivilege.ViewAudits,
    },
    {
      id: "audit-audits",
      label: "Audits",
      path: "/audit/audits",
      icon: ClipboardList,
      requiredPrivilege: AuditPrivilege.ViewAudits,
    },
  ],
};
