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
  Button as MuiButton,
  Divider,
  Paper,
  Skeleton,
  Stack,
} from "@mui/material";
import { Box, Typography } from "@wso2/oxygen-ui";
import {
  AlertCircle,
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ClipboardList,
  ListChecks,
} from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useNavigate, useParams } from "react-router";
import FilterPanel from "@modules/audit/components/FilterPanel";
import AuditStatusChip from "@modules/audit/components/AuditStatusChip";
import ControlsTable from "@modules/audit/components/ControlsTable";
import ControlDrawer from "@modules/audit/components/ControlDrawer";
import { useGetAudit } from "@modules/audit/api/useGetAudit";
import { useGetControls } from "@modules/audit/api/useGetControls";
import { formatDateRange } from "@modules/audit/utils/format";
import {
  EMPTY_CONTROL_FILTERS,
  applyControlFilters,
  buildControlFilterFields,
  activeFilterCount,
} from "@modules/audit/utils/controlFilters";
import type { AuditControl } from "@modules/audit/types/audit";

interface StatCardProps {
  icon: JSX.Element;
  label: string;
  value: number;
  color?: string;
}

function StatCard({ icon, label, value, color }: StatCardProps): JSX.Element {
  return (
    <Paper
      variant="outlined"
      sx={{ p: 2, borderRadius: 2, display: "flex", alignItems: "center", gap: 2 }}
    >
      <Box
        sx={{
          width: 44,
          height: 44,
          borderRadius: 2,
          bgcolor: color ? `${color}.light` : "action.selected",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: color ? `${color}.main` : "text.secondary",
          flexShrink: 0,
        }}
      >
        {icon}
      </Box>
      <Box>
        <Typography variant="h5" fontWeight={700} lineHeight={1}>
          {value}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {label}
        </Typography>
      </Box>
    </Paper>
  );
}

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

  const controls = controlsData?.items ?? [];
  const filteredControls = applyControlFilters(controls, filters, search);
  const filterFields = buildControlFilterFields(controls);

  const inProgressCount = controls.filter(
    (c) =>
      c.status !== "APPROVED" &&
      c.status !== "NOT_STARTED" &&
      c.status !== "SAMPLING_REQUIRED",
  ).length;
  const overdueCount = controls.filter((c) => c.isOverdue).length;
  const approvedCount = controls.filter((c) => c.status === "APPROVED").length;

  const handleBack = () => void navigate("/audit/audits");

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
        <MuiButton variant="outlined" onClick={handleBack}>
          Back to Audits
        </MuiButton>
      </Box>
    );
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      {/* Back button */}
      <MuiButton
        startIcon={<ChevronLeft size={16} />}
        onClick={handleBack}
        sx={{ mb: 2, textTransform: "none", color: "text.secondary", pl: 0 }}
      >
        Audits
      </MuiButton>

      {/* Audit header */}
      {auditLoading ? (
        <Box sx={{ mb: 3 }}>
          <Skeleton variant="text" width={320} height={40} />
          <Skeleton variant="text" width={240} height={24} sx={{ mt: 0.5 }} />
        </Box>
      ) : (
        audit && (
          <Box sx={{ mb: 3 }}>
            <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" mb={0.75}>
              <Typography variant="h5" fontWeight={700}>
                {audit.name}
              </Typography>
              <AuditStatusChip status={audit.status} />
            </Stack>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
              <Typography variant="body2" color="text.secondary">
                {audit.framework.name}
                {audit.framework.version ? ` (${audit.framework.version})` : ""}
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
            </Stack>
          </Box>
        )
      )}

      {/* Stat cards */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "repeat(2, 1fr)", sm: "repeat(4, 1fr)" },
          gap: 2,
          mb: 3,
        }}
      >
        {controlsLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} variant="rectangular" height={80} sx={{ borderRadius: 2 }} />
          ))
        ) : (
          <>
            <StatCard icon={<ClipboardList size={20} />} label="Total Controls" value={controls.length} />
            <StatCard icon={<CheckCircle2 size={20} />} label="Approved" value={approvedCount} color="success" />
            <StatCard icon={<ListChecks size={20} />} label="In Progress" value={inProgressCount} color="info" />
            <StatCard icon={<AlertCircle size={20} />} label="Overdue" value={overdueCount} color="error" />
          </>
        )}
      </Box>

      {/* Filter + search bar */}
      <Box sx={{ mb: 1.5 }}>
        <FilterPanel
          fields={filterFields}
          values={filters}
          onChange={setFilters}
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search by control number or description..."
        />
        {(activeFilterCount(filters) > 0 || search.trim().length > 0) && (
          <Typography variant="caption" color="text.secondary" sx={{ mt: 0.75, display: "block" }}>
            {filteredControls.length} of {controls.length} controls
          </Typography>
        )}
      </Box>

      {/* Controls table */}
      <ControlsTable
        controls={filteredControls}
        isLoading={controlsLoading}
        onRowClick={(control) => setSelectedControl(control)}
      />

      {/* Control detail drawer */}
      <ControlDrawer
        control={selectedControl}
        open={selectedControl !== null}
        onClose={() => setSelectedControl(null)}
      />
    </Box>
  );
}
