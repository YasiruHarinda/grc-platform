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

import { RiskPrivilege } from "../privileges";

// mergeRiskPrivileges folds the identity-axis signal
// (GET /api/v1/risks/me/involvement) into the grant-derived privilege list
// from GET /api/v1/me/privileges.
//
// An Action Owner may hold no risk role at all, so their /me/privileges
// response is empty — yet they must still see the Risk Hub nav section and its
// Registers tab. When they are `namedOnRisk`, we materialise RISK_VIEW_RISKS
// into the set so every consumer (SideBar, PrivilegeGuard) treats them as
// holding it, with no consumer needing to know the identity axis exists.
//
// `names === null` is backend allow-all (local dev, no privilege store): pass
// it straight through — `can()` already returns true for everything there, and
// the involvement answer is irrelevant.
export function mergeRiskPrivileges(
  names: string[] | null,
  namedOnRisk: boolean,
): Set<string> | null {
  if (names === null) return null;
  const set = new Set(names);
  if (namedOnRisk) set.add(RiskPrivilege.ViewRisks);
  return set;
}
