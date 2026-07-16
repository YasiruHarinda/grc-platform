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

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControl,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem,
  PageContent,
  PageTitle,
  Paper,
  Select,
  Stack,
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
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Eye, RefreshCw, Search, X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import type * as React from "react";
import {
  approveRisk,
  cancelRisk,
  closeRisk,
  completeRisk,
  createAssessment,
  fetchAssignmentTeams,
  fetchComplianceReferences,
  fetchRiskDetail,
  fetchRisks,
  fetchRiskScores,
  fetchSourceRegisterTeams,
  fetchUsers,
  managementApproveRisk,
  ownerApproveRisk,
  rejectRisk,
  resubmitRisk,
  updateRisk,
} from "../api/riskApi";
import type {
  ComplianceReference,
  RiskDetail,
  RiskListItem,
  RiskScore,
  RiskTeam,
  UpdateRiskPayload,
  UserOption,
} from "../api/riskApi";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { useRiskPrivileges } from "../hooks/useRiskPrivileges";
import RiskDetailDrawer from "./risk-registers/RiskDetailDrawer";
import RejectDialog from "./risk-registers/RejectDialog";
import ReassessmentDialog from "./risk-registers/ReassessmentDialog";
import EditRiskDialog from "./risk-registers/EditRiskDialog";
import ColumnFilter from "./risk-registers/ColumnFilter";
import DateRangeFilter from "./risk-registers/DateRangeFilter";
import {
  APPROVED_ALL_STATUSES,
  PENDING_COMPLIANCE_STATUSES,
  PENDING_MANAGEMENT_STATUSES,
  PENDING_OWNER_STATUSES,
  PENDING_REVISION_STATUSES,
  STATUS_CONFIG,
  calcDue,
  formatDate,
} from "./risk-registers/utils";

// ── Tab definitions ────────────────────────────────────────────────────────────

type TabKey = "approved" | "pending-owner" | "pending-management" | "pending-compliance" | "pending-revision";

interface TabDef {
  key: TabKey;
  label: string;
  statuses: string[];
  showRiskType: boolean;
}

const TABS: TabDef[] = [
  { key: "approved",            label: "Approved Risks",              statuses: APPROVED_ALL_STATUSES,        showRiskType: false },
  { key: "pending-owner",       label: "Pending Risk Owner Approval", statuses: PENDING_OWNER_STATUSES,       showRiskType: true },
  { key: "pending-management",  label: "Pending Management Approval", statuses: PENDING_MANAGEMENT_STATUSES,  showRiskType: true },
  { key: "pending-compliance",  label: "Pending Compliance Approval", statuses: PENDING_COMPLIANCE_STATUSES,  showRiskType: true },
  { key: "pending-revision",    label: "Pending Revision",            statuses: PENDING_REVISION_STATUSES,    showRiskType: true },
];

// ── Chips ──────────────────────────────────────────────────────────────────────

// Matches OutlinedStatusChip's displayed text, so the Status column filter's
// checkbox labels read the same as what's actually shown in that column.
function statusLabel(status: string): string {
  if (status === "IN_REMEDIATION") return "Open";
  if (status === "CLOSED") return "Closed";
  return STATUS_CONFIG[status]?.label ?? status;
}

function OutlinedStatusChip({ status }: { status: string }): JSX.Element {
  if (status === "IN_REMEDIATION") return <Chip label="Open" color="info" size="small" variant="outlined" />;
  if (status === "CLOSED") return <Chip label="Closed" color="success" size="small" variant="outlined" />;
  const cfg = STATUS_CONFIG[status] ?? { label: status, color: "default" as const };
  return <Chip label={cfg.label} color={cfg.color} size="small" variant="outlined" />;
}

function LevelChip({ level, color }: { level: string; color: string }): JSX.Element {
  if (!level) return <Typography variant="body2">—</Typography>;
  return (
    <Chip
      label={level}
      size="small"
      sx={{ bgcolor: color || undefined, color: color ? "#fff" : undefined, fontWeight: 700 }}
    />
  );
}

const RISK_CLOSURE_STATUSES = new Set([
  "PENDING_OWNER_COMPLETION_APPROVAL",
  "PENDING_COMPLIANCE_CLOSURE",
]);

function RiskTypeChip({ riskType, workflowStatus, rejectionStage }: { riskType: string; workflowStatus: string; rejectionStage?: string | null }): JSX.Element {
  const isRiskClosure =
    RISK_CLOSURE_STATUSES.has(workflowStatus) ||
    (workflowStatus === "PENDING_REVISION" && rejectionStage === "COMPLETION_OWNER");
  if (isRiskClosure) {
    return <Chip label="Risk Closure" color="success" size="small" variant="outlined" />;
  }
  return (
    <Chip
      label={riskType === "UPDATED" ? "Updated Risk" : "New Risk"}
      color={riskType === "UPDATED" ? "warning" : "info"}
      size="small"
      variant="outlined"
    />
  );
}

// ── Filters ────────────────────────────────────────────────────────────────────

// Every multi-value field is shared between the top FilterBar (which writes
// a single-element array through its classic single-select UI) and the new
// per-column checkbox filters (which can add more values to the same array)
// — matching the Audit module's FilterPanel + ColumnFilter precedent, where
// both surfaces read/write one shared filter state.
interface Filters {
  search: string;
  teamId: number[];
  level: string[];
  status: string[];
  riskType: string[];
  ownerId: number[];
  submittedFrom: string;
  submittedTo: string;
  dueFrom: string;
  dueTo: string;
  dueOverdueOnly: boolean;
}

const EMPTY_FILTERS: Filters = {
  search: "",
  teamId: [],
  level: [],
  status: [],
  riskType: [],
  ownerId: [],
  submittedFrom: "",
  submittedTo: "",
  dueFrom: "",
  dueTo: "",
  dueOverdueOnly: false,
};

function FilterBar({
  filters,
  teams,
  showApprovedFilter,
  approvedFilter,
  onApprovedFilterChange,
  showRiskTypeFilter,
  onChange,
  onRefresh,
}: {
  filters: Filters;
  teams: RiskTeam[];
  showApprovedFilter: boolean;
  approvedFilter: "" | "open" | "closed";
  onApprovedFilterChange: (val: "" | "open" | "closed") => void;
  showRiskTypeFilter: boolean;
  onChange: (f: Filters) => void;
  onRefresh: () => void;
}): JSX.Element {
  return (
    <Stack direction="row" gap={1.5} flexWrap="wrap" alignItems="center">
      <TextField
        size="small"
        placeholder="Search risk code or title..."
        value={filters.search}
        onChange={(e) => onChange({ ...filters, search: e.target.value })}
        sx={{ minWidth: 240 }}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <Search size={16} />
            </InputAdornment>
          ),
          endAdornment: filters.search ? (
            <InputAdornment position="end">
              <IconButton size="small" aria-label="Clear search" onClick={() => onChange({ ...filters, search: "" })}>
                <X size={14} />
              </IconButton>
            </InputAdornment>
          ) : undefined,
        }}
      />

      <FormControl size="small" sx={{ minWidth: 160 }}>
        <InputLabel>Register</InputLabel>
        <Select
          label="Register"
          value={filters.teamId[0] ?? ""}
          onChange={(e) => onChange({ ...filters, teamId: e.target.value ? [Number(e.target.value)] : [] })}
        >
          <MenuItem value="">All Registers</MenuItem>
          {teams.map((t) => (
            <MenuItem key={t.id} value={t.id}>
              {t.name}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <FormControl size="small" sx={{ minWidth: 130 }}>
        <InputLabel>Level</InputLabel>
        <Select
          label="Level"
          value={filters.level[0] ?? ""}
          onChange={(e) => onChange({ ...filters, level: e.target.value ? [e.target.value as string] : [] })}
        >
          <MenuItem value="">All Levels</MenuItem>
          <MenuItem value="LOW">Low</MenuItem>
          <MenuItem value="MEDIUM">Medium</MenuItem>
          <MenuItem value="HIGH">High</MenuItem>
        </Select>
      </FormControl>

      {showRiskTypeFilter && (
        <FormControl size="small" sx={{ minWidth: 150 }}>
          <InputLabel>Risk Type</InputLabel>
          <Select
            label="Risk Type"
            value={filters.riskType[0] ?? ""}
            onChange={(e) => onChange({ ...filters, riskType: e.target.value ? [e.target.value as string] : [] })}
          >
            <MenuItem value="">All Types</MenuItem>
            <MenuItem value="NEW">New Risk</MenuItem>
            <MenuItem value="UPDATED">Updated Risk</MenuItem>
          </Select>
        </FormControl>
      )}

      {showApprovedFilter && (
        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Status</InputLabel>
          <Select
            label="Status"
            value={approvedFilter}
            onChange={(e) => onApprovedFilterChange(e.target.value as "" | "open" | "closed")}
          >
            <MenuItem value="">All Risks</MenuItem>
            <MenuItem value="open">Open</MenuItem>
            <MenuItem value="closed">Closed</MenuItem>
          </Select>
        </FormControl>
      )}

      <Tooltip title="Refresh">
        <IconButton size="small" aria-label="Refresh list" onClick={onRefresh}>
          <RefreshCw size={16} />
        </IconButton>
      </Tooltip>
    </Stack>
  );
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function RiskRegisters(): JSX.Element {
  const authFetch = useAuthApiClient();
  const { can } = useRiskPrivileges();

  const [activeTab, setActiveTab] = useState<TabKey>("approved");
  const [approvedFilter, setApprovedFilter] = useState<"" | "open" | "closed">("");
  const [risks, setRisks] = useState<RiskListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(50);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState("");

  const [sourceTeams, setSourceTeams] = useState<RiskTeam[]>([]);
  const [assignmentTeams, setAssignmentTeams] = useState<RiskTeam[]>([]);
  const [riskScores, setRiskScores] = useState<RiskScore[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [complianceRefs, setComplianceRefs] = useState<ComplianceReference[]>([]);

  const [filters, setFilters] = useState<Filters>(EMPTY_FILTERS);

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerDetail, setDrawerDetail] = useState<RiskDetail | null>(null);
  const [drawerLoading, setDrawerLoading] = useState(false);
  const [drawerError, setDrawerError] = useState("");

  const [editDetail, setEditDetail] = useState<RiskDetail | null>(null);
  const [assessOpen, setAssessOpen] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false);

  const [actionError, setActionError] = useState("");
  const [actionSuccess, setActionSuccess] = useState("");
  const [actionInFlight, setActionInFlight] = useState(false);

  const loadSeqRef = useRef(0);

  const activeTabDef = TABS.find((t) => t.key === activeTab)!;

  // The tab (and, within "Approved Risks", the approvedFilter select) already
  // scope which statuses are in play; the Status column filter narrows that
  // further (AND), same as every other column filter.
  const getStatuses = useCallback((): string[] => {
    const tabStatuses = (() => {
      if (activeTab === "approved") {
        if (approvedFilter === "open") return ["IN_REMEDIATION"];
        if (approvedFilter === "closed") return ["CLOSED"];
        return APPROVED_ALL_STATUSES;
      }
      return activeTabDef.statuses;
    })();
    if (filters.status.length === 0) return tabStatuses;
    return tabStatuses.filter((s) => filters.status.includes(s));
  }, [activeTab, activeTabDef.statuses, approvedFilter, filters.status]);

  // Full pool of statuses the Status column filter can offer, independent of
  // approvedFilter's own narrowing — so e.g. "Closed" is always selectable
  // even while approvedFilter is currently set to "Open".
  const statusOptions = activeTab === "approved" ? APPROVED_ALL_STATUSES : activeTabDef.statuses;

  function setColumnFilter<K extends keyof Filters>(key: K, value: Filters[K]) {
    setFilters((prev) => ({ ...prev, [key]: value }));
  }

  const registerOptions = sourceTeams.map((t) => ({ label: t.name, value: String(t.id) }));
  const ownerOptions = users.map((u) => ({ label: u.display_name, value: String(u.id) }));
  const levelOptions = [
    { label: "Low", value: "LOW" },
    { label: "Medium", value: "MEDIUM" },
    { label: "High", value: "HIGH" },
  ];
  const riskTypeOptions = [
    { label: "New Risk", value: "NEW" },
    { label: "Updated Risk", value: "UPDATED" },
  ];
  const statusColumnOptions = statusOptions.map((s) => ({ label: statusLabel(s), value: s }));

  useEffect(() => {
    fetchSourceRegisterTeams(authFetch).then(setSourceTeams).catch(console.error);
    fetchAssignmentTeams(authFetch).then(setAssignmentTeams).catch(console.error);
    fetchRiskScores(authFetch).then(setRiskScores).catch(console.error);
    fetchUsers(authFetch).then(setUsers).catch(console.error);
    fetchComplianceReferences(authFetch).then(setComplianceRefs).catch(console.error);
  }, []);

  // Reset to page 0 whenever the tab or filters change so the user never lands
  // on a page that doesn't exist for the new result set.
  useEffect(() => {
    setPage(0);
  }, [activeTab, filters]);

  const loadRisks = useCallback(async () => {
    const seq = ++loadSeqRef.current;
    const statuses = getStatuses();
    // getStatuses() returns [] both when no Status filter is selected (no
    // restriction) and when the selected Status values are disjoint from the
    // current tab/approvedFilter scope (genuinely zero matches). fetchRisks
    // treats an empty array as "no filter" and omits the statuses param
    // entirely, which the backend then reads as "no restriction" — silently
    // returning risks from every workflow status instead of none. Short-
    // circuit the disjoint case here rather than hitting the server.
    if (statuses.length === 0 && filters.status.length > 0) {
      setRisks([]);
      setTotal(0);
      setListError("");
      setLoading(false);
      return;
    }
    setLoading(true);
    setListError("");
    try {
      const result = await fetchRisks(authFetch, {
        statuses,
        team_id: filters.teamId.length ? filters.teamId : undefined,
        level: filters.level.length ? filters.level : undefined,
        search: filters.search || undefined,
        risk_type: filters.riskType.length ? filters.riskType : undefined,
        owner_id: filters.ownerId.length ? filters.ownerId : undefined,
        submitted_from: filters.submittedFrom || undefined,
        submitted_to: filters.submittedTo || undefined,
        due_from: filters.dueFrom || undefined,
        due_to: filters.dueTo || undefined,
        due_overdue: filters.dueOverdueOnly || undefined,
        offset: page * rowsPerPage,
        limit: rowsPerPage,
      });
      if (seq !== loadSeqRef.current) return;
      setRisks(result.items ?? []);
      setTotal(result.total);
    } catch (e: unknown) {
      if (seq !== loadSeqRef.current) return;
      setListError(e instanceof Error ? e.message : "Failed to load risks.");
    } finally {
      if (seq === loadSeqRef.current) setLoading(false);
    }
  }, [activeTab, filters, getStatuses, page, rowsPerPage]);

  useEffect(() => {
    loadRisks();
  }, [loadRisks]);

  const openDrawer = async (id: number) => {
    setDrawerOpen(true);
    setDrawerDetail(null);
    setDrawerError("");
    setDrawerLoading(true);
    try {
      const detail = await fetchRiskDetail(authFetch, id);
      setDrawerDetail(detail);
    } catch (e: unknown) {
      setDrawerError(e instanceof Error ? e.message : "Failed to load risk details.");
    } finally {
      setDrawerLoading(false);
    }
  };

  const closeDrawer = () => {
    setDrawerOpen(false);
    setDrawerDetail(null);
    setDrawerError("");
  };

  const runAction = async (fn: () => Promise<void>, successMsg: string) => {
    if (actionInFlight) return;
    setActionInFlight(true);
    setActionError("");
    setActionSuccess("");
    try {
      await fn();
      setActionSuccess(successMsg);
      closeDrawer();
      loadRisks();
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "Action failed.");
    } finally {
      setActionInFlight(false);
    }
  };

  const drawerActions = {
    onOwnerApprove: () =>
      runAction(
        () => ownerApproveRisk(authFetch, drawerDetail!.id),
        "Owner approved.",
      ),

    onManagementApprove: () =>
      runAction(
        () => managementApproveRisk(authFetch, drawerDetail!.id),
        "Management approved. Risk moved to Pending Compliance Approval.",
      ),

    onApprove: () =>
      runAction(
        () => approveRisk(authFetch, drawerDetail!.id),
        "Risk approved. It has been moved to Approved Risks.",
      ),

    onReject: () => setRejectOpen(true),

    onComplete: () =>
      runAction(
        () => completeRisk(authFetch, drawerDetail!.id),
        "Risk submitted for completion approval by risk owner.",
      ),

    onResubmit: () =>
      runAction(
        () => resubmitRisk(authFetch, drawerDetail!.id),
        "Risk resubmitted and sent for owner approval.",
      ),

    onCloseRisk: () =>
      runAction(
        () => closeRisk(authFetch, drawerDetail!.id),
        "Risk closed successfully.",
      ),

    onEdit: () => setEditDetail(drawerDetail),
    onAssess: () => setAssessOpen(true),
    onCancel: () => setCancelConfirmOpen(true),
  };

  const handleRejectConfirm = async (comment: string) => {
    if (!drawerDetail) return;
    await rejectRisk(authFetch, drawerDetail.id, comment);
    setActionSuccess("Risk rejected and returned for revision.");
    closeDrawer();
    loadRisks();
  };

  const handleEditSave = async (payload: UpdateRiskPayload) => {
    if (!editDetail) return;
    const savedId = editDetail.id;
    const isPendingRevision = editDetail.workflow_status === "PENDING_REVISION";
    await updateRisk(authFetch, savedId, payload);
    setEditDetail(null);
    loadRisks();
    if (isPendingRevision) {
      openDrawer(savedId);
    } else {
      const willMove = editDetail.workflow_status === "IN_REMEDIATION";
      setActionSuccess(
        willMove
          ? "Risk updated. It has been moved for Risk Owner Approval."
          : "Risk updated successfully.",
      );
      closeDrawer();
    }
  };

  const handleAssessSubmit = async (payload: Parameters<typeof createAssessment>[2]) => {
    if (!drawerDetail) return;
    await createAssessment(authFetch, drawerDetail.id, payload);
    setActionSuccess("Assessment saved.");
    setAssessOpen(false);
    openDrawer(drawerDetail.id);
    loadRisks();
  };

  const handleTabChange = (_: React.SyntheticEvent, val: TabKey) => {
    setActiveTab(val);
    setFilters(EMPTY_FILTERS);
    setApprovedFilter("");
  };

  const showStatusCol = activeTab === "approved";
  const showRiskTypeCol = activeTabDef.showRiskType;
  const colSpan = 8 + (showStatusCol ? 1 : 0) + (showRiskTypeCol ? 1 : 0);

  return (
    <PageContent>
      <PageTitle>
        <PageTitle.Header>Risk Registers</PageTitle.Header>
      </PageTitle>

      <Box sx={{ borderBottom: 1, borderColor: "divider", mb: 3 }}>
        <Tabs value={activeTab} onChange={handleTabChange}>
          {TABS.map((tab) => (
            <Tab key={tab.key} label={tab.label} value={tab.key} />
          ))}
        </Tabs>
      </Box>

      <Box sx={{ mb: 2 }}>
        <FilterBar
          filters={filters}
          teams={sourceTeams}
          showApprovedFilter={activeTab === "approved"}
          approvedFilter={approvedFilter}
          onApprovedFilterChange={setApprovedFilter}
          showRiskTypeFilter={activeTabDef.showRiskType}
          onChange={setFilters}
          onRefresh={loadRisks}
        />
      </Box>

      {actionError && (
        <Alert severity="error" onClose={() => setActionError("")} sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}
      {actionSuccess && (
        <Alert severity="success" onClose={() => setActionSuccess("")} sx={{ mb: 2 }}>
          {actionSuccess}
        </Alert>
      )}

      <Paper variant="outlined">
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 700 }}>Risk Code</TableCell>
                <TableCell sx={{ fontWeight: 700 }}>Title</TableCell>
                <TableCell sx={{ fontWeight: 700 }}>
                  <Box sx={{ display: "flex", alignItems: "center" }}>
                    Register
                    <ColumnFilter
                      label="Register"
                      options={registerOptions}
                      selected={filters.teamId.map(String)}
                      onChange={(v) => setColumnFilter("teamId", v.map(Number))}
                      searchable
                    />
                  </Box>
                </TableCell>
                <TableCell sx={{ fontWeight: 700 }}>
                  <Box sx={{ display: "flex", alignItems: "center" }}>
                    Level
                    <ColumnFilter
                      label="Level"
                      options={levelOptions}
                      selected={filters.level}
                      onChange={(v) => setColumnFilter("level", v)}
                    />
                  </Box>
                </TableCell>
                <TableCell sx={{ fontWeight: 700 }}>
                  <Box sx={{ display: "flex", alignItems: "center" }}>
                    Owner
                    <ColumnFilter
                      label="Owner"
                      options={ownerOptions}
                      selected={filters.ownerId.map(String)}
                      onChange={(v) => setColumnFilter("ownerId", v.map(Number))}
                      searchable
                    />
                  </Box>
                </TableCell>
                <TableCell sx={{ fontWeight: 700 }}>
                  <Box sx={{ display: "flex", alignItems: "center" }}>
                    Submitted
                    <DateRangeFilter
                      label="Submitted"
                      from={filters.submittedFrom}
                      to={filters.submittedTo}
                      onChange={(from, to) => setFilters((prev) => ({ ...prev, submittedFrom: from, submittedTo: to }))}
                    />
                  </Box>
                </TableCell>
                {showStatusCol && (
                  <TableCell sx={{ fontWeight: 700 }}>
                    <Box sx={{ display: "flex", alignItems: "center" }}>
                      Status
                      <ColumnFilter
                        label="Status"
                        options={statusColumnOptions}
                        selected={filters.status}
                        onChange={(v) => setColumnFilter("status", v)}
                      />
                    </Box>
                  </TableCell>
                )}
                {showRiskTypeCol && (
                  <TableCell sx={{ fontWeight: 700 }}>
                    <Box sx={{ display: "flex", alignItems: "center" }}>
                      Risk Type
                      <ColumnFilter
                        label="Risk Type"
                        options={riskTypeOptions}
                        selected={filters.riskType}
                        onChange={(v) => setColumnFilter("riskType", v)}
                      />
                    </Box>
                  </TableCell>
                )}
                <TableCell sx={{ fontWeight: 700 }}>
                  <Box sx={{ display: "flex", alignItems: "center" }}>
                    Due
                    <DateRangeFilter
                      label="Due"
                      from={filters.dueFrom}
                      to={filters.dueTo}
                      onChange={(from, to) => setFilters((prev) => ({ ...prev, dueFrom: from, dueTo: to }))}
                      overdueOnly={filters.dueOverdueOnly}
                      onOverdueOnlyChange={(v) => setColumnFilter("dueOverdueOnly", v)}
                    />
                  </Box>
                </TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={colSpan} align="center" sx={{ py: 6 }}>
                    <CircularProgress size={28} />
                  </TableCell>
                </TableRow>
              ) : listError ? (
                <TableRow>
                  <TableCell colSpan={colSpan} align="center" sx={{ py: 4 }}>
                    <Typography color="error">{listError}</Typography>
                  </TableCell>
                </TableRow>
              ) : risks.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={colSpan} align="center" sx={{ py: 6 }}>
                    <Typography color="text.secondary">No risks found.</Typography>
                  </TableCell>
                </TableRow>
              ) : (
                risks.map((risk) => (
                  <TableRow
                    key={risk.id}
                    hover
                    sx={{ cursor: "pointer" }}
                    onClick={() => openDrawer(risk.id)}
                  >
                    <TableCell>
                      <Typography variant="body2" fontWeight={600} color="primary">
                        {risk.risk_code}
                      </Typography>
                    </TableCell>
                    <TableCell sx={{ maxWidth: 220 }}>
                      <Typography
                        variant="body2"
                        sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                      >
                        {risk.risk_title}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">{risk.source_register_name}</Typography>
                    </TableCell>
                    <TableCell>
                      <LevelChip level={risk.risk_level} color={risk.risk_level_color} />
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">{risk.owner_name || "—"}</Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">{formatDate(risk.created_at)}</Typography>
                    </TableCell>
                    {showStatusCol && (
                      <TableCell>
                        <OutlinedStatusChip status={risk.workflow_status} />
                      </TableCell>
                    )}
                    {showRiskTypeCol && (
                      <TableCell>
                        <RiskTypeChip riskType={risk.risk_type} workflowStatus={risk.workflow_status} rejectionStage={risk.rejection_stage} />
                      </TableCell>
                    )}
                    <TableCell>
                      {(() => {
                        const due = calcDue(risk.implementation_date);
                        return (
                          <Typography variant="body2" fontWeight={600} sx={{ color: due.color }}>
                            {due.label}
                          </Typography>
                        );
                      })()}
                    </TableCell>
                    <TableCell align="right" onClick={(e) => e.stopPropagation()}>
                      <Tooltip title="View Details">
                        <IconButton size="small" aria-label={`View details for ${risk.risk_code}`} onClick={() => openDrawer(risk.id)}>
                          <Eye size={16} />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination
          component="div"
          count={total}
          page={page}
          rowsPerPage={rowsPerPage}
          rowsPerPageOptions={[25, 50, 100]}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
        />
      </Paper>

      <RiskDetailDrawer
        open={drawerOpen}
        detail={drawerDetail}
        loading={drawerLoading}
        error={drawerError}
        actionsDisabled={actionInFlight}
        can={can}
        onClose={closeDrawer}
        {...drawerActions}
      />

      <RejectDialog
        open={rejectOpen}
        title="Reject Risk"
        description="Provide a rejection reason. The risk will be returned for revision."
        onClose={() => setRejectOpen(false)}
        onConfirm={handleRejectConfirm}
      />

      {drawerDetail && (
        <ReassessmentDialog
          open={assessOpen}
          riskCode={drawerDetail.risk_code}
          riskScores={riskScores}
          onClose={() => setAssessOpen(false)}
          onSubmit={handleAssessSubmit}
        />
      )}

      {editDetail && (() => {
        const isFullMode =
          editDetail.owner_first_approved_at === null &&
          (editDetail.workflow_status === "PENDING_RISK_OWNER_APPROVAL" ||
           editDetail.workflow_status === "PENDING_REVISION");
        return (
          <EditRiskDialog
            open
            detail={editDetail}
            mode={isFullMode ? "full" : "restricted"}
            assignmentTeams={assignmentTeams}
            users={isFullMode ? users : undefined}
            riskScores={isFullMode ? riskScores : undefined}
            complianceRefs={isFullMode ? complianceRefs : undefined}
            onClose={() => setEditDetail(null)}
            onSave={handleEditSave}
          />
        );
      })()}

      <Dialog open={cancelConfirmOpen} onClose={() => setCancelConfirmOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Cancel Risk</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to cancel this risk? It will be removed from the pending queue and cannot be resubmitted.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCancelConfirmOpen(false)} color="inherit">
            Go Back
          </Button>
          <Button
            color="error"
            variant="contained"
            disabled={actionInFlight}
            onClick={() => {
              setCancelConfirmOpen(false);
              runAction(
                () => cancelRisk(authFetch, drawerDetail!.id),
                "Risk cancelled successfully.",
              );
            }}
          >
            Cancel Risk
          </Button>
        </DialogActions>
      </Dialog>
    </PageContent>
  );
}
