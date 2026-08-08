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
  Card,
  CardActionArea,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Divider,
  IconButton,
  InputAdornment,
  LinearProgress,
  Skeleton,
  Snackbar,
  TextField,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { AlertTriangle, ChevronLeft, Plus, Search, ShieldCheck, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useNavigate, useSearchParams } from "react-router";
import AuditCard from "@modules/audit/components/AuditCard";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useGetFrameworks } from "@modules/audit/api/useGetFrameworks";
import { useDeleteAudit } from "@modules/audit/api/useDeleteAudit";
import { useUpdateAuditStatus } from "@modules/audit/api/useUpdateAuditStatus";
import type { Audit, AuditFramework, AuditStatus } from "@modules/audit/types/audit";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";

type StatusFilter = "ACTIVE" | "COMPLETED" | "ARCHIVED" | "ALL";
const STATUS_FILTERS: StatusFilter[] = ["ACTIVE", "COMPLETED", "ARCHIVED", "ALL"];
const STATUS_FILTER_LABELS: Record<StatusFilter, string> = {
  ACTIVE: "Active",
  COMPLETED: "Completed",
  ARCHIVED: "Archived",
  ALL: "All",
};

function filterByStatus(audits: Audit[], statusFilter: StatusFilter): Audit[] {
  const visible = audits.filter((a) => a.status !== "REMOVED");
  if (statusFilter === "ALL") return visible;
  return visible.filter((a) => a.status === (statusFilter as AuditStatus));
}

// ── Framework identity ────────────────────────────────────────────────────────

// Framework card left border follows AuditCard's STATUS_ACCENT convention:
// green when the framework has active audits, grey when it has none. The icon
// and progress bars use the active theme's primary colour.
const FRAMEWORK_ACTIVE_ACCENT = "#2E7D32";
const FRAMEWORK_IDLE_ACCENT = "#757575";

// Number of active audits previewed on a framework card before "+N more".
const FRAMEWORK_CARD_PREVIEW = 3;

interface FrameworkCardProps {
  framework: AuditFramework;
  /** ACTIVE audits belonging to this framework. */
  activeAudits: Audit[];
  /** Whether the framework has any (non-removed) audits at all. */
  hasAudits: boolean;
  onClick: () => void;
}

// FrameworkCard mirrors AuditCard's skeleton (accent border, chip row, title,
// meta line, divider, progress footer) so the drill-in feels seamless.
function FrameworkCard({ framework, activeAudits, hasAudits, onClick }: FrameworkCardProps): JSX.Element {
  const theme = useTheme();
  const color = theme.palette.primary.main;
  const borderAccent = activeAudits.length > 0 ? FRAMEWORK_ACTIVE_ACCENT : FRAMEWORK_IDLE_ACCENT;
  const overdue = activeAudits.reduce((s, a) => s + a.controlCounts.overdue, 0);
  const preview = activeAudits.slice(0, FRAMEWORK_CARD_PREVIEW);
  const moreCount = activeAudits.length - preview.length;

  return (
    <Card
      variant="outlined"
      sx={{
        height: "100%",
        borderLeft: `3px solid ${borderAccent}`,
        transition: "box-shadow 0.2s",
        "&:hover": { boxShadow: 4 },
      }}
    >
      <CardActionArea onClick={onClick} sx={{ height: "100%", alignItems: "flex-start" }}>
        <CardContent sx={{ height: "100%", display: "flex", flexDirection: "column", gap: 1.5, p: 2.5 }}>
          {/* Top row: chips left + icon right — mirrors AuditCard's chip/ring row */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
              {activeAudits.length > 0 ? (
                <Chip
                  label={`${activeAudits.length} active`}
                  size="small"
                  color="success"
                  variant="outlined"
                />
              ) : (
                <Chip label={hasAudits ? "No active audits" : "No audits yet"} size="small" variant="outlined" />
              )}
              {overdue > 0 && (
                <Chip
                  icon={<AlertTriangle size={11} />}
                  label={`${overdue} overdue`}
                  size="small"
                  sx={{
                    fontWeight: 700,
                    color: "#EF4444", bgcolor: "transparent", border: "1px solid #EF4444",
                    "& .MuiChip-icon": { color: "#EF4444" },
                  }}
                />
              )}
            </Box>
            <Box
              sx={{
                width: 40, height: 40, borderRadius: 1.5, flexShrink: 0,
                display: "flex", alignItems: "center", justifyContent: "center",
                color, bgcolor: `${color}18`,
                "[data-color-scheme='dark'] &": { bgcolor: `${color}33` },
              }}
            >
              <ShieldCheck size={20} />
            </Box>
          </Box>

          {/* Framework name — heading variant (h5) so its font metrics match the
              page title / header brand instead of an oversized h6. */}
          <Typography variant="h5" sx={{ fontWeight: 700, lineHeight: 1.3, mt: -0.5 }}>
            {framework.name}
          </Typography>

          <Divider sx={{ mt: "auto" }} />

          {/* Progress footer — per-audit rows in the same slot as AuditCard's bar */}
          {preview.length > 0 ? (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              {preview.map((a) => {
                const pct = a.controlCounts.total > 0
                  ? Math.round((a.controlCounts.approved / a.controlCounts.total) * 100)
                  : 0;
                return (
                  <Box key={a.id}>
                    <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.5, gap: 1 }}>
                      <Typography variant="caption" color="text.secondary" noWrap title={a.name}>
                        {a.name}
                      </Typography>
                      <Typography variant="caption" fontWeight={700} sx={{ color, flexShrink: 0 }}>
                        {pct}%
                      </Typography>
                    </Box>
                    <LinearProgress
                      variant="determinate"
                      value={pct}
                      sx={{
                        height: 6, borderRadius: 3, bgcolor: "#E0E0E0",
                        "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
                        "& .MuiLinearProgress-bar": { bgcolor: color, borderRadius: 3 },
                      }}
                    />
                  </Box>
                );
              })}
              {moreCount > 0 && (
                <Typography variant="caption" color="text.secondary">
                  +{moreCount} more active audit{moreCount === 1 ? "" : "s"}
                </Typography>
              )}
            </Box>
          ) : (
            <Typography variant="caption" color="text.secondary">
              Create an audit to start tracking progress
            </Typography>
          )}
        </CardContent>
      </CardActionArea>
    </Card>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AuditsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { can } = useAuditPrivileges();
  const canCreateAudit = can(AuditPrivilege.CreateAudit);
  const {
    data: auditsData,
    isLoading: auditsLoading,
    isError: auditsError,
    refetch: refetchAudits,
  } = useGetAudits();
  const { data: frameworksData, isLoading: frameworksLoading } = useGetFrameworks();
  const deleteAudit = useDeleteAudit();
  const updateAuditStatus = useUpdateAuditStatus();
  const canUpdateAudit = can(AuditPrivilege.UpdateAudit);

  // Drill + status filter live in the URL so refresh/back/sharing all work.
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedFrameworkId = searchParams.get("framework") !== null
    ? Number(searchParams.get("framework"))
    : null;
  const statusParam = (searchParams.get("status") ?? "ACTIVE") as StatusFilter;
  const statusFilter: StatusFilter = STATUS_FILTERS.includes(statusParam) ? statusParam : "ACTIVE";

  const [search, setSearch] = useState("");
  const [auditToDelete, setAuditToDelete] = useState<Audit | null>(null);

  const allAudits = auditsData?.items ?? [];
  const allFrameworks = frameworksData ?? [];
  const selectedFramework = allFrameworks.find((f) => f.id === selectedFrameworkId) ?? null;

  function selectFramework(id: number | null) {
    const next = new URLSearchParams(searchParams);
    if (id === null) {
      next.delete("framework");
      next.delete("status");
    } else {
      next.set("framework", String(id));
      next.delete("status");
    }
    setSearchParams(next, { replace: id === null });
    setSearch("");
  }

  function setStatusFilter(sf: StatusFilter) {
    const next = new URLSearchParams(searchParams);
    if (sf === "ACTIVE") next.delete("status");
    else next.set("status", sf);
    setSearchParams(next, { replace: true });
  }

  const q = search.trim().toLowerCase();

  // Global search (framework overview level): flat results across all audits.
  const globalMatches = q && !selectedFramework
    ? allAudits.filter((a) => a.status !== "REMOVED" && a.name.toLowerCase().includes(q))
    : [];

  const frameworkAudits = selectedFrameworkId !== null
    ? allAudits.filter((a) => a.framework.id === selectedFrameworkId)
    : [];
  const statusFiltered = filterByStatus(frameworkAudits, statusFilter);
  const displayed = q ? statusFiltered.filter((a) => a.name.toLowerCase().includes(q)) : statusFiltered;

  const statusCounts = Object.fromEntries(
    STATUS_FILTERS.map((sf) => [sf, filterByStatus(frameworkAudits, sf).length]),
  ) as Record<StatusFilter, number>;

  const activeAuditsTotal = allAudits.filter((a) => a.status === "ACTIVE").length;
  const isLoading = auditsLoading || frameworksLoading;

  const handleDeleteConfirm = () => {
    if (!auditToDelete) return;
    deleteAudit.mutate(auditToDelete.id, {
      onSuccess: () => setAuditToDelete(null),
    });
  };

  // Archive is reversible, so no confirmation dialog — just a result snackbar.
  const [archiveSnack, setArchiveSnack] = useState<{ severity: "success" | "error"; message: string } | null>(null);
  const handleArchiveToggle = (audit: Audit) => {
    const archiving = audit.status !== "ARCHIVED";
    // We don't persist the pre-archive status, so infer the restore target from
    // completion: a fully-approved audit returns to Completed, otherwise Active.
    const { total, approved } = audit.controlCounts;
    const restoreStatus: AuditStatus = total > 0 && approved === total ? "COMPLETED" : "ACTIVE";
    const targetStatus: AuditStatus = archiving ? "ARCHIVED" : restoreStatus;
    updateAuditStatus.mutate(
      { auditId: audit.id, status: targetStatus },
      {
        onSuccess: () =>
          setArchiveSnack({
            severity: "success",
            message: archiving
              ? `"${audit.name}" archived.`
              : `"${audit.name}" restored to ${restoreStatus === "COMPLETED" ? "completed" : "active"}.`,
          }),
        onError: () =>
          setArchiveSnack({
            severity: "error",
            message: archiving ? "Failed to archive the audit." : "Failed to restore the audit.",
          }),
      },
    );
  };

  const searchField = (
    <TextField
      size="small"
      placeholder={selectedFramework ? `Search ${selectedFramework.name} audits…` : "Search all audits…"}
      value={search}
      onChange={(e) => setSearch(e.target.value)}
      sx={{ minWidth: 230 }}
      slotProps={{
        input: {
          startAdornment: (
            <InputAdornment position="start">
              <Search size={16} />
            </InputAdornment>
          ),
          endAdornment: search ? (
            <InputAdornment position="end">
              <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => setSearch("")}>
                <X size={14} />
              </IconButton>
            </InputAdornment>
          ) : null,
        },
      }}
    />
  );

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      {/* Page header */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          mb: 3,
          flexWrap: "wrap",
          gap: 2,
        }}
      >
        {selectedFramework ? (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Button
              startIcon={<ChevronLeft size={16} />}
              onClick={() => selectFramework(null)}
              sx={{ textTransform: "none", color: "text.secondary", pl: 0, minWidth: 0 }}
            >
              Frameworks
            </Button>
            <Typography variant="body2" color="text.secondary" sx={{ mx: 0.5 }}>
              /
            </Typography>
            <Typography variant="h5" fontWeight={700}>
              {selectedFramework.name}
            </Typography>
          </Box>
        ) : (
          <Box>
            <Typography variant="h4" fontWeight={700}>
              Audits
            </Typography>
            {!isLoading && (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                {allFrameworks.length} framework{allFrameworks.length === 1 ? "" : "s"} · {activeAuditsTotal} active audit{activeAuditsTotal === 1 ? "" : "s"}
              </Typography>
            )}
          </Box>
        )}

        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          {!selectedFramework && searchField}
          {canCreateAudit && (
            <Button
              variant="contained"
              startIcon={<Plus size={16} />}
              sx={{ textTransform: "none" }}
              onClick={() =>
                void navigate(
                  selectedFrameworkId !== null
                    ? `/audit/audits/create?framework=${selectedFrameworkId}`
                    : "/audit/audits/create",
                )
              }
            >
              New Audit
            </Button>
          )}
        </Box>
      </Box>

      {/* Loading state */}
      {isLoading && (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
            gap: 2.5,
          }}
        >
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} variant="rectangular" height={180} sx={{ borderRadius: 2 }} />
          ))}
        </Box>
      )}

      {/* Global search results (framework overview + query) */}
      {!isLoading && !selectedFramework && q && (
        <>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {globalMatches.length} audit{globalMatches.length === 1 ? "" : "s"} matching “{search.trim()}”
          </Typography>
          {globalMatches.length > 0 ? (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
                gap: 2.5,
              }}
            >
              {globalMatches.map((audit) => (
                <AuditCard
                  key={audit.id}
                  audit={audit}
                  onClick={() => void navigate(`/audit/audits/${audit.id}`)}
                  onDelete={() => setAuditToDelete(audit)}
                  canDelete={canCreateAudit}
                  onArchiveToggle={() => handleArchiveToggle(audit)}
                  canArchive={canUpdateAudit}
                />
              ))}
            </Box>
          ) : (
            <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", py: 8, gap: 1.5 }}>
              <Search size={40} style={{ opacity: 0.25 }} />
              <Typography variant="body1" color="text.secondary">No audits match your search.</Typography>
            </Box>
          )}
        </>
      )}

      {/* Framework overview */}
      {!isLoading && !selectedFramework && !q && (
        <>
          {allFrameworks.length === 0 ? (
            <Box
              sx={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                py: 10,
                gap: 2,
              }}
            >
              <ShieldCheck size={48} style={{ opacity: 0.25 }} />
              <Typography variant="h6">No frameworks yet</Typography>
              <Typography variant="body2" color="text.secondary">
                Create a framework when creating your first audit.
              </Typography>
              {canCreateAudit && (
                <Button
                  variant="contained"
                  startIcon={<Plus size={16} />}
                  sx={{ textTransform: "none" }}
                  onClick={() => void navigate("/audit/audits/create")}
                >
                  New Audit
                </Button>
              )}
            </Box>
          ) : (
            <>
              <Box
                sx={{
                  display: "grid",
                  gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
                  gap: 2.5,
                }}
              >
                {allFrameworks.map((fw) => {
                  const fwAudits = allAudits.filter((a) => a.framework.id === fw.id && a.status !== "REMOVED");
                  return (
                    <FrameworkCard
                      key={fw.id}
                      framework={fw}
                      activeAudits={fwAudits.filter((a) => a.status === "ACTIVE")}
                      hasAudits={fwAudits.length > 0}
                      onClick={() => selectFramework(fw.id)}
                    />
                  );
                })}
              </Box>
            </>
          )}
        </>
      )}

      {/* Drilled framework view */}
      {!isLoading && selectedFramework && (
        <>
          {/* Status chips + search bar */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 2,
              mb: 3,
              flexWrap: "wrap",
            }}
          >
            <Box sx={{ display: "flex", gap: 1 }}>
              {STATUS_FILTERS.map((sf) => (
                <Chip
                  key={sf}
                  clickable
                  label={sf === "ALL" ? STATUS_FILTER_LABELS[sf] : `${STATUS_FILTER_LABELS[sf]} (${statusCounts[sf]})`}
                  onClick={() => setStatusFilter(sf)}
                  color={statusFilter === sf ? "primary" : "default"}
                  variant={statusFilter === sf ? "filled" : "outlined"}
                  sx={{ fontWeight: statusFilter === sf ? 700 : 400 }}
                />
              ))}
            </Box>

            {searchField}

            <Typography variant="caption" color="text.secondary" sx={{ ml: "auto" }}>
              {displayed.length} {displayed.length === 1 ? "audit" : "audits"}
            </Typography>
          </Box>

          {/* Error */}
          {auditsError && (
            <Box
              sx={{ display: "flex", flexDirection: "column", alignItems: "center", py: 8, gap: 2 }}
            >
              <Typography variant="body1" color="text.secondary">
                Failed to load audits.
              </Typography>
              <Button variant="outlined" onClick={() => void refetchAudits()}>
                Try again
              </Button>
            </Box>
          )}

          {/* Audit cards */}
          {!auditsError && displayed.length > 0 && (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
                gap: 2.5,
              }}
            >
              {displayed.map((audit) => (
                <AuditCard
                  key={audit.id}
                  audit={audit}
                  onClick={() => void navigate(`/audit/audits/${audit.id}`)}
                  onDelete={() => setAuditToDelete(audit)}
                  canDelete={canCreateAudit}
                  onArchiveToggle={() => handleArchiveToggle(audit)}
                  canArchive={canUpdateAudit}
                />
              ))}
            </Box>
          )}

          {/* Empty state */}
          {!auditsError && displayed.length === 0 && (
            <Box
              sx={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                py: 10,
                gap: 2,
              }}
            >
              <ShieldCheck size={48} style={{ opacity: 0.25 }} />
              <Typography variant="h6">No audits found</Typography>
              <Typography variant="body2" color="text.secondary">
                {search.trim()
                  ? "No audits match your search."
                  : statusFilter === "ALL"
                  ? `No ${selectedFramework.name} audits yet.`
                  : `No ${STATUS_FILTER_LABELS[statusFilter].toLowerCase()} ${selectedFramework.name} audits.`}
              </Typography>
              {statusFilter !== "ALL" && frameworkAudits.filter((a) => a.status !== "REMOVED").length > 0 && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => setStatusFilter("ALL")}
                >
                  Show all audits
                </Button>
              )}
            </Box>
          )}
        </>
      )}

      {/* Delete confirmation dialog */}
      <Dialog
        open={Boolean(auditToDelete)}
        onClose={() => setAuditToDelete(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>Delete audit?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            <strong>{auditToDelete?.name}</strong> and all its controls will be permanently deleted.
            This cannot be undone.
          </DialogContentText>
          {deleteAudit.isError && (
            <Alert severity="error" sx={{ mt: 2 }}>
              Failed to delete the audit. Please try again.
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAuditToDelete(null)} disabled={deleteAudit.isPending}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={handleDeleteConfirm}
            disabled={deleteAudit.isPending}
          >
            {deleteAudit.isPending ? "Deleting…" : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Archive / restore result snackbar */}
      <Snackbar
        open={Boolean(archiveSnack)}
        autoHideDuration={4000}
        onClose={() => setArchiveSnack(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          severity={archiveSnack?.severity ?? "success"}
          variant="filled"
          onClose={() => setArchiveSnack(null)}
        >
          {archiveSnack?.message}
        </Alert>
      </Snackbar>
    </Box>
  );
}
