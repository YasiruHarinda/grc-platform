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
  Avatar,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  Paper,
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
      <DialogTitle sx={{ pb: 1.5 }}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="h6" component="span" fontWeight={700} lineHeight={1.3} sx={{ flex: 1, minWidth: 0 }}>
            Manage Grants
          </Typography>
          <IconButton size="small" onClick={onClose} aria-label="Close" sx={{ mr: -0.5 }}>
            <X size={16} />
          </IconButton>
        </Stack>
      </DialogTitle>
      <Divider />

      <DialogContent sx={{ pt: 2.5 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {user && (
          <Paper
            variant="outlined"
            sx={{ display: "flex", alignItems: "center", gap: 1.5, borderRadius: 1.5, p: 1.5, bgcolor: "action.hover" }}
          >
            <Avatar sx={{ width: 36, height: 36, fontSize: 13, flexShrink: 0 }}>
              {initials(user.displayName || user.email || "?")}
            </Avatar>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" fontWeight={700} noWrap>
                {user.displayName || user.email || user.uuid}
              </Typography>
              {user.email && (
                <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
                  {user.email}
                </Typography>
              )}
            </Box>
          </Paper>
        )}

        <Stack direction="row" alignItems="center" spacing={1} sx={{ mt: 2.5, mb: 1.25 }}>
          <Typography variant="body2" fontWeight={700}>
            Current grants
          </Typography>
          {!!user?.grants.length && <Chip size="small" label={user.grants.length} />}
        </Stack>

        {!user?.grants.length ? (
          <Box
            sx={{
              border: "1px dashed",
              borderColor: "divider",
              borderRadius: 1.5,
              px: 2,
              py: 2.25,
              textAlign: "center",
            }}
          >
            <Typography variant="body2" color="text.secondary">
              No grants yet
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Pick a role and scope below, then select Add.
            </Typography>
          </Box>
        ) : (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
            {user.grants.map((g) => (
              <Chip
                key={g.id}
                size="small"
                variant="outlined"
                color={g.module === "SHARED" ? "default" : "primary"}
                label={`${g.roleName} @ ${g.scopeType === "GLOBAL" ? "Global (ALL)" : g.scopeName || g.scopeType}`}
                disabled={busy}
                onDelete={() => handleRevoke(g.id)}
              />
            ))}
          </Stack>
        )}

        <Divider sx={{ mt: 2.5, mb: 1.5 }} />
        <Typography variant="body2" fontWeight={700} sx={{ mb: 0.5 }}>
          Add a grant
        </Typography>
        <GrantPicker roles={roles} onAdd={handleAdd} userType={user?.userType} />
      </DialogContent>

      <Divider />
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}

function initials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .map((s) => s[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}
