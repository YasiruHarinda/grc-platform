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
  Chip,
  CircularProgress,
  MenuItem,
  Paper,
  Select,
  type SelectChangeEvent,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { type JSX, useEffect, useRef, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import {
  fetchActivityLog,
  type AdminActivityAction,
  type AdminActivityEntityType,
  type AdminActivityLogEntry,
} from "../api/adminApi";

const ACTION_LABELS: Record<AdminActivityAction, string> = {
  CREATED: "Created",
  UPDATED: "Updated",
  DELETED: "Deleted",
  STATUS_CHANGED: "Status changed",
  GRANTED: "Granted",
  REVOKED: "Revoked",
};

const ENTITY_TYPE_LABELS: Record<AdminActivityEntityType, string> = {
  USER: "User",
  GRANT: "Grant",
  RISK_TEAM: "Risk Team",
  RISK_CATEGORY: "Risk Category",
  COMPLIANCE_REFERENCE: "Compliance Reference",
  RISK_SCORE: "Risk Score",
  AUDIT_TEAM: "Audit Team",
};

const PAGE_SIZE = 25;

// One-line human summary of an entry's details, tolerant of unknown fields.
function describeDetails(d: AdminActivityLogEntry["details"]): string {
  if (!d) return "";
  const parts: string[] = [];
  if (typeof d.user === "string") parts.push(`user: ${d.user}`);
  if (typeof d.name === "string") parts.push(`"${d.name}"`);
  if (typeof d.role === "string") parts.push(d.role);
  if (typeof d.scope === "string") parts.push(`@ ${d.scope}`);
  if (typeof d.status === "string") parts.push(`status: ${d.status}`);
  if (typeof d.userType === "string") parts.push(String(d.userType));
  if (typeof d.teamType === "string") parts.push(String(d.teamType));
  return parts.join(" · ");
}

// The Admin Console's "who did what and when" log, rendered as a tab on
// UsersPage.tsx rather than its own route.
export default function ActivityLogPage(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [entries, setEntries] = useState<AdminActivityLogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [entityType, setEntityType] = useState<AdminActivityEntityType | "">("");
  const [action, setAction] = useState<AdminActivityAction | "">("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [page, setPage] = useState(0);

  const loadSeqRef = useRef(0);

  const isFiltered = entityType !== "" || action !== "" || from !== "" || to !== "";

  useEffect(() => {
    const seq = ++loadSeqRef.current;
    setLoading(true);
    setError(null);
    fetchActivityLog(authFetch, {
      entityType: entityType || undefined,
      action: action || undefined,
      from: from || undefined,
      to: to || undefined,
      limit: PAGE_SIZE,
      offset: page * PAGE_SIZE,
    })
      .then((resp) => {
        if (seq !== loadSeqRef.current) return;
        setEntries(resp.entries);
        setTotal(resp.total);
      })
      .catch((e) => {
        if (seq !== loadSeqRef.current) return;
        setError(e instanceof Error ? e.message : "Failed to load activity log");
      })
      .finally(() => {
        if (seq !== loadSeqRef.current) return;
        setLoading(false);
      });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entityType, action, from, to, page]);

  const clearFilters = () => {
    setEntityType("");
    setAction("");
    setFrom("");
    setTo("");
    setPage(0);
  };

  return (
    <Box>
      <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" useFlexGap sx={{ mb: 2 }}>
        <Select
          size="small"
          displayEmpty
          value={entityType}
          onChange={(e: SelectChangeEvent) => {
            setEntityType(e.target.value as AdminActivityEntityType | "");
            setPage(0);
          }}
          sx={{ minWidth: 180 }}
        >
          <MenuItem value="">All entity types</MenuItem>
          {(Object.keys(ENTITY_TYPE_LABELS) as AdminActivityEntityType[]).map((t) => (
            <MenuItem key={t} value={t}>
              {ENTITY_TYPE_LABELS[t]}
            </MenuItem>
          ))}
        </Select>
        <Select
          size="small"
          displayEmpty
          value={action}
          onChange={(e: SelectChangeEvent) => {
            setAction(e.target.value as AdminActivityAction | "");
            setPage(0);
          }}
          sx={{ minWidth: 160 }}
        >
          <MenuItem value="">All actions</MenuItem>
          {(Object.keys(ACTION_LABELS) as AdminActivityAction[]).map((a) => (
            <MenuItem key={a} value={a}>
              {ACTION_LABELS[a]}
            </MenuItem>
          ))}
        </Select>
        <TextField
          label="From"
          type="date"
          size="small"
          value={from}
          onChange={(e) => { setFrom(e.target.value); setPage(0); }}
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <TextField
          label="To"
          type="date"
          size="small"
          value={to}
          onChange={(e) => { setTo(e.target.value); setPage(0); }}
          slotProps={{ inputLabel: { shrink: true } }}
        />
        {isFiltered && (
          <Button size="small" onClick={clearFilters} sx={{ textTransform: "none", color: "text.secondary" }}>
            Clear filters
          </Button>
        )}
      </Stack>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 700, whiteSpace: "nowrap" }}>Time</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Actor</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Action</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Entity</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Details</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                  <CircularProgress size={22} />
                </TableCell>
              </TableRow>
            )}
            {!loading && entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    {isFiltered ? "No activity matches the selected filters." : "No activity yet."}
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              entries.map((e) => (
                <TableRow key={e.id}>
                  <TableCell sx={{ whiteSpace: "nowrap" }}>{new Date(e.createdOn).toLocaleString()}</TableCell>
                  <TableCell>{e.actorName || e.actorEmail || e.actorId}</TableCell>
                  <TableCell>
                    <Chip size="small" variant="outlined" label={ACTION_LABELS[e.action] ?? e.action} />
                  </TableCell>
                  <TableCell sx={{ whiteSpace: "nowrap" }}>{ENTITY_TYPE_LABELS[e.entityType] ?? e.entityType}</TableCell>
                  <TableCell>{describeDetails(e.details)}</TableCell>
                </TableRow>
              ))}
          </TableBody>
        </Table>
        {total > 0 && (
          <TablePagination
            component="div"
            count={total}
            page={page}
            rowsPerPage={PAGE_SIZE}
            rowsPerPageOptions={[PAGE_SIZE]}
            onPageChange={(_, p) => setPage(p)}
          />
        )}
      </TableContainer>
    </Box>
  );
}
