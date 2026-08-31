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
  IconButton,
  MenuItem,
  Paper,
  Select,
  type SelectChangeEvent,
  Stack,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  Tabs,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil, Plus, Search } from "@wso2/oxygen-ui-icons-react";
import { type JSX, type SyntheticEvent, useEffect, useMemo, useRef, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { fetchAdminUsers, fetchRoles, updateUserStatus, type AdminUser, type Role } from "../api/adminApi";
import { useAdminPrivileges } from "../hooks/useAdminPrivileges";
import { AdminPrivilege } from "../privileges";
import AddUserDialog from "./AddUserDialog";
import ActivityLogPage from "./ActivityLogPage";
import GrantEditorDialog from "./GrantEditorDialog";

type SubTab = "users" | "activity";

type UserStatus = AdminUser["status"];

const statusColor: Record<UserStatus, "success" | "default" | "error"> = {
  ACTIVE: "success",
  INACTIVE: "default",
  REMOVED: "error",
};

export default function UsersPage(): JSX.Element {
  const authFetch = useAuthApiClient();
  const { can } = useAdminPrivileges();
  // Activity Log tab needs all three Admin Console privileges, not just
  // ManageUsers (which already gates this whole page/route).
  const canSeeActivityLog =
    can(AdminPrivilege.ManageUsers) && can(AdminPrivilege.ManageRiskHub) && can(AdminPrivilege.ManageAuditHub);
  const [tab, setTab] = useState<SubTab>("users");
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [grantEditorUser, setGrantEditorUser] = useState<AdminUser | null>(null);
  // Tracks every row whose status change is in flight, so overlapping updates
  // on different rows (or a second update on the same row) each disable only
  // their own Select rather than sharing one slot that clears on whichever
  // request finishes first.
  const [statusUpdatingIds, setStatusUpdatingIds] = useState<Set<number>>(new Set());
  // Guards against a stale load() response (from an earlier status change)
  // overwriting the state set by a load() that started later but resolved
  // first.
  const loadSeqRef = useRef(0);

  const load = (): Promise<void> => {
    const seq = ++loadSeqRef.current;
    setLoading(true);
    setError(null);
    return Promise.all([fetchAdminUsers(authFetch), fetchRoles(authFetch)])
      .then(([u, r]) => {
        if (seq !== loadSeqRef.current) return;
        setUsers(u);
        setRoles(r);
      })
      .catch((e) => {
        if (seq !== loadSeqRef.current) return;
        setError(e instanceof Error ? e.message : "Failed to load users");
      })
      .finally(() => {
        if (seq !== loadSeqRef.current) return;
        setLoading(false);
      });
  };

  useEffect(() => {
    load();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return users;
    return users.filter(
      (u) => u.displayName.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
    );
  }, [users, search]);

  // Keep the grant editor's selected user in sync with the freshly reloaded
  // list (e.g. after a grant is added/revoked inside it) rather than holding
  // a stale snapshot from when it was opened.
  useEffect(() => {
    if (!grantEditorUser) return;
    const fresh = users.find((u) => u.id === grantEditorUser.id);
    if (fresh) setGrantEditorUser(fresh);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [users]);

  // REMOVED is the more final state (an ex-employee, gone for good) — confirm
  // before applying it rather than letting the same control used for routine
  // ACTIVE/INACTIVE toggling trigger it on a misclick. The server separately
  // refuses a caller changing their own status (self-lockout guard); that
  // failure surfaces through the ordinary error alert below.
  const handleStatusChange = (user: AdminUser, next: UserStatus) => {
    if (next === "REMOVED" && !window.confirm(`Mark ${user.displayName || user.email || user.uuid} as Removed?`)) {
      return;
    }
    setError(null);
    setStatusUpdatingIds((prev) => new Set(prev).add(user.id));
    updateUserStatus(authFetch, user.id, next)
      .then(load)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to update status"))
      .finally(() =>
        setStatusUpdatingIds((prev) => {
          const nextIds = new Set(prev);
          nextIds.delete(user.id);
          return nextIds;
        }),
      );
  };

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="flex-end" sx={{ mb: 2 }}>
        <Box>
          <Typography variant="h4" fontWeight={700} sx={{ mb: 0.5 }}>
            Users
          </Typography>
          <Typography variant="body2" color="text.secondary">
            People provisioned into the platform with at least one role grant, plus their current (role @ scope)
            pairs.
          </Typography>
        </Box>
      </Stack>

      {canSeeActivityLog && (
        <Tabs
          value={tab}
          onChange={(_e: SyntheticEvent, value: SubTab) => setTab(value)}
          sx={{ mb: 2 }}
        >
          <Tab label="Users" value="users" />
          <Tab label="Activity Log" value="activity" />
        </Tabs>
      )}

      {tab === "activity" && canSeeActivityLog ? (
        <ActivityLogPage />
      ) : (
        <>
          <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 2 }}>
            <TextField
              size="small"
              placeholder="Search users by name or email"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              slotProps={{ input: { startAdornment: <Search size={15} style={{ marginRight: 8 }} /> } }}
              sx={{ minWidth: 280 }}
            />
            <Box sx={{ flex: 1 }} />
            <Button variant="contained" startIcon={<Plus size={14} />} onClick={() => setAddOpen(true)}>
              Add User
            </Button>
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
              <TableCell sx={{ fontWeight: 700, width: 220 }}>Name</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Email</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Date Added</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Roles &amp; Scopes</TableCell>
              <TableCell sx={{ fontWeight: 700 }} align="right">
                Actions
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                  <CircularProgress size={22} />
                </TableCell>
              </TableRow>
            )}
            {!loading && filtered.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    No users found.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              filtered.map((u) => (
                <TableRow key={u.id} sx={u.status !== "ACTIVE" ? { opacity: 0.55 } : undefined}>
                  <TableCell sx={{ fontWeight: 600 }}>{u.displayName || "—"}</TableCell>
                  <TableCell>{u.email || "—"}</TableCell>
                  <TableCell>
                    <Select
                      size="small"
                      variant="standard"
                      disableUnderline
                      value={u.status}
                      disabled={statusUpdatingIds.has(u.id)}
                      onChange={(e: SelectChangeEvent) => handleStatusChange(u, e.target.value as UserStatus)}
                      renderValue={(value) => (
                        <Chip
                          size="small"
                          label={value === "ACTIVE" ? "Active" : value === "INACTIVE" ? "Inactive" : "Removed"}
                          color={statusColor[value as UserStatus]}
                        />
                      )}
                    >
                      <MenuItem value="ACTIVE">Active</MenuItem>
                      <MenuItem value="INACTIVE">Inactive</MenuItem>
                      <MenuItem value="REMOVED">Removed</MenuItem>
                    </Select>
                  </TableCell>
                  <TableCell>{u.createdOn ? new Date(u.createdOn).toLocaleDateString() : "—"}</TableCell>
                  <TableCell>
                    {u.grants.length === 0 ? (
                      <Typography variant="body2" color="text.secondary" fontStyle="italic">
                        No grants
                      </Typography>
                    ) : (
                      <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                        {u.grants.map((g) => (
                          <Chip
                            key={g.id}
                            size="small"
                            variant="outlined"
                            color={g.module === "SHARED" ? "default" : "primary"}
                            label={`${g.roleName} @ ${g.scopeType === "GLOBAL" ? "Global (ALL)" : g.scopeName || g.scopeType}`}
                          />
                        ))}
                      </Stack>
                    )}
                  </TableCell>
                  <TableCell align="right">
                    <IconButton size="small" onClick={() => setGrantEditorUser(u)} aria-label="Manage grants">
                      <Pencil size={15} />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        </>
      )}

      <AddUserDialog
        open={addOpen}
        roles={roles}
        existingUsers={users}
        onClose={() => setAddOpen(false)}
        onCreated={load}
      />
      <GrantEditorDialog
        open={!!grantEditorUser}
        user={grantEditorUser}
        roles={roles}
        onClose={() => setGrantEditorUser(null)}
        onChanged={load}
      />
    </Box>
  );
}
