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

import { Box, Button, ListingTable, TablePagination, TextField, Typography } from "@wso2/oxygen-ui";
import { ChevronLeft, History } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState, type JSX } from "react";
import { useNavigate, useParams } from "react-router";
import { ColumnFilter } from "@modules/audit/components/ControlsTable";
import { useGetAudit } from "@modules/audit/api/useGetAudit";
import { useGetControls } from "@modules/audit/api/useGetControls";
import { useGetAuditActivity, activityLogPageSize } from "@modules/audit/api/useGetAuditActivity";
import type { TrailEntry, TrailDetails } from "@modules/audit/api/useGetTrail";
import { formatTimestamp } from "@modules/audit/utils/format";

const ACTION_LABELS: Record<TrailEntry["action"], string> = {
  CREATED: "Created",
  UPDATED: "Updated",
  UPLOADED: "Evidence uploaded",
  RESUBMITTED: "Resubmitted",
  APPROVED: "Approved",
  REJECTED: "Rejected",
  COMMENTED: "Commented",
  DELETED: "Deleted",
  ESCALATED: "Escalated",
  AI_VALIDATED: "AI validated",
  EXPORTED: "Exported",
};

/** One-line human summary of an entry's details for the Details column — plain
 * text, not a rendered timeline (this list can run to thousands of rows). */
function describeDetails(d: TrailDetails | null | undefined): string {
  if (!d) return "";
  const parts: string[] = [];
  if (d.name) parts.push(`"${d.name}"`);
  if (d.controlNumber) parts.push(`control ${d.controlNumber}`);
  if (d.from && d.to) parts.push(`${d.from} → ${d.to}`);
  if (d.status) parts.push(`status: ${d.status}`);
  if (d.periodStart) parts.push(`period start: ${d.periodStart}`);
  if (d.periodEnd) parts.push(`period end: ${d.periodEnd}`);
  if (d.scopeDescription) parts.push("scope updated");
  if (d.via) parts.push(`via ${d.via === "evidence-app" ? "Evidence Portal" : "Audit Hub"}`);
  if (d.comment) parts.push(d.isInternal ? `internal note: "${d.comment}"` : `"${d.comment}"`);
  return parts.join(" · ");
}

export default function AuditActivityLogPage(): JSX.Element {
  const navigate = useNavigate();
  const { auditId: auditIdParam } = useParams<{ auditId: string }>();
  const auditId = parseInt(auditIdParam ?? "0", 10);

  const { data: audit } = useGetAudit(auditId);
  const { data: controlsData } = useGetControls(auditId);
  const controls = useMemo(() => controlsData?.items ?? [], [controlsData]);
  const controlOptions = useMemo(
    () => controls.map((c) => ({ label: c.controlNumber, value: String(c.id) })),
    [controls],
  );

  const [selectedControlIds, setSelectedControlIds] = useState<string[]>([]);
  // Date range has no equivalent column filter in ControlsTable, so it stays as
  // plain date fields above the table.
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [page, setPage] = useState(0);

  function handleDateChange(setter: (v: string) => void, value: string) {
    setter(value);
    setPage(0);
  }

  const isFiltered = selectedControlIds.length > 0 || from.length > 0 || to.length > 0;

  function clearAllFilters() {
    setSelectedControlIds([]);
    setFrom("");
    setTo("");
    setPage(0);
  }

  const apiFilters = useMemo(() => ({
    controlIds: selectedControlIds.map(Number),
    from: from || undefined,
    to: to || undefined,
  }), [selectedControlIds, from, to]);

  const { data, isLoading, isError } = useGetAuditActivity(auditId, apiFilters, page);
  const entries = data?.items ?? [];
  const total = data?.total ?? 0;

  const handleBack = () => void navigate(`/audit/audits/${auditId}`);

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      <Button
        startIcon={<ChevronLeft size={16} />}
        onClick={handleBack}
        sx={{ mb: 2, textTransform: "none", color: "text.secondary", pl: 0 }}
      >
        {audit?.name ?? "Audit"}
      </Button>

      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 2, flexWrap: "wrap", gap: 1 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <History size={20} />
          <Typography variant="h5" fontWeight={700}>
            Activity Log
          </Typography>
        </Box>

        {/* Date range — no equivalent column filter to embed this in */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <TextField
            label="From"
            type="date"
            size="small"
            value={from}
            onChange={(e) => handleDateChange(setFrom, e.target.value)}
            InputLabelProps={{ shrink: true }}
          />
          <TextField
            label="To"
            type="date"
            size="small"
            value={to}
            onChange={(e) => handleDateChange(setTo, e.target.value)}
            InputLabelProps={{ shrink: true }}
          />
          {isFiltered && (
            <Button size="small" onClick={clearAllFilters} sx={{ textTransform: "none", color: "text.secondary" }}>
              Clear filters
            </Button>
          )}
        </Box>
      </Box>

      <ListingTable.Container>
        <ListingTable size="small" density="compact" stickyHeader>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Time</ListingTable.Cell>
              <ListingTable.Cell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Actor</ListingTable.Cell>
              <ListingTable.Cell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Action</ListingTable.Cell>
              <ListingTable.Cell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Control
                  <ColumnFilter
                    label="Control"
                    options={controlOptions}
                    selected={selectedControlIds}
                    onChange={(v) => { setSelectedControlIds(v); setPage(0); }}
                    searchable
                  />
                </Box>
              </ListingTable.Cell>
              <ListingTable.Cell sx={{ fontWeight: 600 }}>Details</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>

          <ListingTable.Body>
            {isLoading ? (
              Array.from({ length: 8 }).map((_, i) => (
                <ListingTable.Row key={`skeleton-${i}`}>
                  <ListingTable.Cell colSpan={5}>&nbsp;</ListingTable.Cell>
                </ListingTable.Row>
              ))
            ) : isError ? (
              <ListingTable.Row>
                <ListingTable.Cell colSpan={5}>
                  <Typography variant="body2" color="error">Failed to load activity log.</Typography>
                </ListingTable.Cell>
              </ListingTable.Row>
            ) : entries.length === 0 ? (
              <ListingTable.Row>
                <ListingTable.Cell colSpan={5}>
                  <ListingTable.EmptyState
                    title={isFiltered ? "No activity matches the selected filters." : "No activity yet."}
                    minHeight={180}
                  />
                </ListingTable.Cell>
              </ListingTable.Row>
            ) : (
              entries.map((e) => {
                const control = e.controlId != null ? controls.find((c) => c.id === e.controlId) : undefined;
                return (
                  <ListingTable.Row key={e.id}>
                    <ListingTable.Cell sx={{ whiteSpace: "nowrap" }}>{formatTimestamp(e.createdAt)}</ListingTable.Cell>
                    <ListingTable.Cell sx={{ whiteSpace: "nowrap" }}>{e.createdBy || "—"}</ListingTable.Cell>
                    <ListingTable.Cell sx={{ whiteSpace: "nowrap" }}>{ACTION_LABELS[e.action] ?? e.action}</ListingTable.Cell>
                    <ListingTable.Cell sx={{ whiteSpace: "nowrap" }}>
                      {e.controlId != null ? (control?.controlNumber ?? `#${e.controlId}`) : "—"}
                    </ListingTable.Cell>
                    <ListingTable.Cell sx={{ maxWidth: 480, overflow: "hidden", textOverflow: "ellipsis" }}>
                      {describeDetails(e.details)}
                    </ListingTable.Cell>
                  </ListingTable.Row>
                );
              })
            )}
          </ListingTable.Body>

          {total > 0 && (
            <ListingTable.Footer>
              <ListingTable.Row>
                <TablePagination
                  count={total}
                  page={page}
                  rowsPerPage={activityLogPageSize}
                  rowsPerPageOptions={[activityLogPageSize]}
                  onPageChange={(_, p) => setPage(p)}
                />
              </ListingTable.Row>
            </ListingTable.Footer>
          )}
        </ListingTable>
      </ListingTable.Container>
    </Box>
  );
}
