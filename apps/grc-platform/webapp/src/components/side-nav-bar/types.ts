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

import type { ComponentType } from "react";

// A single clickable item inside a sidebar section.
export interface NavItem {
  id: string; // unique, module-prefixed (e.g. "audit-dashboard")
  label: string;
  path: string;
  icon: ComponentType<{ size?: number }>;
  requiredPrivilege?: string; // when set, item is hidden until the user holds this privilege
}

// A collapsible module section in the sidebar (e.g. Audit Hub).
// Each module owns its own NavSection in modules/<module>/nav.ts so that
// the Audit and Risk owners never edit the same file.
export interface NavSection {
  id: string; // module id, e.g. "audit"
  label: string; // section heading, e.g. "Audit Hub"
  icon: ComponentType<{ size?: number }>;
  items: NavItem[];
  // When set, the ENTIRE section is hidden until the caller holds this
  // privilege (and hidden, not shown, while that is still loading — fail
  // closed for a whole section, unlike a single item's requiredPrivilege).
  //
  // Risk Hub and Audit Hub deliberately don't set this: their sections are
  // always visible, with each route gated individually via PrivilegeGuard,
  // because an empty/403'd item within an otherwise-relevant section is a
  // normal state for those hubs. The Admin Console is different — there is
  // no safe "empty" version of an admin console for a random employee to
  // land on, and its mere presence in the nav invites curiosity-clicking —
  // so its whole section is withheld instead (see modules/admin/nav.ts).
  hideSectionWithoutPrivilege?: string;
}
