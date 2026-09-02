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

import type { NavItem, NavSection } from "./types";

// The privilege resolver a module's useXPrivileges hook returns.
export interface SectionPrivResolver {
  can: (privilege: string) => boolean;
  loading: boolean;
}

export interface VisibleSection {
  section: NavSection;
  items: NavItem[];
}

export interface ResolvedNav {
  sections: VisibleSection[];
  // True while any section's privilege resolver is still loading. The SideBar
  // ignores this and renders progressively as resolvers settle; LandingRedirect
  // waits for it to go false before it trusts `sections` enough to pick a
  // destination.
  loading: boolean;
}

// resolveVisibleNav is the single authority for "what nav is this user allowed
// to see" — consumed by both the SideBar (renders all of it) and
// LandingRedirect (navigates to the first item, or the no-access page when
// there is none).
//
//   - A section carrying `hideSectionWithoutPrivilege` is shown only when its
//     resolver is registered, done loading, and reports AT LEAST ONE of those
//     privileges. Fail closed: hidden while loading or if no resolver exists.
//   - Within a shown section, an item carrying `requiredPrivilege` is shown
//     only when the resolver reports it (and is done loading). An item without
//     one always shows.
//   - A section with neither a section gate nor a registered resolver shows
//     with all its items — the pre-privilege default.
export function resolveVisibleNav(
  sections: NavSection[],
  sectionPrivs: Record<string, SectionPrivResolver | undefined>,
): ResolvedNav {
  const loading = sections.some((s) => sectionPrivs[s.id]?.loading ?? false);

  const visible: VisibleSection[] = [];
  for (const section of sections) {
    const privs = sectionPrivs[section.id];

    if (section.hideSectionWithoutPrivilege) {
      if (!privs || privs.loading) continue;
      if (!section.hideSectionWithoutPrivilege.some((p) => privs.can(p))) continue;
    }

    const items = section.items.filter((item) => {
      if (!item.requiredPrivilege || !privs) return true;
      return !privs.loading && privs.can(item.requiredPrivilege);
    });

    visible.push({ section, items });
  }

  return { sections: visible, loading };
}
