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
  alpha,
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
  Divider,
  FormLabel,
  IconButton,
  InputAdornment,
  List,
  ListItemButton,
  ListItemText,
  Paper,
  Skeleton,
  Stack,
  Step,
  StepLabel,
  Stepper,
  type SxProps,
  TextField,
  type Theme,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Building2,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Search,
  SearchX,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { type JSX, type ReactNode, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import {
  createGrant,
  createAdminUser,
  searchDirectory,
  searchExternalDirectory,
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
// separation of "create the user" from "grant a role".
export default function AddUserDialog({
  open,
  roles,
  existingUsers,
  onClose,
  onCreated,
}: AddUserDialogProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [stage, setStage] = useState<1 | 2>(1);
  // Internal is the common case — most people added here are WSO2-org
  // employees — so it's the zero-click default rather than forcing a choice.
  const [userType, setUserType] = useState<"INTERNAL" | "EXTERNAL">("INTERNAL");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<DirectoryPerson[]>([]);
  const [searching, setSearching] = useState(false);
  const [selected, setSelected] = useState<DirectoryPerson | null>(null);
  const [pendingGrants, setPendingGrants] = useState<PendingGrantEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // Set once createAdminUser succeeds. User creation and each grant are
  // separate backend calls with no shared transaction — tracking this lets a
  // retry after a failed grant skip re-provisioning the (already-created)
  // user and only replay the grants that didn't go through.
  const [createdUserId, setCreatedUserId] = useState<number | null>(null);

  const reset = () => {
    setStage(1);
    setUserType("INTERNAL");
    setQuery("");
    setResults([]);
    setSelected(null);
    setPendingGrants([]);
    setError(null);
    setCreatedUserId(null);
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

  // Debounced directory search — Internal hits the cached WSO2-org snapshot,
  // External is a live call against the external auditor organization (see
  // searchExternalDirectory's doc comment for why it can't be cached the same
  // way). Both already refuse a query under 2 characters, so the debounce
  // timer is the only thing guarding against a request per keystroke.
  useEffect(() => {
    if (stage !== 1 || query.trim().length < 2) {
      setResults([]);
      setSearching(false);
      return;
    }
    // cancelled guards against a slower, superseded search resolving after a
    // newer one and overwriting its results — the debounce timer alone only
    // stops a request that hasn't fired yet, not one already in flight when
    // the query changes again.
    let cancelled = false;
    const search = userType === "EXTERNAL" ? searchExternalDirectory : searchDirectory;
    setSearching(true);
    const t = setTimeout(() => {
      search(authFetch, query)
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
    let userId = createdUserId;
    try {
      if (userId === null) {
        const created = await createAdminUser(authFetch, selected.uuid, userType);
        userId = created.id;
        setCreatedUserId(userId);
      }
      // Grants are applied one at a time, and removed from pendingGrants as
      // each succeeds — so if one fails partway, only the grants that never
      // went through remain in the list for a retry.
      let remaining = pendingGrants;
      while (remaining.length) {
        await createGrant(authFetch, userId, remaining[0]);
        remaining = remaining.slice(1);
        setPendingGrants(remaining);
      }
      onCreated();
      onClose();
    } catch (e) {
      // The user (and any grants that succeeded before the failure) are
      // already persisted server-side even though this call failed —
      // refresh the caller's list so that isn't invisible, and leave the
      // dialog open so the admin can retry just the remaining grants.
      if (userId !== null) onCreated();
      const msg = e instanceof Error ? e.message : "Failed to create user";
      setError(userId !== null ? `User created, but a grant failed: ${msg}. Retry to apply the remaining grants.` : msg);
    } finally {
      setSubmitting(false);
    }
  };

  const trimmed = query.trim();
  const hasRegisteredResult = results.some((p) => existingUsers.some((u) => u.uuid === p.uuid));

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth PaperProps={{ sx: dialogPaperSx }}>
      <DialogTitle sx={{ pb: 1.5 }}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="h6" component="span" fontWeight={700} lineHeight={1.3} sx={{ flex: 1, minWidth: 0 }}>
            Add User
          </Typography>
          <IconButton size="small" onClick={onClose} aria-label="Close" sx={{ mr: -0.5 }}>
            <X size={16} />
          </IconButton>
        </Stack>
      </DialogTitle>

      <Box sx={{ px: 3, pb: 2 }}>
        <Stepper activeStep={stage - 1}>
          {STEPS.map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>
      </Box>
      <Divider />

      {/* Fixed height so the dialog doesn't grow/shrink as content changes —
          picking a person, switching stages, or a long results/grants list
          all scroll inside this box instead of resizing the dialog itself. */}
      <DialogContent sx={{ height: 360, overflowY: "auto", pt: 2.5 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {stage === 1 && (
          <Box>
            <FormLabel component="legend" sx={{ mb: 1, fontWeight: 700, fontSize: "0.75rem", display: "block" }}>
              User type
            </FormLabel>
            <ToggleButtonGroup
              value={userType}
              exclusive
              fullWidth
              size="small"
              onChange={(_, next: "INTERNAL" | "EXTERNAL" | null) => next && handleUserTypeChange(next)}
              aria-label="User type"
              sx={{ mb: 2.5 }}
            >
              <ToggleButton value="INTERNAL" sx={userTypeButtonSx}>
                <Stack direction="row" spacing={0.75} alignItems="center">
                  <Building2 size={14} />
                  <Typography variant="body2" fontWeight={700}>
                    Internal
                  </Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary">
                  A WSO2-org employee.
                </Typography>
              </ToggleButton>
              <ToggleButton value="EXTERNAL" sx={userTypeButtonSx}>
                <Stack direction="row" spacing={0.75} alignItems="center">
                  <ExternalLink size={14} />
                  <Typography variant="body2" fontWeight={700}>
                    External
                  </Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary">
                  An external auditor.
                </Typography>
              </ToggleButton>
            </ToggleButtonGroup>

            {selected ? (
              // Once someone is picked, the search box's own spot shows who —
              // no separate colored callout, just a neutral recap (matching
              // stage 2's look) with a way to search again.
              <PersonCard
                person={selected}
                trailing={
                  // Disabled once the user is actually created (createdUserId
                  // set) — swapping the selection at that point would leave a
                  // retry granting roles to the wrong, already-provisioned
                  // person.
                  <Button size="small" disabled={createdUserId !== null} onClick={() => setSelected(null)}>
                    Change
                  </Button>
                }
              />
            ) : (
              <>
                <TextField
                  autoFocus
                  fullWidth
                  size="small"
                  type="search"
                  name="directory-search"
                  autoComplete="off"
                  sx={searchFieldSx}
                  placeholder={
                    userType === "INTERNAL"
                      ? "Search the internal user directory by name or email…"
                      : "Search the external user directory by name or email…"
                  }
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  helperText={
                    userType === "INTERNAL"
                      ? "Only accounts in the internal organization are eligible."
                      : "Only accounts in the external organization are eligible."
                  }
                  slotProps={{
                    input: {
                      startAdornment: (
                        <InputAdornment position="start">
                          <Search size={15} />
                        </InputAdornment>
                      ),
                      endAdornment: query ? (
                        <InputAdornment position="end">
                          <IconButton size="small" aria-label="Clear search" onClick={() => setQuery("")}>
                            <X size={14} />
                          </IconButton>
                        </InputAdornment>
                      ) : undefined,
                    },
                  }}
                />

                <Box sx={{ mt: 1.5 }}>
                  <Paper variant="outlined" sx={{ borderRadius: 1.5, maxHeight: 224, overflowY: "auto" }}>
                    {searching ? (
                      <Stack divider={<Divider />}>
                        {[0, 1, 2].map((i) => (
                          <Stack key={i} direction="row" spacing={1.5} alignItems="center" sx={{ px: 1.75, py: 1.25 }}>
                            <Skeleton variant="circular" width={32} height={32} />
                            <Box sx={{ flex: 1 }}>
                              <Skeleton variant="text" width="42%" height={14} />
                              <Skeleton variant="text" width="62%" height={12} />
                            </Box>
                          </Stack>
                        ))}
                      </Stack>
                    ) : trimmed.length < 2 ? (
                      <EmptyPanel
                        icon={<Search size={20} />}
                        title="Search the directory"
                        detail="Type at least two characters to look someone up."
                      />
                    ) : results.length === 0 ? (
                      <EmptyPanel
                        icon={<SearchX size={20} />}
                        title={`No matches for “${trimmed}”`}
                        detail="Check the spelling, or try an email address instead."
                      />
                    ) : (
                      <List
                        component="div"
                        disablePadding
                      >
                        {results.map((p, i) => {
                          const alreadyRegistered = existingUsers.some((u) => u.uuid === p.uuid);
                          return (
                            <Box key={p.uuid}>
                              {i > 0 && <Divider />}
                              <ListItemButton
                                disabled={alreadyRegistered}
                                onClick={() => setSelected(p)}
                                sx={{ px: 1.75, py: 1.25, gap: 1.5 }}
                              >
                                <Avatar sx={{ width: 32, height: 32, fontSize: 12, flexShrink: 0 }}>
                                  {initials(p.displayName)}
                                </Avatar>
                                <ListItemText
                                  primary={p.displayName}
                                  secondary={p.email}
                                  slotProps={{
                                    primary: { variant: "body2", fontWeight: 600, noWrap: true },
                                    secondary: { variant: "caption", noWrap: true },
                                  }}
                                  sx={{ my: 0, minWidth: 0 }}
                                />
                                {alreadyRegistered ? (
                                  <Chip size="small" label="Already registered" variant="outlined" />
                                ) : (
                                  <ChevronRight size={15} opacity={0.4} />
                                )}
                              </ListItemButton>
                            </Box>
                          );
                        })}
                      </List>
                    )}
                  </Paper>
                  {hasRegisteredResult && (
                    <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.75 }}>
                      The user is already a platform user. Open Manage Grants for them from the Users list
                      instead of adding them again here.
                    </Typography>
                  )}
                </Box>
              </>
            )}
          </Box>
        )}

        {stage === 2 && selected && (
          <Box>
            <PersonCard
              person={selected}
              trailing={
                <Chip
                  size="small"
                  variant="outlined"
                  icon={userType === "INTERNAL" ? <Building2 size={12} /> : <ExternalLink size={12} />}
                  label={userType === "INTERNAL" ? "Internal" : "External"}
                />
              }
            />

            <Stack direction="row" alignItems="center" spacing={1} sx={{ mt: 2.5, mb: 1.25 }}>
              <Typography variant="body2" fontWeight={700}>
                Grants to assign
              </Typography>
              {pendingGrants.length > 0 && <Chip size="small" label={pendingGrants.length} />}
            </Stack>

            {!pendingGrants.length ? (
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
                  Pick a role and scope below, then select Add. At least one is required.
                </Typography>
              </Box>
            ) : (
              <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                {pendingGrants.map((g, i) => (
                  <Chip
                    key={i}
                    size="small"
                    variant="outlined"
                    label={g.label}
                    onDelete={() => handleRemoveGrant(i)}
                  />
                ))}
              </Stack>
            )}

            <Divider sx={{ mt: 2.5, mb: 1.5 }} />
            <Typography variant="body2" fontWeight={700} sx={{ mb: 0.5 }}>
              Add a grant
            </Typography>
            <GrantPicker roles={roles} onAdd={handleAddGrant} userType={userType} />
          </Box>
        )}
      </DialogContent>

      <Divider />
      <DialogActions sx={{ px: 3, py: 2 }}>
        {stage === 2 && (
          <Button startIcon={<ChevronLeft size={14} />} onClick={() => setStage(1)} sx={{ mr: "auto" }}>
            Back
          </Button>
        )}
        <Button onClick={onClose}>Cancel</Button>
        {stage === 1 && (
          <Button
            variant="contained"
            endIcon={<ChevronRight size={14} />}
            disabled={!selected}
            onClick={() => setStage(2)}
          >
            Next
          </Button>
        )}
        {stage === 2 && (
          <Button
            variant="contained"
            disabled={!pendingGrants.length || submitting}
            onClick={handleFinish}
            startIcon={submitting ? <CircularProgress size={14} color="inherit" /> : undefined}
          >
            {submitting ? "Creating…" : createdUserId !== null ? "Retry remaining grants" : "Create User & Grant"}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}

// Two things wash the search box in colour mid-typing, and neither is wanted
// here: the browser paints its own yellow/blue fill over any field it guesses
// is a username (autoComplete="off" alone doesn't stop the tint — only holding
// the transition off does), and the theme's focus state jumps the outline to a
// hard 2px orange. Focus stays visible as a 1px primary border with a soft
// tint ring instead.
// Keeps the browser from painting its own blue autofill wash over the value.
const searchFieldSx: SxProps<Theme> = {
  "& input[type='search']::-webkit-search-cancel-button": { display: "none" },
  "& input:-webkit-autofill, & input:-webkit-autofill:hover, & input:-webkit-autofill:focus": {
    WebkitTextFillColor: "inherit",
    WebkitBoxShadow: "none",
    transition: "background-color 600000s 0s",
  },
  "& input:-internal-autofill-selected": {
    WebkitTextFillColor: "inherit",
    WebkitBoxShadow: "none",
    backgroundColor: "transparent !important",
  },
  "& input::selection": {
    backgroundColor: (theme) => alpha(theme.palette.primary.main, 0.3),
  },
  "& .MuiOutlinedInput-root": {
    transition: "box-shadow 120ms ease",
    "&.Mui-focused": {
      boxShadow: (theme) => `0 0 0 3px ${alpha(theme.palette.primary.main, 0.14)}`,
      "& .MuiOutlinedInput-notchedOutline": { borderWidth: 1, borderColor: "primary.main" },
    },
  },
};

const userTypeButtonSx = {
  flexDirection: "column",
  alignItems: "flex-start",
  textAlign: "left",
  textTransform: "none",
  gap: 0.25,
  px: 1.5,
  py: 1,
};

function PersonCard({ person, trailing }: { person: DirectoryPerson; trailing?: ReactNode }): JSX.Element {
  return (
    <Paper
      variant="outlined"
      sx={{ display: "flex", alignItems: "center", gap: 1.5, borderRadius: 1.5, p: 1.5, bgcolor: "action.hover" }}
    >
      <Avatar sx={{ width: 36, height: 36, fontSize: 13, flexShrink: 0 }}>{initials(person.displayName)}</Avatar>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="body2" fontWeight={700} noWrap>
          {person.displayName}
        </Typography>
        <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
          {person.email}
        </Typography>
      </Box>
      {trailing}
    </Paper>
  );
}

function EmptyPanel({ icon, title, detail }: { icon: ReactNode; title: string; detail: string }): JSX.Element {
  return (
    <Stack alignItems="center" spacing={0.5} sx={{ px: 3, py: 3.5, textAlign: "center", color: "text.secondary" }}>
      <Box sx={{ display: "flex", opacity: 0.5 }}>{icon}</Box>
      <Typography variant="body2" fontWeight={600}>
        {title}
      </Typography>
      <Typography variant="caption">{detail}</Typography>
    </Stack>
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
