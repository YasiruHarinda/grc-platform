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

import { describe, expect, it } from "vitest";
import { mergeRiskPrivileges } from "./mergeRiskPrivileges";
import { RiskPrivilege } from "../privileges";

describe("mergeRiskPrivileges", () => {
  it("passes null (backend allow-all) straight through, regardless of involvement", () => {
    expect(mergeRiskPrivileges(null, false)).toBeNull();
    expect(mergeRiskPrivileges(null, true)).toBeNull();
  });

  it("returns the grant privileges unchanged when not named on a risk", () => {
    const out = mergeRiskPrivileges([RiskPrivilege.CreateRisk], false);
    expect([...out!].sort()).toEqual([RiskPrivilege.CreateRisk]);
  });

  it("materialises a synthetic RISK_VIEW_RISKS for a grant-less Action Owner", () => {
    const out = mergeRiskPrivileges([], true);
    expect(out!.has(RiskPrivilege.ViewRisks)).toBe(true);
    expect(out!.size).toBe(1);
  });

  it("adds RISK_VIEW_RISKS alongside real grants when both apply", () => {
    const out = mergeRiskPrivileges([RiskPrivilege.CreateRisk], true);
    expect(out!.has(RiskPrivilege.CreateRisk)).toBe(true);
    expect(out!.has(RiskPrivilege.ViewRisks)).toBe(true);
  });

  it("does not duplicate RISK_VIEW_RISKS when it is already granted", () => {
    const out = mergeRiskPrivileges([RiskPrivilege.ViewRisks], true);
    expect([...out!]).toEqual([RiskPrivilege.ViewRisks]);
  });
});
