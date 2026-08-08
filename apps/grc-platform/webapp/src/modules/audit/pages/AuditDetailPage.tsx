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

import {
  Box,
  Button,
  Chip,
  Divider,
  LinearProgress,
  Paper,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  CalendarDays,
  ChevronLeft,
  History,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import FilterPanel from "@modules/audit/components/FilterPanel";
import AuditStatusChip from "@modules/audit/components/AuditStatusChip";
import ControlsTable, { ColumnPicker } from "@modules/audit/components/ControlsTable";
import {
  CONTROL_COLUMNS,
  DEFAULT_VISIBLE_CONTROL_COLUMNS,
  CONTROL_COLUMNS_STORAGE_KEY,
} from "@modules/audit/components/controlColumns";
import ControlDrawer from "@modules/audit/components/ControlDrawer";
import ControlSettingsPanel from "@modules/audit/components/ControlSettingsPanel";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import { useGetAudit } from "@modules/audit/api/useGetAudit";
import { useGetControls } from "@modules/audit/api/useGetControls";
import { daysLeft, formatDateRange } from "@modules/audit/utils/format";
import {
  EMPTY_CONTROL_FILTERS,
  applyControlFilters,
  activeFilterCount,
} from "@modules/audit/utils/controlFilters";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { AuditControl, ControlStatus } from "@modules/audit/types/audit";

// ── Active filter chip helpers ────────────────────────────────────────────────

const FILTER_COLUMN_LABELS: Record<string, string> = {
  status:          "Status",
  requirementType: "Req. Type",
  controlType:     "Control Type",
  scope:           "Scope",
  teamName:        "Team",
  auditorName:     "Auditor POC",
  ownerName:       "Process Owner",
};

const FILTER_VALUE_LABELS: Record<string, Record<string, string>> = {
  requirementType: { DESIGN: "Design", OE: "OE" },
  controlType:     { CONFIG: "Config", NON_CONFIG: "Non-Config" },
  scope:           { COMMON: "Common", PRODUCT_SPECIFIC: "Product Specific" },
};

function getFilterValueLabel(key: string, value: string): string {
  if (key === "status") {
    if (value === "OVERDUE") return "Overdue";
    return CONTROL_STATUS_LABELS[value as ControlStatus] ?? value;
  }
  return FILTER_VALUE_LABELS[key]?.[value] ?? value;
}

// ── Quick filter (tab) helpers ────────────────────────────────────────────────

type QuickFilter = "approved" | "inProgress" | "overdue";
const QUICK_FILTERS: QuickFilter[] = ["approved", "inProgress", "overdue"];

function applyQuickFilter(controls: AuditControl[], qf: QuickFilter): AuditControl[] {
  if (qf === "approved") return controls.filter((c) => c.status === "COMPLETE");
  if (qf === "overdue") return controls.filter((c) => c.isOverdue);
  return controls.filter((c) => c.status !== "COMPLETE" && !c.isOverdue);
}

const ENDING_SOON_DAYS = 14;

export default function AuditDetailPage(): JSX.Element {
  const navigate = useNavigate();
  const { auditId: auditIdParam } = useParams<{ auditId: string }>();
  const auditId = parseInt(auditIdParam ?? "0", 10);

  const {
    data: audit,
    isLoading: auditLoading,
    isError: auditError,
  } = useGetAudit(auditId);

  const {
    data: controlsData,
    isLoading: controlsLoading,
    isError: controlsError,
  } = useGetControls(auditId);

  const [filters, setFilters] = useState<Record<string, string[]>>(EMPTY_CONTROL_FILTERS);
  const [search, setSearch] = useState("");
  const [selectedControl, setSelectedControl] = useState<AuditControl | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const { can } = useAuditPrivileges();
  const canManageControls = can(AuditPrivilege.ManageControls);

  const controls = useMemo(() => controlsData?.items ?? [], [controlsData]);

  const [searchParams, setSearchParams] = useSearchParams();

  // Quick filter (tab) — persisted in the URL (?filter=overdue) so the
  // dashboard and shared links can open a pre-filtered view.
  const filterParam = searchParams.get("filter");
  const activeQuickFilter: QuickFilter | null =
    QUICK_FILTERS.includes(filterParam as QuickFilter) ? (filterParam as QuickFilter) : null;

  function setQuickFilter(qf: QuickFilter | null) {
    const next = new URLSearchParams(searchParams);
    if (qf === null) next.delete("filter");
    else next.set("filter", qf);
    setSearchParams(next, { replace: true });
    setFilters(EMPTY_CONTROL_FILTERS);
    setSearch("");
  }

  // Deep link from the dashboard: ?control={id} opens that control's drawer once
  // controls have loaded, then the param is cleared so it doesn't re-open on close.
  useEffect(() => {
    const cid = searchParams.get("control");
    if (!cid || controls.length === 0) return;
    const match = controls.find((c) => c.id === Number(cid));
    if (match) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSelectedControl(match);
      searchParams.delete("control");
      setSearchParams(searchParams, { replace: true });
    }
  }, [controls, searchParams, setSearchParams]);

  // Column visibility for the controls table — the picker lives in the filter bar.
  const [visibleColumnIds, setVisibleColumnIds] = useState<string[]>(() => {
    try {
      const stored = localStorage.getItem(CONTROL_COLUMNS_STORAGE_KEY);
      if (stored) {
        const parsed: unknown = JSON.parse(stored);
        if (Array.isArray(parsed) && parsed.every((v) => typeof v === "string")) {
          return parsed as string[];
        }
      }
    } catch { /* ignore malformed storage */ }
    return DEFAULT_VISIBLE_CONTROL_COLUMNS;
  });
  useEffect(() => {
    try { localStorage.setItem(CONTROL_COLUMNS_STORAGE_KEY, JSON.stringify(visibleColumnIds)); } catch { /* ignore */ }
  }, [visibleColumnIds]);

  // Quick filter (tab) takes precedence over panel filters
  const filteredControls =
    activeQuickFilter !== null
      ? applyQuickFilter(controls, activeQuickFilter)
      : applyControlFilters(controls, filters, search);

  const approvedCount = controls.filter((c) => c.status === "COMPLETE").length;
  const inProgressCount = controls.filter(
    (c) => c.status !== "COMPLETE" && !c.isOverdue,
  ).length;
  const overdueCount = controls.filter((c) => c.isOverdue).length;
  const approvedPct = controls.length > 0 ? Math.round((approvedCount / controls.length) * 100) : 0;

  function handleFilterChange(newFilters: Record<string, string[]>) {
    setFilters(newFilters);
    if (activeQuickFilter !== null) {
      const next = new URLSearchParams(searchParams);
      next.delete("filter");
      setSearchParams(next, { replace: true });
    }
  }

  function handleSearchChange(newSearch: string) {
    setSearch(newSearch);
    if (activeQuickFilter !== null) {
      const next = new URLSearchParams(searchParams);
      next.delete("filter");
      setSearchParams(next, { replace: true });
    }
  }

  const isFiltered =
    activeQuickFilter !== null ||
    activeFilterCount(filters) > 0 ||
    search.trim().length > 0;

  // Return to the framework-scoped audit list the user likely came from
  // (AuditsListPage's drilled view, /audit/audits?framework=<id>) rather than
  // always dropping back to the top-level framework overview — the audit's
  // own framework id is already loaded, so no navigation state needs threading.
  const handleBack = () =>
    void navigate(audit ? `/audit/audits?framework=${audit.framework.id}` : "/audit/audits");

  // Days-left pill for active audits.
  const remaining = audit?.status === "ACTIVE" ? daysLeft(audit.periodEnd) : null;
  const endingSoon = remaining !== null && remaining >= 0 && remaining <= ENDING_SOON_DAYS;

  if (auditError || controlsError) {
    return (
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          py: 10,
          gap: 2,
        }}
      >
        <Typography variant="body1" color="text.secondary">
          Failed to load audit.
        </Typography>
        <Button variant="outlined" onClick={handleBack}>
          Back to Audits
        </Button>
      </Box>
    );
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      {/* Back button */}
      <Button
        startIcon={<ChevronLeft size={16} />}
        onClick={handleBack}
        sx={{ mb: 2, textTransform: "none", color: "text.secondary", pl: 0 }}
      >
        Audits
      </Button>

      {/* Audit header card — identity + progress in one place */}
      {auditLoading ? (
        <Skeleton variant="rectangular" height={130} sx={{ borderRadius: 2, mb: 3 }} />
      ) : (
        audit && (
          <Paper variant="outlined" sx={{ borderRadius: 2, p: 2.5, mb: 3 }}>
            <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 2, flexWrap: "wrap" }}>
              <Box sx={{ minWidth: 260 }}>
                <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" mb={0.75}>
                  <Typography variant="h5" fontWeight={700}>
                    {audit.name}
                  </Typography>
                  <AuditStatusChip status={audit.status} />
                </Stack>
                <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                  <Typography variant="body2" color="text.secondary">
                    {audit.framework.name}
                  </Typography>
                  <Divider orientation="vertical" flexItem />
                  <Typography variant="body2" color="text.secondary">
                    {audit.product.name}
                  </Typography>
                  <Divider orientation="vertical" flexItem />
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <CalendarDays size={13} />
                    <Typography variant="body2" color="text.secondary">
                      {formatDateRange(audit.periodStart, audit.periodEnd)}
                    </Typography>
                  </Box>
                  {remaining !== null && remaining >= 0 && (
                    <Chip
                      label={remaining === 0 ? "ends today" : `${remaining}d left`}
                      size="small"
                      sx={{
                        height: 20, fontSize: "0.68rem", fontWeight: 700,
                        color: endingSoon ? "#E53935" : "text.secondary",
                        bgcolor: endingSoon ? "rgba(229,57,53,0.12)" : "action.hover",
                        "[data-color-scheme='dark'] &": endingSoon ? { bgcolor: "rgba(229,57,53,0.25)" } : undefined,
                      }}
                    />
                  )}
                </Stack>
              </Box>
              <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
                <Button
                  variant="outlined"
                  startIcon={<History size={16} />}
                  onClick={() => void navigate(`/audit/audits/${auditId}/activity`)}
                  sx={{ textTransform: "none" }}
                >
                  Activity Log
                </Button>
                {canManageControls && (
                  <Button
                    variant="outlined"
                    startIcon={<Settings size={16} />}
                    onClick={() => setSettingsOpen(true)}
                    sx={{ textTransform: "none" }}
                  >
                    Manage Controls
                  </Button>
                )}
              </Stack>
            </Box>

            {/* Completion bar */}
            <Box sx={{ mt: 2 }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.75, flexWrap: "wrap", gap: 1 }}>
                <Typography variant="body2" color="text.secondary">
                  <Box component="span" fontWeight={700} color="text.primary">
                    {approvedCount}/{controls.length}
                  </Box>{" "}
                  controls approved ({approvedPct}%)
                </Typography>
                {overdueCount > 0 && (
                  <Chip
                    icon={<AlertTriangle size={12} />}
                    label={`${overdueCount} overdue`}
                    size="small"
                    clickable
                    onClick={() => setQuickFilter("overdue")}
                    sx={{
                      height: 22, fontSize: "0.7rem", fontWeight: 700,
                      color: "#E53935", bgcolor: "transparent", border: "1px solid #E53935",
                      "& .MuiChip-icon": { color: "#E53935" },
                    }}
                  />
                )}
              </Box>
              <LinearProgress
                variant="determinate"
                value={approvedPct}
                color={overdueCount > 0 ? "warning" : "success"}
                sx={{ height: 8, borderRadius: 4 }}
              />
            </Box>
          </Paper>
        )
      )}

      {/* Quick filter tabs — replace the old stat cards; identical filtering */}
      {controlsLoading ? (
        <Skeleton variant="rectangular" height={42} sx={{ borderRadius: 1, mb: 2 }} />
      ) : (
        <Tabs
          value={activeQuickFilter ?? "all"}
          onChange={(_, v: string) => setQuickFilter(v === "all" ? null : (v as QuickFilter))}
          sx={{
            mb: 2, borderBottom: 1, borderColor: "divider", minHeight: 40,
            "& .MuiTab-root": { minHeight: 40, textTransform: "none", fontWeight: 600 },
          }}
        >
          <Tab value="all" label={`All (${controls.length})`} />
          <Tab value="approved" label={`Approved (${approvedCount})`} />
          <Tab value="inProgress" label={`In Progress (${inProgressCount})`} />
          <Tab
            value="overdue"
            label={`Overdue (${overdueCount})`}
            sx={overdueCount > 0 ? { color: "#E53935", "&.Mui-selected": { color: "#E53935" } } : undefined}
          />
        </Tabs>
      )}

      {/* Filter + search bar */}
      <Box sx={{ mb: 1.5 }}>
        <FilterPanel
          search={search}
          onSearchChange={handleSearchChange}
          searchPlaceholder="Search by control number or description..."
        />

        {/* Active filter chips — always rendered to prevent layout shift */}
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75, mt: 1, alignItems: "center", minHeight: 28 }}>
          {activeFilterCount(filters) > 0 && (
            <>
              {Object.entries(filters).flatMap(([key, values]) =>
                values.map((value) => (
                  <Chip
                    key={`${key}:${value}`}
                    label={`${FILTER_COLUMN_LABELS[key]}: ${getFilterValueLabel(key, value)}`}
                    size="small"
                    onDelete={() =>
                      handleFilterChange({ ...filters, [key]: filters[key].filter((v) => v !== value) })
                    }
                    sx={{ fontSize: "0.78rem" }}
                  />
                ))
              )}
              <Button
                size="small"
                onClick={() => handleFilterChange(EMPTY_CONTROL_FILTERS)}
                sx={{ textTransform: "none", fontSize: "0.78rem", color: "text.secondary" }}
              >
                Clear all
              </Button>
            </>
          )}
          {isFiltered && (
            <Typography variant="caption" color="text.secondary">
              {filteredControls.length} of {controls.length} controls
            </Typography>
          )}
          <Box sx={{ ml: "auto" }}>
            <ColumnPicker
              columns={CONTROL_COLUMNS}
              visible={visibleColumnIds}
              onChange={setVisibleColumnIds}
              onReset={() => setVisibleColumnIds(DEFAULT_VISIBLE_CONTROL_COLUMNS)}
            />
          </Box>
        </Box>
      </Box>

      {/* Controls table */}
      <ControlsTable
        controls={filteredControls}
        allControls={controls}
        filters={filters}
        onFiltersChange={handleFilterChange}
        isLoading={controlsLoading}
        onRowClick={(control) => setSelectedControl(control)}
        visibleColumnIds={visibleColumnIds}
      />

      {/* Control detail drawer */}
      <ControlDrawer
        control={selectedControl}
        open={selectedControl !== null}
        onClose={() => setSelectedControl(null)}
      />

      {/* Control settings panel */}
      <ControlSettingsPanel
        auditId={auditId}
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </Box>
  );
}
