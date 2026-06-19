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

import { Box, Sidebar } from "@wso2/oxygen-ui";
import { type JSX, useEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { auditNav } from "@modules/audit/nav";
import { riskNav } from "@modules/risk/nav";

// Each module registers its own NavSection (modules/<module>/nav.ts). To add a
// new module's section, import its nav here and append it — no other change.
const SECTIONS = [auditNav, riskNav];

const NAV_PATHS: Record<string, string> = Object.fromEntries(
  SECTIONS.flatMap((s) => s.items).map((i) => [i.id, i.path]),
);

interface SideBarProps {
  collapsed: boolean;
  expandedMenus?: Record<string, boolean>;
  onSelect?: (id: string) => void;
  onToggleExpand?: (id: string) => void;
}

export default function SideBar({
  collapsed,
  expandedMenus = {},
  onSelect,
  onToggleExpand,
}: SideBarProps): JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();

  // When true, the sidebar is temporarily expanded from collapsed state on click.
  const [tempExpanded, setTempExpanded] = useState(false);

  const segments = location.pathname.split("/").filter(Boolean);
  const module = segments[0] ?? "audit";
  const page = segments[1] ?? "dashboard";
  const activeItem = `${module}-${page}`;

  // Collapse temp-expansion whenever the actual sidebar prop changes to expanded.
  useEffect(() => {
    if (!collapsed) setTempExpanded(false);
  }, [collapsed]);

  // Auto-expand the current module's submenu on first render.
  const initialized = useRef(false);
  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    if (SECTIONS.some((s) => s.id === module) && !expandedMenus[module]) {
      onToggleExpand?.(module);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // The sidebar is only visually collapsed when both the prop says so AND we are
  // not in a temporary click-to-expand state.
  const effectiveCollapsed = collapsed && !tempExpanded;

  const handleToggleExpand = (id: string) => {
    onToggleExpand?.(id);
  };

  const handleSelect = (id: string) => {
    const path = NAV_PATHS[id];
    if (path) navigate(path);
    onSelect?.(id);
  };

  // Hovering over the collapsed sidebar expands it temporarily.
  const handleMouseEnter = () => {
    if (collapsed) setTempExpanded(true);
  };

  // Mouse leaving collapses it back to icon-only.
  const handleMouseLeave = () => {
    if (tempExpanded) setTempExpanded(false);
  };

  return (
    // display: contents makes this Box transparent to layout while still
    // capturing hover events for the temp-expand behaviour.
    <Box
      sx={{ display: "contents" }}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      <Sidebar
        collapsed={effectiveCollapsed}
        activeItem={activeItem}
        expandedMenus={expandedMenus}
        onSelect={handleSelect}
        onToggleExpand={handleToggleExpand}
        sx={{
          // Chevron: points right (→) when closed, down (↓) when open.
          // Oxygen UI renders ChevronDown when closed and ChevronUp when open.
          // rotate(-90deg) on ↓ = → ; rotate(180deg) on ↑ = ↓
          "& .MuiSidebar-itemChevron .lucide-chevron-down": {
            transform: "rotate(-90deg)",
          },
          "& .MuiSidebar-itemChevron .lucide-chevron-up": {
            transform: "rotate(180deg)",
          },
        }}
      >
        <Sidebar.Nav>
          <Sidebar.Category>
            {SECTIONS.map((section) => (
              <Sidebar.Item key={section.id} id={section.id}>
                <Sidebar.ItemIcon>
                  <section.icon size={20} />
                </Sidebar.ItemIcon>
                <Sidebar.ItemLabel>{section.label}</Sidebar.ItemLabel>
                {section.items.map((item) => (
                  <Sidebar.Item key={item.id} id={item.id}>
                    <Sidebar.ItemIcon>
                      <item.icon size={20} />
                    </Sidebar.ItemIcon>
                    <Sidebar.ItemLabel>{item.label}</Sidebar.ItemLabel>
                  </Sidebar.Item>
                ))}
              </Sidebar.Item>
            ))}
          </Sidebar.Category>
        </Sidebar.Nav>
      </Sidebar>
    </Box>
  );
}
