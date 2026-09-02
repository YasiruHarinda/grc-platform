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

import { adminNav } from "@modules/admin/nav";
import { auditNav } from "@modules/audit/nav";
import { riskNav } from "@modules/risk/nav";
import type { NavSection } from "./types";

// Every module registers its own NavSection (modules/<module>/nav.ts). To add a
// new module's section, import its nav here and append it — no other change.
//
// Order matters: it is the sidebar's top-to-bottom order, and the order
// LandingRedirect walks to pick the first tab a user can actually see.
export const SECTIONS: NavSection[] = [auditNav, riskNav, adminNav];
