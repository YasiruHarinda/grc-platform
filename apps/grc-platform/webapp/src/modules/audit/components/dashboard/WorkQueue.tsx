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
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  FormControlLabel,
  IconButton,
  Popover,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowDown, ArrowUp, CheckCircle, Filter, Search, X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { ActionItem } from "@modules/audit/types/dashboard";
import { useGetWorkQueue, type WorkQueueTab, type DueSort } from "@modules/audit/api/useGetWorkQueue";
import { useGetTeams } from "@modules/audit/api/useGetTeams";
import { useGetUsers } from "@modules/audit/api/useGetUsers";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { dueInfo } from "./dueDate";

function actionLabel(status: string, canApprove: boolean): string {
  switch (status) {
    case "EVIDENCE_PENDING":              return "Submit evidence";
    case "SUBMITTED_SAMPLE":              return "Submit evidence for sample";
    case "EVIDENCE_NEED_CLARIFICATION":   return "Resubmit evidence";
    case "POPULATION_PENDING":            return "Submit population";
    case "POPULATION_NEED_CLARIFICATION": return "Resubmit population";
    case "EVIDENCE_INTERNAL_REVIEW":
    case "POPULATION_INTERNAL_REVIEW":
      return canApprove ? "Review & approve" : "Pending review";
    case "EVIDENCE_UNDER_VALIDATION":     return "Approve / request resubmission";
    case "POPULATION_UNDER_VALIDATION":   return "Approve / reject population";
    case "POPULATION_COMPLETE":           return "Submit sample";
    case "AWAITING_SAMPLE":               return "Submit sample";
    default:                              return "Action required";
  }
}

// Statuses offered in the Status / Action-needed column filters. COMPLETE is
// terminal and never surfaces in the work queue, so it is excluded.
const FILTERABLE_STATUSES = (Object.keys(CONTROL_STATUS_LABELS) as ControlStatus[]).filter(
  (s) => s !== "COMPLETE",
);

// Fixed status sets mirroring the backend's per-tab filters (audit_dashboard_repo.go),
// so the Status / Action-needed dropdowns never offer a status that can't appear on
// that tab.
const PENDING_STATUSES: ControlStatus[] = [
  "EVIDENCE_PENDING", "POPULATION_PENDING", "POPULATION_NEED_CLARIFICATION",
  "EVIDENCE_NEED_CLARIFICATION", "SUBMITTED_SAMPLE",
];
const VALIDATION_STATUSES: ControlStatus[] = [
  "EVIDENCE_UNDER_VALIDATION", "POPULATION_UNDER_VALIDATION", "POPULATION_COMPLETE", "AWAITING_SAMPLE",
];
const REVIEW_STATUSES: ControlStatus[] = ["EVIDENCE_INTERNAL_REVIEW", "POPULATION_INTERNAL_REVIEW"];

// statusesForTab returns the statuses that can actually appear on a given tab, so
// the column filters only ever list relevant options. Due Soon and Overdue span
// every non-terminal status; Action Items mirrors the backend's role-based filter
// (approximated here from the two privilege flags the frontend already has).
function statusesForTab(tab: WorkQueueTab, canApprove: boolean, canSubmit: boolean): ControlStatus[] {
  switch (tab) {
    case "pending":    return PENDING_STATUSES;
    case "validation": return VALIDATION_STATUSES;
    case "action-items":
      if (canApprove) return REVIEW_STATUSES;
      if (canSubmit) return PENDING_STATUSES;
      return VALIDATION_STATUSES;
    default: // due-soon, overdue
      return FILTERABLE_STATUSES;
  }
}

interface ActionGroup {
  label: string;
  statuses: ControlStatus[];
}

// buildActionGroups collapses the given statuses by their action label, so the
// "Action needed" filter offers the human-readable actions and resolves each back
// to its underlying statuses (some labels depend on canApprove).
function buildActionGroups(canApprove: boolean, statuses: ControlStatus[]): ActionGroup[] {
  const byLabel = new Map<string, ControlStatus[]>();
  for (const s of statuses) {
    const label = actionLabel(s, canApprove);
    const arr = byLabel.get(label) ?? [];
    arr.push(s);
    byLabel.set(label, arr);
  }
  return [...byLabel.entries()].map(([label, statuses]) => ({ label, statuses }));
}

// ── Column filter ─────────────────────────────────────────────────────────────

type FilterId = string | number;

interface FilterOption<T extends FilterId = number> {
  id: T;
  label: string;
}

interface ColFilterProps<T extends FilterId> {
  label: string;
  options: FilterOption<T>[];
  selected: T[];
  onChange: (v: T[]) => void;
}

function ColFilter<T extends FilterId>({ label, options, selected, onChange }: ColFilterProps<T>): JSX.Element {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");
  const isActive = selected.length > 0;
  const visible = query.trim()
    ? options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()))
    : options;

  function toggle(id: T) {
    onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  }

  return (
    <>
      <IconButton
        size="small"
        aria-label={`Filter by ${label}`}
        onClick={(e) => { e.stopPropagation(); setAnchor(e.currentTarget); }}
        sx={{
          ml: 0.25, p: 0.25, borderRadius: 0.75,
          color: isActive ? "primary.main" : "action.disabled",
          bgcolor: isActive ? "rgba(25,118,210,0.08)" : "transparent",
          "&:hover": { color: isActive ? "primary.main" : "text.secondary", bgcolor: isActive ? "rgba(25,118,210,0.12)" : "action.hover" },
        }}
      >
        <Filter size={12} />
      </IconButton>

      <Popover
        open={Boolean(anchor)}
        anchorEl={anchor}
        onClose={() => { setAnchor(null); setQuery(""); }}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
        slotProps={{ paper: { sx: { width: 230, borderRadius: 2, mt: 0.5 } } }}
        onClick={(e) => e.stopPropagation()}
      >
        <Box sx={{ p: 1.25 }}>
          <TextField
            size="small" fullWidth placeholder="Search..." value={query}
            onChange={(e) => setQuery(e.target.value)} autoFocus sx={{ mb: 0.75 }}
            slotProps={{
              input: {
                startAdornment: <Search size={14} style={{ marginRight: 4 }} />,
                endAdornment: query ? (
                  <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => setQuery("")}><X size={12} /></IconButton>
                ) : null,
              },
            }}
          />
          {isActive && (
            <Button size="small" onClick={() => onChange([])}
              sx={{ textTransform: "none", fontSize: "0.72rem", py: 0.25, mb: 0.5, display: "block" }}>
              Clear ({selected.length} selected)
            </Button>
          )}
          <Box sx={{ maxHeight: 260, overflowY: "auto" }}>
            {visible.length === 0 ? (
              <Typography variant="caption" color="text.secondary" sx={{ px: 1, py: 1, display: "block" }}>No matches</Typography>
            ) : visible.map((opt) => (
              <FormControlLabel
                key={opt.id}
                control={<Checkbox size="small" checked={selected.includes(opt.id)} onChange={() => toggle(opt.id)} disableRipple sx={{ p: 0.5 }} />}
                label={<Typography variant="body2" sx={{ fontSize: "0.82rem", lineHeight: 1.4 }}>{opt.label || "—"}</Typography>}
                sx={{ display: "flex", alignItems: "center", px: 0.5, py: 0.1, borderRadius: 1, mx: 0, width: "100%", "&:hover": { bgcolor: "action.hover" } }}
              />
            ))}
          </Box>
        </Box>
      </Popover>
    </>
  );
}

// TextColFilter is a header substring-search filter (used for Control No). The
// caller debounces the value before issuing a request.
function TextColFilter({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }): JSX.Element {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const isActive = value.trim().length > 0;

  return (
    <>
      <IconButton
        size="small"
        aria-label={`Filter by ${label}`}
        onClick={(e) => { e.stopPropagation(); setAnchor(e.currentTarget); }}
        sx={{
          ml: 0.25, p: 0.25, borderRadius: 0.75,
          color: isActive ? "primary.main" : "action.disabled",
          bgcolor: isActive ? "rgba(25,118,210,0.08)" : "transparent",
          "&:hover": { color: isActive ? "primary.main" : "text.secondary", bgcolor: isActive ? "rgba(25,118,210,0.12)" : "action.hover" },
        }}
      >
        <Filter size={12} />
      </IconButton>

      <Popover
        open={Boolean(anchor)}
        anchorEl={anchor}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
        slotProps={{ paper: { sx: { width: 230, borderRadius: 2, mt: 0.5 } } }}
        onClick={(e) => e.stopPropagation()}
      >
        <Box sx={{ p: 1.25 }}>
          <TextField
            size="small" fullWidth placeholder={`Search ${label}...`} value={value}
            onChange={(e) => onChange(e.target.value)} autoFocus
            slotProps={{
              input: {
                startAdornment: <Search size={14} style={{ marginRight: 4 }} />,
                endAdornment: value ? (
                  <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => onChange("")}><X size={12} /></IconButton>
                ) : null,
              },
            }}
          />
        </Box>
      </Popover>
    </>
  );
}

// ── Paginated tab panel ────────────────────────────────────────────────────────

interface TabPanelProps {
  tab: WorkQueueTab;
  canApprove: boolean;
  canSubmit: boolean;
  emptyText: string;
}

function TabPanel({ tab, canApprove, canSubmit, emptyText }: TabPanelProps): JSX.Element {
  const navigate = useNavigate();
  const [page, setPage] = useState(0); // 0-based for MUI, 1-based for API
  const [teamFilter, setTeamFilter] = useState<number[]>([]);
  const [ownerFilter, setOwnerFilter] = useState<number[]>([]);
  const [auditFilter, setAuditFilter] = useState<number[]>([]);
  const [statusFilter, setStatusFilter] = useState<string[]>([]);
  const [actionFilter, setActionFilter] = useState<string[]>([]);
  const [controlInput, setControlInput] = useState(""); // immediate text-box value
  const [controlNumber, setControlNumber] = useState(""); // debounced value sent to API
  const [dueSort, setDueSort] = useState<DueSort>("asc");

  // Debounce the control-number box so we don't issue a request per keystroke.
  useEffect(() => {
    const t = setTimeout(() => { setControlNumber(controlInput); setPage(0); }, 350);
    return () => clearTimeout(t);
  }, [controlInput]);

  // Statuses that can actually appear on this tab, so the filter dropdowns below
  // don't offer options that will only ever return zero rows.
  const tabStatuses = useMemo(() => statusesForTab(tab, canApprove, canSubmit), [tab, canApprove, canSubmit]);

  // Map the selected action labels back to their statuses and merge with the raw
  // status filter — both column filters constrain the same status column (union).
  const actionGroups = useMemo(() => buildActionGroups(canApprove, tabStatuses), [canApprove, tabStatuses]);
  const effectiveStatuses = useMemo(() => {
    const set = new Set<string>(statusFilter);
    for (const label of actionFilter) {
      actionGroups.find((g) => g.label === label)?.statuses.forEach((s) => set.add(s));
    }
    return [...set];
  }, [statusFilter, actionFilter, actionGroups]);

  const { data, isLoading, isError } = useGetWorkQueue(tab, page + 1, {
    teamIds: teamFilter, ownerIds: ownerFilter, auditIds: auditFilter,
    statuses: effectiveStatuses, controlNumber, dueSort,
  });
  const { data: teamsData } = useGetTeams();
  const { data: usersData } = useGetUsers();
  const { data: auditsData } = useGetAudits();

  const items: ActionItem[] = useMemo(() => data?.items ?? [], [data]);
  const total = data?.total ?? 0;
  const limit = data?.limit ?? 25;

  // Source filter options from the full unfiltered lists so all values are
  // selectable regardless of which page is currently displayed.
  const teams: FilterOption<number>[] = (teamsData ?? [])
    .map((t) => ({ id: t.id, label: t.name }))
    .sort((a, b) => a.label.localeCompare(b.label));

  // Process-owner filter options: union of the users list and the owners actually
  // present in the loaded queue rows. Deriving from the rows means the filter still
  // works even when /audit/users is empty or a user isn't flagged INTERNAL.
  const owners: FilterOption<number>[] = useMemo(() => {
    const byId = new Map<number, string>();
    (usersData ?? []).forEach((u) => byId.set(u.id, u.displayName));
    items.forEach((it) => {
      if (it.ownerId != null && !byId.has(it.ownerId)) {
        byId.set(it.ownerId, it.processOwner || `#${it.ownerId}`);
      }
    });
    return [...byId.entries()]
      .map(([id, label]) => ({ id, label }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [usersData, items]);

  const audits: FilterOption<number>[] = (auditsData?.items ?? [])
    .map((a) => ({ id: a.id, label: a.name }))
    .sort((a, b) => a.label.localeCompare(b.label));

  const statusOptions: FilterOption<string>[] = tabStatuses.map((s) => ({ id: s, label: CONTROL_STATUS_LABELS[s] }));
  const actionOptions: FilterOption<string>[] = actionGroups.map((g) => ({ id: g.label, label: g.label }));

  const hasFilters =
    teamFilter.length > 0 || ownerFilter.length > 0 || auditFilter.length > 0 ||
    statusFilter.length > 0 || actionFilter.length > 0 || controlNumber.trim().length > 0;

  if (isLoading) {
    return (
      <Box sx={{ py: 4, display: "flex", justifyContent: "center" }}>
        <CircularProgress size={28} />
      </Box>
    );
  }

  if (isError) {
    return <Alert severity="error" sx={{ m: 1 }}>Failed to load items. Please refresh.</Alert>;
  }

  if (total === 0 && !hasFilters) {
    return (
      <Box sx={{ py: 4, textAlign: "center" }}>
        <CheckCircle size={32} color="#43A047" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{emptyText}</Typography>
      </Box>
    );
  }

  return (
    <Box>
      {hasFilters && (
        <Box sx={{ px: 0.5, pb: 1, display: "flex", flexWrap: "wrap", gap: 0.5, alignItems: "center" }}>
          {teamFilter.map((id) => (
            <Chip key={id} label={teams.find((t) => t.id === id)?.label ?? String(id)} size="small" onDelete={() => { setTeamFilter((p) => p.filter((x) => x !== id)); setPage(0); }} />
          ))}
          {ownerFilter.map((id) => (
            <Chip key={id} label={owners.find((o) => o.id === id)?.label ?? String(id)} size="small" onDelete={() => { setOwnerFilter((p) => p.filter((x) => x !== id)); setPage(0); }} />
          ))}
          {auditFilter.map((id) => (
            <Chip key={`a${id}`} label={audits.find((a) => a.id === id)?.label ?? String(id)} size="small" onDelete={() => { setAuditFilter((p) => p.filter((x) => x !== id)); setPage(0); }} />
          ))}
          {statusFilter.map((s) => (
            <Chip key={`s${s}`} label={CONTROL_STATUS_LABELS[s as ControlStatus] ?? s} size="small" onDelete={() => { setStatusFilter((p) => p.filter((x) => x !== s)); setPage(0); }} />
          ))}
          {actionFilter.map((a) => (
            <Chip key={`ac${a}`} label={a} size="small" onDelete={() => { setActionFilter((p) => p.filter((x) => x !== a)); setPage(0); }} />
          ))}
          {controlNumber.trim() && (
            <Chip label={`Control: ${controlNumber.trim()}`} size="small" onDelete={() => { setControlInput(""); setControlNumber(""); setPage(0); }} />
          )}
          <Button size="small" onClick={() => { setTeamFilter([]); setOwnerFilter([]); setAuditFilter([]); setStatusFilter([]); setActionFilter([]); setControlInput(""); setControlNumber(""); setPage(0); }}
            sx={{ textTransform: "none", fontSize: "0.75rem", py: 0.25 }}>
            Clear all
          </Button>
        </Box>
      )}

      {total === 0 ? (
        <Box sx={{ py: 4, textAlign: "center" }}>
          <Typography variant="body2" color="text.secondary">No matches for the current filters.</Typography>
        </Box>
      ) : null}

      <TableContainer sx={{ display: total === 0 ? "none" : undefined }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Control
                  <TextColFilter label="Control No" value={controlInput} onChange={setControlInput} />
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Audit
                  <ColFilter label="Audit" options={audits} selected={auditFilter} onChange={(v) => { setAuditFilter(v); setPage(0); }} />
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Action needed
                  <ColFilter label="Action needed" options={actionOptions} selected={actionFilter} onChange={(v) => { setActionFilter(v); setPage(0); }} />
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Status
                  <ColFilter label="Status" options={statusOptions} selected={statusFilter} onChange={(v) => { setStatusFilter(v); setPage(0); }} />
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Due date
                  <IconButton
                    size="small"
                    aria-label={dueSort === "asc" ? "Sort due date descending" : "Sort due date ascending"}
                    onClick={(e) => { e.stopPropagation(); setDueSort((s) => (s === "asc" ? "desc" : "asc")); setPage(0); }}
                    sx={{ ml: 0.25, p: 0.25, borderRadius: 0.75, color: "primary.main", "&:hover": { bgcolor: "action.hover" } }}
                  >
                    {dueSort === "asc" ? <ArrowUp size={13} /> : <ArrowDown size={13} />}
                  </IconButton>
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Team
                  <ColFilter label="Team" options={teams} selected={teamFilter} onChange={(v) => { setTeamFilter(v); setPage(0); }} />
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Process Owner
                  <ColFilter label="Process Owner" options={owners} selected={ownerFilter} onChange={(v) => { setOwnerFilter(v); setPage(0); }} />
                </Box>
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {items.map((item) => {
              const due = dueInfo(item.dueDate);
              return (
                <TableRow
                  key={item.controlId}
                  hover tabIndex={0} sx={{ cursor: "pointer" }}
                  onClick={() => void navigate(`/audit/audits/${item.auditId}?control=${item.controlId}`)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      void navigate(`/audit/audits/${item.auditId}?control=${item.controlId}`);
                    }
                  }}
                >
                  <TableCell sx={{ whiteSpace: "nowrap", fontWeight: 600 }}>{item.controlNumber}</TableCell>
                  <TableCell sx={{ maxWidth: 180 }}>
                    <Typography variant="body2" noWrap title={item.auditName}>{item.auditName}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" color="primary.main">{actionLabel(item.status, canApprove)}</Typography>
                  </TableCell>
                  <TableCell>
                    <ControlStatusChip status={item.status as ControlStatus} />
                  </TableCell>
                  <TableCell sx={{ whiteSpace: "nowrap" }}>
                    <Typography variant="body2" sx={{ color: due.color, fontWeight: due.sortKey <= 3 ? 600 : 400 }}>
                      {due.label}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" noWrap>{item.team || "—"}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" noWrap>{item.processOwner || "—"}</Typography>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>

      {total > 0 && (
        <TablePagination
          component="div"
          count={total}
          page={page}
          onPageChange={(_, p) => setPage(p)}
          rowsPerPage={limit}
          rowsPerPageOptions={[limit]}
          sx={{ borderTop: 1, borderColor: "divider" }}
        />
      )}
    </Box>
  );
}

// ── WorkQueue ─────────────────────────────────────────────────────────────────

export const QUEUE_TAB_AWAITING = 0;
export const QUEUE_TAB_DUE_SOON = 1;
export const QUEUE_TAB_PENDING = 2;
export const QUEUE_TAB_VALIDATION = 3;
export const QUEUE_TAB_OVERDUE = 4;

interface WorkQueueProps {
  totalActionItems: number;
  totalDueSoonItems: number;
  totalPendingItems: number;
  totalValidationItems: number;
  totalOverdueControls: number;
  // canViewAll (AUDIT_VIEW_ALL_AUDITS) sees the full tab set; a submitter without
  // it sees only Pending Submission; an auditor without it only Under Validation.
  canViewAll: boolean;
  canApprove: boolean;
  canSubmit: boolean;
  canValidate: boolean;
  queueTitle: string;
  tab: number;
  onTabChange: (tab: number) => void;
}

interface QueueTabDef {
  value: number;
  label: string;
  tabKey: WorkQueueTab;
  emptyText: string;
  sx?: object;
}

export default function WorkQueue({
  totalActionItems, totalDueSoonItems, totalPendingItems, totalValidationItems, totalOverdueControls,
  canViewAll, canApprove, canSubmit, canValidate, queueTitle, tab, onTabChange,
}: WorkQueueProps): JSX.Element {
  const allTabs: QueueTabDef[] = [
    { value: QUEUE_TAB_AWAITING, label: `${queueTitle} (${totalActionItems})`, tabKey: "action-items", emptyText: "No pending actions" },
    { value: QUEUE_TAB_DUE_SOON, label: `Due Soon (${totalDueSoonItems})`, tabKey: "due-soon", emptyText: "Nothing due in the next 7 days" },
    { value: QUEUE_TAB_PENDING, label: `Pending Submission (${totalPendingItems})`, tabKey: "pending", emptyText: "Nothing pending submission or clarification" },
    { value: QUEUE_TAB_VALIDATION, label: `Under Validation (${totalValidationItems})`, tabKey: "validation", emptyText: "Nothing under validation" },
    {
      value: QUEUE_TAB_OVERDUE, label: `Overdue (${totalOverdueControls})`, tabKey: "overdue", emptyText: "No overdue controls",
      sx: totalOverdueControls > 0 ? { color: "#E53935", "&.Mui-selected": { color: "#E53935" } } : undefined,
    },
  ];

  // Privilege-driven tab visibility (ADR-0002): org-wide readers get everything;
  // a submitter sees only their Pending Submission queue; an auditor only Under
  // Validation. Each Tab carries an explicit `value` so hidden tabs don't shift
  // the selected index.
  const visibleTabs = canViewAll
    ? allTabs
    : canSubmit
      ? allTabs.filter((t) => t.value === QUEUE_TAB_PENDING)
      : canValidate
        ? allTabs.filter((t) => t.value === QUEUE_TAB_VALIDATION)
        : [];

  if (visibleTabs.length === 0) {
    return (
      <Box sx={{ py: 4, textAlign: "center" }}>
        <Typography variant="body2" color="text.secondary">No work items for your role.</Typography>
      </Box>
    );
  }

  // The default landing tab may be hidden for narrow roles — fall back to the
  // first visible tab.
  const effectiveTab = visibleTabs.some((t) => t.value === tab) ? tab : visibleTabs[0].value;
  const active = visibleTabs.find((t) => t.value === effectiveTab) ?? visibleTabs[0];

  return (
    <Box>
      <Tabs
        value={effectiveTab}
        onChange={(_, v: number) => onTabChange(v)}
        sx={{ borderBottom: 1, borderColor: "divider", minHeight: 40, "& .MuiTab-root": { minHeight: 40, textTransform: "none", fontWeight: 600 } }}
      >
        {visibleTabs.map((t) => (
          <Tab key={t.value} value={t.value} label={t.label} sx={t.sx} />
        ))}
      </Tabs>
      <Box sx={{ pt: 1 }}>
        <TabPanel tab={active.tabKey} canApprove={canApprove} canSubmit={canSubmit} emptyText={active.emptyText} />
      </Box>
    </Box>
  );
}
