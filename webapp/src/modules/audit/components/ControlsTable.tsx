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
  Chip,
  Paper,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Tooltip,
} from "@mui/material";
import { Box, Typography } from "@wso2/oxygen-ui";
import { AlertCircle } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import UserAvatar from "@modules/audit/components/UserAvatar";
import { formatAuditDate } from "@modules/audit/utils/format";
import type { AuditControl } from "@modules/audit/types/audit";

interface ControlsTableProps {
  controls: AuditControl[];
  isLoading: boolean;
  onRowClick: (control: AuditControl) => void;
}

const HEADERS = [
  "Control No.",
  "Description",
  "Req. Type",
  "Control Type",
  "Status",
  "Auditor POC",
  "Process Owner",
  "Team",
  "Scope",
  "Due Date",
];

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

export default function ControlsTable({
  controls,
  isLoading,
  onRowClick,
}: ControlsTableProps): JSX.Element {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);

  // Clamp page if controls shrank after filtering
  const totalPages = Math.ceil(controls.length / rowsPerPage);
  const safePage = totalPages === 0 ? 0 : Math.min(page, totalPages - 1);
  if (safePage !== page) setPage(safePage);

  const displayed = controls.slice(safePage * rowsPerPage, (safePage + 1) * rowsPerPage);

  if (isLoading) {
    return (
      <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden" }}>
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                {HEADERS.map((h) => (
                  <TableCell key={h} sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>
                    {h}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {HEADERS.map((h) => (
                    <TableCell key={h}>
                      <Skeleton variant="text" width={h === "Description" ? 200 : 80} />
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Paper>
    );
  }

  if (controls.length === 0) {
    return (
      <Paper
        variant="outlined"
        sx={{
          borderRadius: 2,
          py: 6,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Typography variant="body2" color="text.secondary">
          No controls match the selected filter.
        </Typography>
      </Paper>
    );
  }

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden" }}>
      <TableContainer>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 90 }}>
                Control No.
              </TableCell>
              <TableCell sx={{ fontWeight: 600, minWidth: 240 }}>
                Description
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 100 }}>
                Req. Type
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 110 }}>
                Control Type
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 170 }}>
                Status
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 120 }}>
                Auditor POC
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 130 }}>
                Process Owner
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 120 }}>
                Team
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 120 }}>
                Scope
              </TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap", minWidth: 110 }}>
                Due Date
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {displayed.map((control) => (
              <TableRow
                key={control.id}
                onClick={() => onRowClick(control)}
                sx={{
                  cursor: "pointer",
                  bgcolor: control.isOverdue ? "rgba(211, 47, 47, 0.06)" : undefined,
                  "&:hover": {
                    bgcolor: control.isOverdue
                      ? "rgba(211, 47, 47, 0.12)"
                      : "action.hover",
                  },
                }}
              >
                {/* Control number */}
                <TableCell>
                  <Typography variant="body2" fontWeight={600} noWrap>
                    {control.controlNumber}
                  </Typography>
                </TableCell>

                {/* Description */}
                <TableCell>
                  <Typography
                    variant="body2"
                    sx={{
                      display: "-webkit-box",
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: "vertical",
                      overflow: "hidden",
                      maxWidth: 340,
                    }}
                  >
                    {control.description}
                  </Typography>
                </TableCell>

                {/* Req. type */}
                <TableCell>
                  <Chip
                    label={REQ_TYPE_LABELS[control.requirementType]}
                    size="small"
                    variant="outlined"
                    color={control.requirementType === "DESIGN" ? "primary" : "secondary"}
                  />
                </TableCell>

                {/* Control type */}
                <TableCell>
                  <Typography variant="body2" noWrap>
                    {CTRL_TYPE_LABELS[control.controlType]}
                  </Typography>
                </TableCell>

                {/* Status */}
                <TableCell>
                  <ControlStatusChip status={control.status} />
                </TableCell>

                {/* Auditor POC */}
                <TableCell>
                  {control.auditorName ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <UserAvatar name={control.auditorName} size={26} />
                      <Typography variant="body2" noWrap>
                        {control.auditorName}
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.disabled">—</Typography>
                  )}
                </TableCell>

                {/* Process owner */}
                <TableCell>
                  {control.ownerName ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <UserAvatar name={control.ownerName} size={26} />
                      <Typography variant="body2" noWrap>
                        {control.ownerName}
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.disabled">—</Typography>
                  )}
                </TableCell>

                {/* Team */}
                <TableCell>
                  <Typography variant="body2" noWrap>
                    {control.teamName ?? "—"}
                  </Typography>
                </TableCell>

                {/* Scope */}
                <TableCell>
                  <Typography variant="body2" noWrap>
                    {SCOPE_LABELS[control.scope]}
                  </Typography>
                </TableCell>

                {/* Due date */}
                <TableCell>
                  {control.dueDate ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                      <Typography
                        variant="body2"
                        noWrap
                        color={control.isOverdue ? "error.main" : "text.primary"}
                        fontWeight={control.isOverdue ? 600 : 400}
                      >
                        {formatAuditDate(control.dueDate)}
                      </Typography>
                      {control.isOverdue && (
                        <Tooltip title="Overdue">
                          <AlertCircle
                            size={14}
                            color="var(--mui-palette-error-main, #d32f2f)"
                          />
                        </Tooltip>
                      )}
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.secondary">
                      —
                    </Typography>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <TablePagination
        component="div"
        count={controls.length}
        page={safePage}
        rowsPerPage={rowsPerPage}
        rowsPerPageOptions={[25, 50, 100]}
        onPageChange={(_, newPage) => setPage(newPage)}
        onRowsPerPageChange={(e) => {
          setRowsPerPage(parseInt(e.target.value, 10));
          setPage(0);
        }}
      />
    </Paper>
  );
}
