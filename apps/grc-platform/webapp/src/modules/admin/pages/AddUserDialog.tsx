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
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { createGrant, createAdminUser, searchDirectory, type DirectoryPerson, type Role } from "../api/adminApi";
import GrantPicker, { type PendingGrant } from "../components/GrantPicker";

interface AddUserDialogProps {
  open: boolean;
  roles: Role[];
  onClose: () => void;
  // Called after the user (and every pending grant) is successfully created,
  // so the caller can refetch the user list.
  onCreated: () => void;
}

interface PendingGrantEntry extends PendingGrant {
  label: string;
}

// Two-stage flow: find a WSO2-org person, then assign one or more initial
// grants. Two separate API calls under the hood (provision, then grant per
// pending entry) — not one combined payload, matching the entity's own
// separation of "create the user" from "grant a role" (see
// ADMIN_CONSOLE_DESIGN.md §2).
export default function AddUserDialog({ open, roles, onClose, onCreated }: AddUserDialogProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [stage, setStage] = useState<1 | 2>(1);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<DirectoryPerson[]>([]);
  const [searching, setSearching] = useState(false);
  const [selected, setSelected] = useState<DirectoryPerson | null>(null);
  const [pendingGrants, setPendingGrants] = useState<PendingGrantEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const reset = () => {
    setStage(1);
    setQuery("");
    setResults([]);
    setSelected(null);
    setPendingGrants([]);
    setError(null);
  };

  useEffect(() => {
    if (!open) reset();
  }, [open]);

  // Debounced directory search — searchDirectory itself already refuses a
  // query under 2 characters, so the debounce timer is the only thing
  // guarding against a request per keystroke.
  useEffect(() => {
    if (stage !== 1 || query.trim().length < 2) {
      setResults([]);
      return;
    }
    setSearching(true);
    const t = setTimeout(() => {
      searchDirectory(authFetch, query)
        .then(setResults)
        .catch(() => setResults([]))
        .finally(() => setSearching(false));
    }, 300);
    return () => clearTimeout(t);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, stage]);

  const handleAddGrant = (grant: PendingGrant, label: string) => {
    setPendingGrants((prev) => [...prev, { ...grant, label }]);
  };

  const handleRemoveGrant = (idx: number) => {
    setPendingGrants((prev) => prev.filter((_, i) => i !== idx));
  };

  const handleFinish = async () => {
    if (!selected) return;
    setError(null);
    setSubmitting(true);
    try {
      const created = await createAdminUser(authFetch, selected.uuid);
      for (const g of pendingGrants) {
        await createGrant(authFetch, created.id, g);
      }
      onCreated();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create user");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        Add User
        <Typography variant="body2" color="text.secondary">
          Provision someone from the WSO2 organization, then grant them a role.
        </Typography>
      </DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {stage === 1 && (
          <Box>
            <TextField
              autoFocus
              fullWidth
              size="small"
              label="Search the WSO2 organization"
              placeholder="Search by name or email…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              helperText="Only people resolvable via the SCIM directory (a real Asgardeo account) are eligible."
            />
            <Box sx={{ mt: 1.5, minHeight: 60 }}>
              {searching && (
                <Box sx={{ display: "flex", justifyContent: "center", py: 2 }}>
                  <CircularProgress size={20} />
                </Box>
              )}
              {!searching && query.trim().length >= 2 && results.length === 0 && (
                <Typography variant="body2" color="text.secondary" fontStyle="italic">
                  No matches in the WSO2 organization.
                </Typography>
              )}
              <List sx={{ border: results.length ? "1px solid" : "none", borderColor: "divider", borderRadius: 1 }}>
                {results.map((p) => (
                  <ListItemButton
                    key={p.uuid}
                    selected={selected?.uuid === p.uuid}
                    onClick={() => setSelected(p)}
                  >
                    <Avatar sx={{ width: 28, height: 28, mr: 1.5, fontSize: 12 }}>
                      {initials(p.displayName)}
                    </Avatar>
                    <ListItemText primary={p.displayName} secondary={p.email} />
                  </ListItemButton>
                ))}
              </List>
            </Box>
          </Box>
        )}

        {stage === 2 && selected && (
          <Box>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                bgcolor: "action.hover",
                borderRadius: 1,
                p: 1.5,
                mb: 2,
              }}
            >
              <Avatar sx={{ width: 28, height: 28, mr: 1.5, fontSize: 12 }}>{initials(selected.displayName)}</Avatar>
              <Box sx={{ flex: 1 }}>
                <Typography variant="body2" fontWeight={700}>
                  {selected.displayName}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {selected.email}
                </Typography>
              </Box>
              <Button size="small" onClick={() => setStage(1)}>
                Change
              </Button>
            </Box>

            <Typography variant="body2" fontWeight={700} sx={{ mb: 1 }}>
              Grants to assign
            </Typography>
            {!pendingGrants.length ? (
              <Typography variant="body2" color="text.secondary" fontStyle="italic">
                No grants added yet — add at least one below before creating.
              </Typography>
            ) : (
              <Stack spacing={1}>
                {pendingGrants.map((g, i) => (
                  <Box key={i} sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                    <Chip size="small" label={g.label} variant="outlined" />
                    <Button size="small" color="error" onClick={() => handleRemoveGrant(i)}>
                      Remove
                    </Button>
                  </Box>
                ))}
              </Stack>
            )}
            <GrantPicker roles={roles} onAdd={handleAddGrant} />
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        {stage === 2 && <Button onClick={() => setStage(1)} sx={{ mr: "auto" }}>← Back</Button>}
        <Button onClick={onClose}>Cancel</Button>
        {stage === 1 && (
          <Button variant="contained" disabled={!selected} onClick={() => setStage(2)}>
            Next →
          </Button>
        )}
        {stage === 2 && (
          <Button
            variant="contained"
            disabled={!pendingGrants.length || submitting}
            onClick={handleFinish}
          >
            {submitting ? "Creating…" : "Create User & Grant"}
          </Button>
        )}
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
