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
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { createGrant, revokeGrant, type AdminUser, type Role } from "../api/adminApi";
import { dialogPaperSx } from "../cardStyles";
import GrantPicker, { type PendingGrant } from "../components/GrantPicker";

interface GrantEditorDialogProps {
  open: boolean;
  user: AdminUser | null;
  roles: Role[];
  onClose: () => void;
  // Called after a successful add/revoke so the caller can refetch the user
  // list (grant ids and the up-to-date grant set live server-side; simplest
  // to just re-fetch than to reconcile local state).
  onChanged: () => void;
}

export default function GrantEditorDialog({ open, user, roles, onClose, onChanged }: GrantEditorDialogProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleAdd = async (grant: PendingGrant) => {
    if (!user) return;
    setError(null);
    setBusy(true);
    try {
      await createGrant(authFetch, user.id, grant);
      onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to add grant");
    } finally {
      setBusy(false);
    }
  };

  const handleRevoke = async (grantId: number) => {
    if (!user) return;
    setError(null);
    setBusy(true);
    try {
      await revokeGrant(authFetch, user.id, grantId);
      onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to revoke grant");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth PaperProps={{ sx: dialogPaperSx }}>
      <DialogTitle>
        Manage Grants
        <Typography variant="body2" color="text.secondary">
          {user ? `${user.displayName || user.email || user.uuid}` : ""}
        </Typography>
      </DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {!user?.grants.length ? (
          <Typography variant="body2" color="text.secondary" fontStyle="italic">
            No grants yet — add one below.
          </Typography>
        ) : (
          <Stack spacing={1}>
            {user.grants.map((g) => (
              <Box
                key={g.id}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  py: 0.5,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                }}
              >
                <Chip
                  size="small"
                  label={`${g.roleName} @ ${g.scopeType === "GLOBAL" ? "Global (ALL)" : g.scopeName || g.scopeType}`}
                  color={g.module === "SHARED" ? "default" : "primary"}
                  variant="outlined"
                />
                <IconButton size="small" onClick={() => handleRevoke(g.id)} disabled={busy} aria-label="Revoke grant">
                  <X size={16} />
                </IconButton>
              </Box>
            ))}
          </Stack>
        )}

        <GrantPicker roles={roles} onAdd={handleAdd} />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}
