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
import { resolveVisibleNav, type SectionPrivResolver } from "./resolveVisibleNav";
import type { NavSection } from "./types";

const icon = () => null;

const riskSection: NavSection = {
  id: "risk",
  label: "Risk Hub",
  icon,
  hideSectionWithoutPrivilege: ["VIEW_RISKS", "VIEW_DASHBOARD", "CREATE_RISK", "VIEW_ANALYTICS"],
  items: [
    { id: "risk-dashboard", label: "Dashboard", path: "/risk/dashboard", icon, requiredPrivilege: "VIEW_DASHBOARD" },
    { id: "risk-registers", label: "Registers", path: "/risk/registers", icon, requiredPrivilege: "VIEW_RISKS" },
    { id: "risk-add", label: "Add", path: "/risk/add", icon, requiredPrivilege: "CREATE_RISK" },
  ],
};

const openSection: NavSection = {
  id: "audit",
  label: "Audit Hub",
  icon,
  items: [{ id: "audit-dashboard", label: "Dashboard", path: "/audit/dashboard", icon }],
};

function resolver(held: string[], loading = false): SectionPrivResolver {
  return { can: (p) => held.includes(p), loading };
}

describe("resolveVisibleNav", () => {
  it("hides a gated section while its resolver is still loading, and reports loading", () => {
    const out = resolveVisibleNav([riskSection], { risk: resolver([], true) });
    expect(out.sections).toHaveLength(0);
    expect(out.loading).toBe(true);
  });

  it("hides a gated section when the resolver holds none of its privileges", () => {
    const out = resolveVisibleNav([riskSection], { risk: resolver(["SOMETHING_ELSE"]) });
    expect(out.sections).toHaveLength(0);
    expect(out.loading).toBe(false);
  });

  it("shows a gated section and filters items by requiredPrivilege", () => {
    const out = resolveVisibleNav([riskSection], {
      risk: resolver(["VIEW_RISKS", "VIEW_DASHBOARD", "CREATE_RISK", "VIEW_ANALYTICS"]),
    });
    expect(out.sections).toHaveLength(1);
    expect(out.sections[0].items.map((i) => i.id)).toEqual(["risk-dashboard", "risk-registers", "risk-add"]);
  });

  it("shows only the Registers item for an involved Action Owner (synthetic VIEW_RISKS only)", () => {
    const out = resolveVisibleNav([riskSection], { risk: resolver(["VIEW_RISKS"]) });
    expect(out.sections).toHaveLength(1);
    expect(out.sections[0].items.map((i) => i.id)).toEqual(["risk-registers"]);
  });

  it("returns no sections when nothing is visible and loading is settled", () => {
    const out = resolveVisibleNav([riskSection], { risk: resolver([]) });
    expect(out.sections).toHaveLength(0);
    expect(out.loading).toBe(false);
  });

  it("drops a section that passes the gate but has no visible item", () => {
    // VIEW_ANALYTICS is in the section gate but no item requires it, so the
    // section would otherwise render as a bare header with no children.
    const out = resolveVisibleNav([riskSection], { risk: resolver(["VIEW_ANALYTICS"]) });
    expect(out.sections).toHaveLength(0);
    expect(out.loading).toBe(false);
  });

  it("shows a section with no gate and no resolver, with all its items", () => {
    const out = resolveVisibleNav([openSection], {});
    expect(out.sections).toHaveLength(1);
    expect(out.sections[0].items).toHaveLength(1);
  });

  it("aggregates loading across all sections' resolvers", () => {
    const out = resolveVisibleNav([openSection, riskSection], {
      audit: resolver(["X"], false),
      risk: resolver([], true),
    });
    expect(out.loading).toBe(true);
  });
});
