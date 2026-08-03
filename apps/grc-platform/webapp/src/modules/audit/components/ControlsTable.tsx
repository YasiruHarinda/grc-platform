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
  Button,
  Checkbox,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Popover,
  Skeleton,
  TablePagination,
  TextField,
  Tooltip,
} from "@wso2/oxygen-ui";
import { Box, ListingTable, Typography } from "@wso2/oxygen-ui";
import type { ListingTableSortDirection } from "@wso2/oxygen-ui";
import { AlertCircle, Filter, Search, SlidersHorizontal, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX, type ReactNode } from "react";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import UserAvatar from "@modules/audit/components/UserAvatar";
import { formatAuditDate } from "@modules/audit/utils/format";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { AuditControl, ControlStatus } from "@modules/audit/types/audit";

// ── Column filter dropdown ──────────────────────────────────────────────────

export interface ColumnFilterProps {
  label: string;
  options: { label: string; value: string }[];
  selected: string[];
  onChange: (values: string[]) => void;
  searchable?: boolean;
}

/** Filter icon + checkbox popover for a table column header. Exported for reuse
 * by other tables that want the same in-header filter UX (e.g. AuditActivityLogPage). */
export function ColumnFilter({ label, options, selected, onChange, searchable = false }: ColumnFilterProps): JSX.Element {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");

  const isActive = selected.length > 0;
  const open = Boolean(anchorEl);

  const visible = query.trim()
    ? options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()))
    : options;

  function toggle(value: string) {
    onChange(
      selected.includes(value)
        ? selected.filter((v) => v !== value)
        : [...selected, value],
    );
  }

  function handleClose() {
    setAnchorEl(null);
    setQuery("");
  }

  return (
    <>
      <IconButton
        size="small"
        aria-label={`Filter by ${label}`}
        onClick={(e) => { e.stopPropagation(); setAnchorEl(e.currentTarget); }}
        sx={{
          ml: 0.25,
          p: 0.25,
          borderRadius: 0.75,
          color: isActive ? "primary.main" : "action.disabled",
          bgcolor: isActive ? "rgba(25,118,210,0.08)" : "transparent",
          "&:hover": {
            color: isActive ? "primary.main" : "text.secondary",
            bgcolor: isActive ? "rgba(25,118,210,0.12)" : "action.hover",
          },
        }}
      >
        <Filter size={12} />
      </IconButton>

      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
        slotProps={{ paper: { sx: { width: 230, borderRadius: 2, mt: 0.5 } } }}
        onClick={(e) => e.stopPropagation()}
      >
        <Box sx={{ p: 1.25 }}>
          {searchable && (
            <TextField
              size="small"
              fullWidth
              placeholder="Search..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              autoFocus
              sx={{ mb: 0.75 }}
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <Search size={14} />
                    </InputAdornment>
                  ),
                  endAdornment: query ? (
                    <InputAdornment position="end">
                      <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => setQuery("")}>
                        <X size={12} />
                      </IconButton>
                    </InputAdornment>
                  ) : null,
                },
              }}
            />
          )}

          {isActive && (
            <Button
              size="small"
              onClick={() => onChange([])}
              sx={{ textTransform: "none", fontSize: "0.72rem", py: 0.25, mb: 0.5, display: "block" }}
            >
              Clear ({selected.length} selected)
            </Button>
          )}

          <Box sx={{ maxHeight: 260, overflowY: "auto" }}>
            {visible.length === 0 ? (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ px: 1, py: 1, display: "block" }}
              >
                No matches
              </Typography>
            ) : (
              visible.map((opt) => (
                <FormControlLabel
                  key={opt.value}
                  control={
                    <Checkbox
                      size="small"
                      checked={selected.includes(opt.value)}
                      onChange={() => toggle(opt.value)}
                      disableRipple
                      sx={{ p: 0.5 }}
                    />
                  }
                  label={
                    <Typography variant="body2" sx={{ fontSize: "0.82rem", lineHeight: 1.4 }}>
                      {opt.label}
                    </Typography>
                  }
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    px: 0.5,
                    py: 0.1,
                    borderRadius: 1,
                    mx: 0,
                    width: "100%",
                    "&:hover": { bgcolor: "action.hover" },
                  }}
                />
              ))
            )}
          </Box>
        </Box>
      </Popover>
    </>
  );
}

// ── Column visibility picker ─────────────────────────────────────────────────

interface ColumnPickerProps {
  columns: { id: string; label: string; alwaysVisible?: boolean }[];
  visible: string[];
  onChange: (ids: string[]) => void;
  onReset: () => void;
}

export function ColumnPicker({ columns, visible, onChange, onReset }: ColumnPickerProps): JSX.Element {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const open = Boolean(anchorEl);

  function toggle(id: string) {
    onChange(visible.includes(id) ? visible.filter((v) => v !== id) : [...visible, id]);
  }

  return (
    <>
      <Button
        size="small"
        startIcon={<SlidersHorizontal size={15} />}
        onClick={(e) => setAnchorEl(e.currentTarget)}
        sx={{ textTransform: "none", color: "text.secondary" }}
      >
        Columns
      </Button>

      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
        slotProps={{ paper: { sx: { width: 240, borderRadius: 2, mt: 0.5 } } }}
      >
        <Box sx={{ p: 1.25 }}>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
              Show columns
            </Typography>
            <Button size="small" onClick={onReset} sx={{ textTransform: "none", fontSize: "0.72rem", py: 0.25 }}>
              Reset
            </Button>
          </Box>
          <Box sx={{ maxHeight: 320, overflowY: "auto" }}>
            {columns.map((col) => (
              <FormControlLabel
                key={col.id}
                control={
                  <Checkbox
                    size="small"
                    checked={col.alwaysVisible || visible.includes(col.id)}
                    disabled={col.alwaysVisible}
                    onChange={() => toggle(col.id)}
                    disableRipple
                    sx={{ p: 0.5 }}
                  />
                }
                label={
                  <Typography variant="body2" sx={{ fontSize: "0.82rem", lineHeight: 1.4 }}>
                    {col.label}
                  </Typography>
                }
                sx={{
                  display: "flex",
                  alignItems: "center",
                  px: 0.5,
                  py: 0.1,
                  borderRadius: 1,
                  mx: 0,
                  width: "100%",
                  "&:hover": { bgcolor: "action.hover" },
                }}
              />
            ))}
          </Box>
        </Box>
      </Popover>
    </>
  );
}

// ── Helpers ─────────────────────────────────────────────────────────────────

const REQ_TYPE_LABELS: Record<string, string> = {
  DESIGN: "Design",
  OE: "OE",
};

const CTRL_TYPE_LABELS: Record<string, string> = {
  CONFIG: "Config",
  NON_CONFIG: "Non-Config",
};

const SCOPE_LABELS: Record<string, string> = {
  COMMON: "Common",
  PRODUCT_SPECIFIC: "Product Specific",
};

const REQ_TYPE_OPTIONS = [
  { label: "Design", value: "DESIGN" },
  { label: "OE", value: "OE" },
];

const CTRL_TYPE_OPTIONS = [
  { label: "Config", value: "CONFIG" },
  { label: "Non-Config", value: "NON_CONFIG" },
];

const SCOPE_OPTIONS = [
  { label: "Common", value: "COMMON" },
  { label: "Product Specific", value: "PRODUCT_SPECIFIC" },
];

const STATUS_FILTER_OPTIONS: { label: string; value: string }[] = [
  { label: "Overdue", value: "OVERDUE" },
  ...(Object.keys(CONTROL_STATUS_LABELS) as ControlStatus[]).map((s) => ({
    label: CONTROL_STATUS_LABELS[s],
    value: s,
  })),
];

// The column catalogue (CONTROL_COLUMNS / DEFAULT_VISIBLE_CONTROL_COLUMNS /
// CONTROL_COLUMNS_STORAGE_KEY) lives in ./controlColumns so this component file
// only exports components (react-refresh/only-export-components).

// Reusable cell renderers
function userCell(name: string | null | undefined): JSX.Element {
  return name ? (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
      <UserAvatar name={name} size={26} />
      <Typography variant="body2" noWrap>{name}</Typography>
    </Box>
  ) : (
    <Typography variant="body2" color="text.disabled">—</Typography>
  );
}

function textCell(value: string | null | undefined): JSX.Element {
  return <Typography variant="body2" noWrap>{value ?? "—"}</Typography>;
}

function dateCell(date: string | null | undefined): JSX.Element {
  return date ? (
    <Typography variant="body2" noWrap>{formatAuditDate(date)}</Typography>
  ) : (
    <Typography variant="body2" color="text.secondary">—</Typography>
  );
}

// ── Column definitions ────────────────────────────────────────────────────────

interface ColumnDef {
  id: string;
  label: string;
  minWidth: number;
  sortField: string;
  filterKey?: string;
  filterOptions?: { label: string; value: string }[];
  searchableFilter?: boolean;
  alwaysVisible?: boolean;
  defaultHidden?: boolean;
  /** Greedy column — absorbs the table's leftover horizontal space. */
  grow?: boolean;
  render: (c: AuditControl) => ReactNode;
}

function applySorting(
  controls: AuditControl[],
  field: string,
  direction: ListingTableSortDirection,
): AuditControl[] {
  return [...controls].sort((a, b) => {
    const aVal = String((a as unknown as Record<string, unknown>)[field] ?? "");
    const bVal = String((b as unknown as Record<string, unknown>)[field] ?? "");
    const cmp = aVal.localeCompare(bVal);
    return direction === "asc" ? cmp : -cmp;
  });
}

// ── Props ────────────────────────────────────────────────────────────────────

interface ControlsTableProps {
  controls: AuditControl[];
  allControls: AuditControl[];
  filters: Record<string, string[]>;
  onFiltersChange: (f: Record<string, string[]>) => void;
  isLoading: boolean;
  onRowClick: (control: AuditControl) => void;
  /** Column ids to show. Owned by the parent so the picker can sit in the filter bar. */
  visibleColumnIds: string[];
}

// ── Component ────────────────────────────────────────────────────────────────

export default function ControlsTable({
  controls,
  allControls,
  filters,
  onFiltersChange,
  isLoading,
  onRowClick,
  visibleColumnIds,
}: ControlsTableProps): JSX.Element {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(50);
  const [sortField, setSortField] = useState<string | null>(null);
  const [sortDirection, setSortDirection] = useState<ListingTableSortDirection>("asc");

  // Derive unique options for the filter dropdowns from the full (unfiltered) list.
  const auditorOptions = [
    ...new Set(allControls.map((c) => c.auditorName).filter((n): n is string => n !== null)),
  ].sort().map((n) => ({ label: n, value: n }));

  const ownerOptions = [
    ...new Set(allControls.map((c) => c.ownerName).filter((n): n is string => n !== null)),
  ].sort().map((n) => ({ label: n, value: n }));

  const teamOptions = [
    ...new Set(allControls.map((c) => c.teamName).filter((n): n is string => n !== null)),
  ].sort().map((n) => ({ label: n, value: n }));

  // Column catalogue. New columns can be added here; population columns are
  // hidden by default so the row stays short until the user opts in.
  const columns: ColumnDef[] = [
    { id: "controlNumber", label: "Control No.", minWidth: 90, sortField: "controlNumber", alwaysVisible: true,
      render: (c) => <Typography variant="body2" fontWeight={600} noWrap>{c.controlNumber}</Typography> },
    { id: "description", label: "Description", minWidth: 240, sortField: "description", grow: true,
      render: (c) => (
        // Clamp to 2 lines to keep rows compact; the column grows to fill leftover
        // width (see `grow`), and full text is on hover + the row-click drawer.
        <Tooltip title={c.description ?? ""} arrow placement="top-start" slotProps={{ tooltip: { sx: { maxWidth: 460 } } }}>
          <Typography variant="body2" sx={{ display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
            {c.description}
          </Typography>
        </Tooltip>
      ) },
    { id: "requirementType", label: "Req. Type", minWidth: 110, sortField: "requirementType",
      filterKey: "requirementType", filterOptions: REQ_TYPE_OPTIONS,
      render: (c) => <Typography variant="body2" noWrap>{REQ_TYPE_LABELS[c.requirementType]}</Typography> },
    { id: "controlType", label: "Control Type", minWidth: 130, sortField: "controlType",
      filterKey: "controlType", filterOptions: CTRL_TYPE_OPTIONS,
      render: (c) => <Typography variant="body2" noWrap>{CTRL_TYPE_LABELS[c.controlType]}</Typography> },
    { id: "status", label: "Status", minWidth: 185, sortField: "status",
      filterKey: "status", filterOptions: STATUS_FILTER_OPTIONS, searchableFilter: true,
      render: (c) => <ControlStatusChip status={c.status} /> },
    { id: "auditorName", label: "Auditor POC", minWidth: 140, sortField: "auditorName",
      filterKey: "auditorName", filterOptions: auditorOptions, searchableFilter: true,
      render: (c) => userCell(c.auditorName) },
    { id: "ownerName", label: "Process Owner", minWidth: 150, sortField: "ownerName",
      filterKey: "ownerName", filterOptions: ownerOptions, searchableFilter: true,
      render: (c) => userCell(c.ownerName) },
    { id: "teamName", label: "Team", minWidth: 130, sortField: "teamName",
      filterKey: "teamName", filterOptions: teamOptions, searchableFilter: true,
      render: (c) => textCell(c.teamName) },
    { id: "scope", label: "Scope", minWidth: 130, sortField: "scope",
      filterKey: "scope", filterOptions: SCOPE_OPTIONS,
      render: (c) => <Typography variant="body2" noWrap>{SCOPE_LABELS[c.scope]}</Typography> },
    { id: "dueDate", label: "Due Date", minWidth: 110, sortField: "dueDate",
      render: (c) => c.dueDate ? (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Typography variant="body2" noWrap color={c.isOverdue ? "error.main" : "text.primary"} fontWeight={c.isOverdue ? 600 : 400}>
            {formatAuditDate(c.dueDate)}
          </Typography>
          {c.isOverdue && (
            <Tooltip title="Overdue">
              <AlertCircle size={14} color="var(--mui-palette-error-main, #d32f2f)" />
            </Tooltip>
          )}
        </Box>
      ) : (
        <Typography variant="body2" color="text.secondary">—</Typography>
      ) },
    // ── New population-phase columns (hidden by default) ──
    { id: "populationDueDate", label: "Population Due Date", minWidth: 150, sortField: "populationDueDate", defaultHidden: true,
      render: (c) => dateCell(c.populationDueDate) },
    { id: "populationOwnerName", label: "Population Owner", minWidth: 160, sortField: "populationOwnerName", defaultHidden: true,
      render: (c) => userCell(c.populationOwnerName) },
    { id: "populationTeamName", label: "Population Team", minWidth: 150, sortField: "populationTeamName", defaultHidden: true,
      render: (c) => textCell(c.populationTeamName) },
  ];

  // Visible columns, in catalogue order (alwaysVisible ones can't be hidden).
  // Visibility is controlled by the parent (AuditDetailPage owns the picker).
  const visibleColumns = columns.filter((c) => c.alwaysVisible || visibleColumnIds.includes(c.id));

  const sorted = sortField ? applySorting(controls, sortField, sortDirection) : controls;
  const totalPages = Math.ceil(sorted.length / rowsPerPage);
  const safePage = totalPages === 0 ? 0 : Math.min(page, totalPages - 1);
  if (safePage !== page) setPage(safePage);
  const displayed = sorted.slice(safePage * rowsPerPage, (safePage + 1) * rowsPerPage);

  function handleSortChange(field: string, direction: ListingTableSortDirection) {
    setSortField(field);
    setSortDirection(direction);
  }

  function setFilter(key: string, values: string[]) {
    onFiltersChange({ ...filters, [key]: values });
  }

  // ── Loading skeleton ──

  if (isLoading) {
    return (
      <ListingTable.Container>
        <ListingTable size="small">
          <ListingTable.Head>
            <ListingTable.Row>
              {visibleColumns.map((col) => (
                <ListingTable.Cell key={col.id} sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>
                  {col.label}
                </ListingTable.Cell>
              ))}
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {Array.from({ length: 5 }).map((_, i) => (
              <ListingTable.Row key={i}>
                {visibleColumns.map((col) => (
                  <ListingTable.Cell key={col.id}>
                    <Skeleton variant="text" width={col.id === "description" ? 200 : 80} />
                  </ListingTable.Cell>
                ))}
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  }

  // ── Table ──

  return (
    <ListingTable.Provider
        sortField={sortField ?? ""}
        sortDirection={sortDirection}
        onSortChange={handleSortChange}
      >
        <ListingTable.Container>
          <ListingTable size="small" stickyHeader>
            <ListingTable.Head>
              <ListingTable.Row>
                {visibleColumns.map((col) => (
                  <ListingTable.Cell key={col.id} sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: col.minWidth, width: col.grow ? "100%" : undefined }}>
                    {col.filterKey ? (
                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <ListingTable.SortLabel field={col.sortField}>{col.label}</ListingTable.SortLabel>
                        <ColumnFilter
                          label={col.label}
                          options={col.filterOptions ?? []}
                          selected={filters[col.filterKey] ?? []}
                          onChange={(v) => setFilter(col.filterKey as string, v)}
                          searchable={col.searchableFilter}
                        />
                      </Box>
                    ) : (
                      <ListingTable.SortLabel field={col.sortField}>{col.label}</ListingTable.SortLabel>
                    )}
                  </ListingTable.Cell>
                ))}
              </ListingTable.Row>
            </ListingTable.Head>

            <ListingTable.Body>
              {displayed.length === 0 ? (
                <ListingTable.Row>
                  <ListingTable.Cell colSpan={visibleColumns.length}>
                    <ListingTable.EmptyState
                      title="No controls match the selected filter."
                      minHeight={180}
                    />
                  </ListingTable.Cell>
                </ListingTable.Row>
              ) : (
                displayed.map((control) => (
                  <ListingTable.Row
                    key={control.id}
                    onClick={() => onRowClick(control)}
                    sx={{ cursor: "pointer", "&:hover": { bgcolor: "action.hover" } }}
                  >
                    {visibleColumns.map((col) => (
                      <ListingTable.Cell key={col.id} sx={{ width: col.grow ? "100%" : undefined }}>{col.render(control)}</ListingTable.Cell>
                    ))}
                  </ListingTable.Row>
                ))
              )}
            </ListingTable.Body>

            <ListingTable.Footer>
              <ListingTable.Row>
                <TablePagination
                  count={sorted.length}
                  page={safePage}
                  rowsPerPage={rowsPerPage}
                  rowsPerPageOptions={[25, 50, 100]}
                  onPageChange={(_, newPage) => setPage(newPage)}
                  onRowsPerPageChange={(e) => {
                    setRowsPerPage(parseInt(e.target.value, 10));
                    setPage(0);
                  }}
                />
              </ListingTable.Row>
            </ListingTable.Footer>
          </ListingTable>
        </ListingTable.Container>
      </ListingTable.Provider>
  );
}
