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
  FormControl,
  FormLabel,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  Step,
  StepLabel,
  Stepper,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@wso2/oxygen-ui";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import {
  createGrant,
  createAdminUser,
  searchDirectory,
  type AdminUser,
  type DirectoryPerson,
  type Role,
} from "../api/adminApi";
import { dialogPaperSx } from "../cardStyles";
import GrantPicker, { type PendingGrant } from "../components/GrantPicker";

const STEPS = ["Find person", "Assign role"];

interface AddUserDialogProps {
  open: boolean;
  roles: Role[];
  // Platform users already provisioned — used to flag a search result that's
  // already registered, so re-selecting them doesn't look like creating a
  // duplicate. Cross-referenced by uuid.
  existingUsers: AdminUser[];
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
export default function AddUserDialog({
  open,
  roles,
  existingUsers,
  onClose,
  onCreated,
}: AddUserDialogProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [stage, setStage] = useState<1 | 2>(1);
  // Internal is the only wired path today — External accounts aren't
  // resolvable via the WSO2-org SCIM directory this search hits, and
  // provisioning them is a separate, not-yet-built flow (owned by the
  // Audit module's own onboarding work). Defaulting to Internal keeps the
  // common case a zero-click experience rather than forcing a choice first.
  const [userType, setUserType] = useState<"INTERNAL" | "EXTERNAL">("INTERNAL");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<DirectoryPerson[]>([]);
  const [searching, setSearching] = useState(false);
  const [selected, setSelected] = useState<DirectoryPerson | null>(null);
  const [pendingGrants, setPendingGrants] = useState<PendingGrantEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const reset = () => {
    setStage(1);
    setUserType("INTERNAL");
    setQuery("");
    setResults([]);
    setSelected(null);
    setPendingGrants([]);
    setError(null);
  };

  useEffect(() => {
    if (!open) reset();
  }, [open]);

  // Switching type mid-search invalidates whatever was found/selected under
  // the other type — clear rather than let a stale External-side selection
  // (impossible today, but future-proof) survive a toggle back to Internal.
  const handleUserTypeChange = (next: "INTERNAL" | "EXTERNAL") => {
    setUserType(next);
    setQuery("");
    setResults([]);
    setSelected(null);
  };

  // Debounced directory search — searchDirectory itself already refuses a
  // query under 2 characters, so the debounce timer is the only thing
  // guarding against a request per keystroke.
  useEffect(() => {
    if (stage !== 1 || userType !== "INTERNAL" || query.trim().length < 2) {
      setResults([]);
      setSearching(false);
      return;
    }
    // cancelled guards against a slower, superseded search resolving after a
    // newer one and overwriting its results — the debounce timer alone only
    // stops a request that hasn't fired yet, not one already in flight when
    // the query changes again.
    let cancelled = false;
    setSearching(true);
    const t = setTimeout(() => {
      searchDirectory(authFetch, query)
        .then((r) => {
          if (!cancelled) setResults(r);
        })
        .catch(() => {
          if (!cancelled) setResults([]);
        })
        .finally(() => {
          if (!cancelled) setSearching(false);
        });
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, stage, userType]);

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
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth PaperProps={{ sx: dialogPaperSx }}>
      <DialogTitle>
        Add User
        <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 400 }}>
          Provision someone from the WSO2 organization, then grant them a role.
        </Typography>
      </DialogTitle>
      <Box sx={{ px: 3 }}>
        <Stepper activeStep={stage - 1} sx={{ mb: 1 }}>
          {STEPS.map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>
      </Box>
      {/* Fixed height so the dialog doesn't grow/shrink as content changes —
          picking a person, switching stages, or a long results/grants list
          all scroll inside this box instead of resizing the dialog itself. */}
      <DialogContent sx={{ height: 300, overflowY: "auto" }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {stage === 1 && (
          <Box>
            <FormControl sx={{ mb: 2 }}>
              <FormLabel sx={{ mb: 0.75, fontWeight: 600, fontSize: "0.8rem" }}>User type</FormLabel>
              <ToggleButtonGroup
                value={userType}
                exclusive
                size="small"
                onChange={(_, next: "INTERNAL" | "EXTERNAL" | null) => next && handleUserTypeChange(next)}
                aria-label="User type"
              >
                <ToggleButton value="INTERNAL">Internal</ToggleButton>
                <ToggleButton value="EXTERNAL">External</ToggleButton>
              </ToggleButtonGroup>
              <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
                Internal: a WSO2-org employee, resolved via the SCIM directory below. External: not available in
                this console yet — that provisioning flow is being built separately.
              </Typography>
            </FormControl>

            {userType === "EXTERNAL" ? (
              <Alert severity="info">
                External user provisioning isn't available here yet — it's a separate flow, coming in a later
                update.
              </Alert>
            ) : selected ? (
              // Once someone is picked, the search box's own spot shows who —
              // no separate colored callout, just a neutral recap (matching
              // stage 2's look) with a way to search again.
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  bgcolor: "action.hover",
                  borderRadius: 1,
                  p: 1.25,
                  gap: 1,
                }}
              >
                <Avatar sx={{ width: 28, height: 28, fontSize: 12 }}>{initials(selected.displayName)}</Avatar>
                <Box sx={{ flex: 1 }}>
                  <Typography variant="body2" fontWeight={700}>
                    {selected.displayName}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {selected.email}
                  </Typography>
                </Box>
                <Button size="small" onClick={() => setSelected(null)}>
                  Change
                </Button>
              </Box>
            ) : (
              <>
                <TextField
                  autoFocus
                  fullWidth
                  size="small"
                  label="User"
                  placeholder="Search by name or email…"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  helperText="Search the user by name or email — only WSO2-org accounts are eligible."
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
                  <List
                    sx={{ border: results.length ? "1px solid" : "none", borderColor: "divider", borderRadius: 1 }}
                  >
                    {results.map((p) => {
                      const alreadyRegistered = existingUsers.some((u) => u.uuid === p.uuid);
                      return (
                        <ListItemButton
                          key={p.uuid}
                          disabled={alreadyRegistered}
                          onClick={() => setSelected(p)}
                        >
                          <Avatar sx={{ width: 28, height: 28, mr: 1.5, fontSize: 12 }}>
                            {initials(p.displayName)}
                          </Avatar>
                          <ListItemText primary={p.displayName} secondary={p.email} />
                          {alreadyRegistered && (
                            <Chip size="small" label="Already registered" color="default" variant="outlined" />
                          )}
                        </ListItemButton>
                      );
                    })}
                  </List>
                  {results.some((p) => existingUsers.some((u) => u.uuid === p.uuid)) && (
                    <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
                      "Already registered" means this person is already a platform user — open Manage Grants for
                      them from the Users list instead of adding them again here.
                    </Typography>
                  )}
                </Box>
              </>
            )}
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
            </Box>

            <Typography variant="body2" fontWeight={700} sx={{ mb: 0.25 }}>
              Grants to assign
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
              At least one role is required before this person can be created.
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
