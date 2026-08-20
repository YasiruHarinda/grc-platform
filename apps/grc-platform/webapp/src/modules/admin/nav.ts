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

import { Settings, Users } from "@wso2/oxygen-ui-icons-react";
import type { NavSection } from "@components/side-nav-bar/types";
import { AdminPrivilege } from "./privileges";

// Admin Console sidebar section. Owned by the Admin module — add Admin nav
// items here without touching the shared SideBar component.
//
// Unlike Risk Hub (always visible, gated per-item), the WHOLE section is
// hidden from anyone lacking MANAGE_USERS — see hideSectionWithoutPrivilege's
// doc comment in side-nav-bar/types.ts for why this console's default is the
// opposite of Risk Hub's.
//
// Only "Users" exists this phase. "Manage Risk Hub" and "Manage Audit Hub"
// (the latter a stub) are later phases of the same project — see
// ADMIN_CONSOLE_DESIGN.md — and get added here when they're built, not
// pre-added as dead links now.
export const adminNav: NavSection = {
  id: "admin",
  label: "Admin Console",
  icon: Settings,
  hideSectionWithoutPrivilege: AdminPrivilege.ManageUsers,
  items: [
    {
      id: "admin-users",
      label: "Users",
      path: "/admin/users",
      icon: Users,
    },
  ],
};
