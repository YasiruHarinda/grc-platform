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
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil, Plus, Search } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useEffect, useMemo, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { fetchAdminUsers, fetchRoles, type AdminUser, type Role } from "../api/adminApi";
import AddUserDialog from "./AddUserDialog";
import GrantEditorDialog from "./GrantEditorDialog";

export default function UsersPage(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [grantEditorUser, setGrantEditorUser] = useState<AdminUser | null>(null);

  const load = () => {
    setLoading(true);
    setError(null);
    Promise.all([fetchAdminUsers(authFetch), fetchRoles(authFetch)])
      .then(([u, r]) => {
        setUsers(u);
        setRoles(r);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load users"))
      .finally(() => setLoading(false));
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
                    <Chip
                      size="small"
                      label={u.status === "ACTIVE" ? "Active" : u.status === "INACTIVE" ? "Inactive" : "Removed"}
                      color={u.status === "ACTIVE" ? "success" : "default"}
                    />
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
