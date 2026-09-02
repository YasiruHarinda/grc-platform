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
  // When set, the ENTIRE section is hidden until the caller holds AT LEAST
  // ONE of these privileges (and hidden, not shown, while that is still
  // loading — fail closed for a whole section, unlike a single item's
  // requiredPrivilege). Any-of, not all-of: a section's items are each gated
  // by their own requiredPrivilege, so a caller holding just one of these
  // still sees the section — with only the item(s) they're entitled to.
  //
  // All three hubs set this now. The rule is the same everywhere: a hidden
  // tab beats one that only 403s on click, and a section with zero reachable
  // items is just noise in the nav. The list is the union of every route
  // privilege the section's items gate on.
  //
  // Risk Hub's list includes RISK_VIEW_RISKS, which an Action Owner holding
  // no risk role gets as a synthetic privilege from useRiskPrivileges (see
  // its GET /api/v1/risks/me/involvement call) — so being named on a risk
  // keeps the Risk Hub section and its Registers tab visible for them, and
  // nothing else.
  hideSectionWithoutPrivilege?: string[];
}
