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
}

// A collapsible module section in the sidebar (e.g. Audit Hub).
// Each module owns its own NavSection in modules/<module>/nav.ts so that
// the Audit and Risk owners never edit the same file.
export interface NavSection {
  id: string; // module id, e.g. "audit"
  label: string; // section heading, e.g. "Audit Hub"
  icon: ComponentType<{ size?: number }>;
  items: NavItem[];
}
