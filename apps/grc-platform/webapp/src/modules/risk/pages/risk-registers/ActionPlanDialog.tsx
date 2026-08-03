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

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { resolveUserByEmail, searchEmployees } from "../../api/riskApi";
import type { EmployeeOption } from "../../api/riskApi";
import { dialogPaperSx } from "../cardStyles";

// Matches the floor used by the Standard action plan's own Action Owner
// picker (ActionPlanStep.tsx) — same live HR-entity search, so "any user"
// really means any WSO2 employee, not just existing grc-platform accounts.
const MIN_EMPLOYEE_SEARCH_LEN = 2;
const EMPLOYEE_SEARCH_DEBOUNCE_MS = 300;

export interface ActionPlanPayload {
  description: string;
  actionOwnerId: number | null;
  steps: string[];
}

interface ActionPlanDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: (payload: ActionPlanPayload) => Promise<void>;
}

// A step row, keyed by a stable local id — see the note by its useState call.
interface StepRow {
  id: number;
  text: string;
}

// Mirrors the Standard action plan form (ActionPlanStep.tsx) — description +
// a repeatable step list + an unrestricted Action Owner picker — but as a
// standalone dialog rather than a wizard step, since Management creates this
// after a risk is already ESCALATED, not at risk-creation time.
export default function ActionPlanDialog({
  open,
  onClose,
  onConfirm,
}: ActionPlanDialogProps): JSX.Element {
  const authFetch = useAuthApiClient();

  const [description, setDescription] = useState("");
  // Rows are keyed by a stable id (not array index): removing a row shifts
  // every later index, and an index key would make React reconcile the
  // reused DOM node by position rather than by which step it semantically
  // was — losing focus out of whatever field the user was typing in.
  const nextStepID = useRef(0);
  const newStepRow = (): StepRow => ({ id: nextStepID.current++, text: "" });
  const [steps, setSteps] = useState<StepRow[]>(() => [newStepRow()]);
  const [stepsError, setStepsError] = useState("");

  const [ownerOptions, setOwnerOptions] = useState<EmployeeOption[]>([]);
  const [ownerSelected, setOwnerSelected] = useState<EmployeeOption | null>(null);
  const [ownerId, setOwnerId] = useState<number | null>(null);
  const [ownerSearchLoading, setOwnerSearchLoading] = useState(false);
  const [ownerResolving, setOwnerResolving] = useState(false);
  const [ownerError, setOwnerError] = useState<string | null>(null);
  const ownerDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [apiError, setApiError] = useState("");

  const runOwnerSearch = useCallback(
    (query: string) => {
      if (query.trim().length < MIN_EMPLOYEE_SEARCH_LEN) {
        setOwnerOptions([]);
        return;
      }
      setOwnerSearchLoading(true);
      searchEmployees(authFetch, query)
        .then(setOwnerOptions)
        .catch(() => setOwnerError("Unable to reach the employee directory. Please try again."))
        .finally(() => setOwnerSearchLoading(false));
    },
    [authFetch],
  );

  function handleOwnerInputChange(value: string): void {
    if (ownerDebounce.current) clearTimeout(ownerDebounce.current);
    ownerDebounce.current = setTimeout(() => runOwnerSearch(value), EMPLOYEE_SEARCH_DEBOUNCE_MS);
  }

  function resetState(): void {
    // The dialog stays mounted between opens (the parent renders it
    // unconditionally, toggling only MUI's `open` prop), so a debounced
    // search left pending across a close/reopen would fire mid-way through
    // the next session and overwrite ownerOptions with stale results.
    if (ownerDebounce.current) clearTimeout(ownerDebounce.current);
    setDescription("");
    setSteps([newStepRow()]);
    setStepsError("");
    setOwnerSelected(null);
    setOwnerId(null);
    setOwnerError(null);
    setApiError("");
  }

  // Belt-and-suspenders: this dialog doesn't currently unmount in practice
  // (see resetState's comment), but clear the timer on unmount too in case
  // that ever changes — e.g. a future refactor to conditionally render this
  // dialog only while open.
  useEffect(() => {
    return () => {
      if (ownerDebounce.current) clearTimeout(ownerDebounce.current);
    };
  }, []);

  function handleClose(): void {
    if (submitting) return;
    resetState();
    onClose();
  }

  async function handleSubmit(): Promise<void> {
    const trimmedSteps = steps.map((s) => s.text.trim()).filter(Boolean);
    if (trimmedSteps.length === 0) {
      setStepsError("At least one action step is required.");
      return;
    }
    setStepsError("");
    setSubmitting(true);
    setApiError("");
    try {
      await onConfirm({ description: description.trim(), actionOwnerId: ownerId, steps: trimmedSteps });
      resetState();
      onClose();
    } catch (e: unknown) {
      setApiError(e instanceof Error ? e.message : "Unable to create the action plan. Please try again.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{ sx: dialogPaperSx }}
    >
      <DialogTitle>
        {/* component="span": DialogTitle already renders an <h2>; a nested
            heading-level Typography (its default element for variant="h6")
            is invalid HTML and trips a hydration warning. */}
        <Typography component="span" variant="h6" fontWeight={700} sx={{ display: "block" }}>
          Create Action Plan
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          This risk was escalated for being overdue. Lay out how Management wants it remediated.
        </Typography>
      </DialogTitle>
      <DialogContent>
        {apiError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {apiError}
          </Alert>
        )}
        <Stack gap={3} sx={{ pt: 1 }}>
          <TextField
            label="Action Plan Description"
            fullWidth
            multiline
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Summarise the management-directed remediation…"
            disabled={submitting}
            helperText="High level description of the plan (optional)"
          />

          <Autocomplete
            options={ownerOptions}
            loading={ownerSearchLoading || ownerResolving}
            filterOptions={(opts) => opts}
            getOptionLabel={(option) => option.name}
            isOptionEqualToValue={(option, value) => option.email === value.email}
            value={ownerSelected}
            disabled={submitting}
            onInputChange={(_, newInputValue, reason) => {
              if (reason === "input") handleOwnerInputChange(newInputValue);
            }}
            onChange={(_, newValue) => {
              if (!newValue) {
                setOwnerSelected(null);
                setOwnerId(null);
                return;
              }
              setOwnerResolving(true);
              resolveUserByEmail(authFetch, newValue)
                .then((resolved) => {
                  setOwnerSelected(newValue);
                  setOwnerId(resolved.id);
                  setOwnerError(null);
                })
                .catch(() => {
                  setOwnerSelected(null);
                  setOwnerId(null);
                  setOwnerError("Unable to link this employee to a user account. Please try again.");
                })
                .finally(() => setOwnerResolving(false));
            }}
            loadingText="Searching…"
            noOptionsText={ownerError ?? "Type at least 2 characters of the employee's email to search"}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Action Owner"
                placeholder="Search by email"
                error={!!ownerError}
                helperText={ownerError ?? "Person responsible for executing this plan (optional)."}
              />
            )}
          />

          <Box>
            <Typography variant="body2" fontWeight={500} color="text.primary" sx={{ mb: 1.5 }}>
              Action Steps
            </Typography>
            <Stack gap={1.5}>
              {steps.map((step, index) => (
                <Stack key={step.id} direction="row" gap={1} alignItems="flex-start">
                  <Typography
                    variant="body2"
                    fontWeight={600}
                    color="text.secondary"
                    sx={{ pt: 1.25, minWidth: 28, flexShrink: 0 }}
                  >
                    {index + 1}.
                  </Typography>
                  <TextField
                    fullWidth
                    size="small"
                    placeholder={`Describe action step ${index + 1}…`}
                    value={step.text}
                    disabled={submitting}
                    onChange={(e) => {
                      const next = [...steps];
                      next[index] = { ...next[index], text: e.target.value };
                      setSteps(next);
                      if (e.target.value && stepsError) setStepsError("");
                    }}
                    error={!!stepsError}
                  />
                  <IconButton
                    onClick={() => setSteps(steps.filter((s) => s.id !== step.id))}
                    disabled={submitting || steps.length === 1}
                    size="small"
                    sx={{ mt: 0.5, flexShrink: 0, color: "error.main" }}
                    aria-label={`Remove step ${index + 1}`}
                  >
                    <Trash2 size={16} />
                  </IconButton>
                </Stack>
              ))}
            </Stack>
            {stepsError && (
              <Typography variant="caption" color="error.main" sx={{ display: "block", mt: 1 }}>
                {stepsError}
              </Typography>
            )}
            <Button
              variant="outlined"
              size="small"
              startIcon={<Plus size={15} />}
              onClick={() => setSteps([...steps, newStepRow()])}
              disabled={submitting}
              sx={{ mt: 2 }}
            >
              Add Step
            </Button>
          </Box>
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={handleClose} disabled={submitting} color="inherit">
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={submitting} variant="contained">
          {submitting ? "Creating…" : "Create Plan"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
