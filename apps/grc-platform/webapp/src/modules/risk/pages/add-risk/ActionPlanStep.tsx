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
import { Controller, useFieldArray, useFormContext, useWatch } from "react-hook-form";
import type { FieldPath } from "react-hook-form";
import {
  Autocomplete,
  Box,
  Button,
  ComplexSelect,
  Divider,
  FormHelperText,
  IconButton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import type { JSX, ReactNode } from "react";
import EvidenceAttachments from "@components/evidence-attachments/EvidenceAttachments";
import type { AddRiskFormValues } from "./types";
import { TREATMENT_STRATEGIES } from "./constants";
import { resolveUserByEmail, searchEmployees } from "../../api/riskApi";
import type { EmployeeOption, RiskTeam, UserOption } from "../../api/riskApi";
import { useAuthApiClient } from "@hooks/useAuthApiClient";

// Minimum characters before searching — matches the backend's own floor.
const MIN_EMPLOYEE_SEARCH_LEN = 2;
const EMPLOYEE_SEARCH_DEBOUNCE_MS = 300;

function FieldLabel({ children }: { children: ReactNode }): JSX.Element {
  return (
    <Typography
      variant="body2"
      fontWeight={500}
      color="text.primary"
      sx={{ display: "block", mb: 1 }}
    >
      {children}
    </Typography>
  );
}

function SectionHeader({ title }: { title: string }): JSX.Element {
  return (
    <Box>
      <Typography variant="subtitle1" fontWeight={600} color="text.primary">
        {title}
      </Typography>
      <Divider sx={{ mt: 1 }} />
    </Box>
  );
}

interface ActionPlanStepProps {
  assignmentTeams: RiskTeam[];
  users: UserOption[];
}

export default function ActionPlanStep({ assignmentTeams, users }: ActionPlanStepProps): JSX.Element {
  const { control, setValue, clearErrors } = useFormContext<AddRiskFormValues>();
  const authFetch = useAuthApiClient();

  const { fields, append, remove } = useFieldArray({ control, name: "actionSteps" });

  const evidenceAttachments = useWatch({ control, name: "evidenceAttachments" });

  // Risk Owner is restricted to users already belonging (via risk_team_id) to
  // either the source register team (picked in Step 1) or this assignment
  // team — unlike Action Owner, Risk Owner must stay a real, already-provisioned
  // grc-platform account (see conversation: HR entity employees don't
  // automatically get platform access, so they're not eligible here).
  const sourceRegister = useWatch({ control, name: "sourceRegister" });
  const assignmentTeam = useWatch({ control, name: "assignmentTeam" });
  const eligibleTeamIds = [sourceRegister, assignmentTeam].filter(
    (id): id is number => typeof id === "number",
  );
  const eligibleRiskOwners = users.filter(
    (u) => u.risk_team_id !== null && eligibleTeamIds.includes(u.risk_team_id),
  );

  // Clear a previously-selected Risk Owner if changing the source register or
  // assignment team makes them no longer eligible — avoids submitting a
  // riskOwner value that's silently stale relative to the visible options.
  const riskOwner = useWatch({ control, name: "riskOwner" });
  useEffect(() => {
    if (riskOwner !== "" && !eligibleRiskOwners.some((u) => u.id === riskOwner)) {
      setValue("riskOwner", "", { shouldDirty: false });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only re-check when the eligible team ids change
  }, [eligibleTeamIds.join(",")]);

  // Action Owner can be any employee, not just an existing grc-platform
  // user, so — like "Risk Identified By: Employee" — options are searched
  // live against the HR entity rather than a pre-fetched list. Unlike that
  // field, action_owner_id is a real FK, so on selection we resolve the
  // chosen employee to an internal user.id (creating the user row on the
  // fly if needed) via resolveUserByEmail before setting the form value.
  const [actionOwnerOptions, setActionOwnerOptions] = useState<EmployeeOption[]>([]);
  const [actionOwnerSelected, setActionOwnerSelected] = useState<EmployeeOption | null>(null);
  const [actionOwnerSearchLoading, setActionOwnerSearchLoading] = useState(false);
  const [actionOwnerResolving, setActionOwnerResolving] = useState(false);
  const [actionOwnerError, setActionOwnerError] = useState<string | null>(null);
  const actionOwnerDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  const runActionOwnerSearch = useCallback((query: string) => {
    if (query.trim().length < MIN_EMPLOYEE_SEARCH_LEN) {
      setActionOwnerOptions([]);
      setActionOwnerError(null);
      return;
    }
    setActionOwnerSearchLoading(true);
    setActionOwnerError(null);
    searchEmployees(authFetch, query)
      .then(setActionOwnerOptions)
      .catch(() => {
        setActionOwnerOptions([]);
        setActionOwnerError("Unable to reach the employee directory. Please try again.");
      })
      .finally(() => setActionOwnerSearchLoading(false));
  }, [authFetch]);

  const handleActionOwnerInputChange = (value: string): void => {
    if (actionOwnerDebounce.current) clearTimeout(actionOwnerDebounce.current);
    actionOwnerDebounce.current = setTimeout(() => runActionOwnerSearch(value), EMPLOYEE_SEARCH_DEBOUNCE_MS);
  };

  return (
    <Stack gap={4}>

      {/* ── Assignment ──────────────────────────────────────────────────────── */}
      <Stack gap={3}>
        <SectionHeader title="Assignment" />

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            gap: 2,
            alignItems: "flex-start",
          }}
        >
          {/* Assignment Team */}
          <Controller
            name="assignmentTeam"
            control={control}
            render={({ field, fieldState }) => (
              <Box>
                <FieldLabel>Assignment Team</FieldLabel>
                <ComplexSelect
                  {...field}
                  fullWidth
                  error={!!fieldState.error}
                  displayEmpty
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("assignmentTeam");
                  }}
                >
                  <ComplexSelect.MenuItem value="" disabled sx={{ display: "none" }}>
                    Select a team
                  </ComplexSelect.MenuItem>
                  {assignmentTeams.map((t) => (
                    <ComplexSelect.MenuItem key={t.id} value={t.id}>
                      {t.name}
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                {fieldState.error && (
                  <FormHelperText error>{fieldState.error.message}</FormHelperText>
                )}
              </Box>
            )}
          />

          {/* Risk Owner */}
          <Controller
            name="riskOwner"
            control={control}
            render={({ field, fieldState }) => (
              <Box>
                <FieldLabel>Risk Owner</FieldLabel>
                <ComplexSelect
                  {...field}
                  fullWidth
                  error={!!fieldState.error}
                  displayEmpty
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("riskOwner");
                  }}
                >
                  <ComplexSelect.MenuItem value="" disabled sx={{ display: "none" }}>
                    Select a risk owner
                  </ComplexSelect.MenuItem>
                  {eligibleRiskOwners.map((u) => (
                    <ComplexSelect.MenuItem key={u.id} value={u.id}>
                      {u.display_name}
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                {fieldState.error ? (
                  <FormHelperText error>{fieldState.error.message}</FormHelperText>
                ) : eligibleTeamIds.length > 0 && eligibleRiskOwners.length === 0 ? (
                  <FormHelperText error>
                    No users are assigned to the selected team(s) yet. Contact an admin to assign team membership.
                  </FormHelperText>
                ) : (
                  <FormHelperText>Person accountable for managing this risk.</FormHelperText>
                )}
              </Box>
            )}
          />
        </Box>

        {/* Action Owner */}
        <Controller
          name="actionOwner"
          control={control}
          render={({ field, fieldState }) => (
            <Autocomplete
              options={actionOwnerOptions}
              loading={actionOwnerSearchLoading || actionOwnerResolving}
              filterOptions={(opts) => opts}
              getOptionLabel={(option) => option.name}
              isOptionEqualToValue={(option, value) => option.email === value.email}
              value={actionOwnerSelected}
              onInputChange={(_, newInputValue, reason) => {
                if (reason === "input") handleActionOwnerInputChange(newInputValue);
              }}
              onChange={(_, newValue) => {
                if (!newValue) {
                  setActionOwnerSelected(null);
                  field.onChange("");
                  return;
                }
                setActionOwnerResolving(true);
                resolveUserByEmail(authFetch, newValue)
                  .then((resolved) => {
                    setActionOwnerSelected(newValue);
                    field.onChange(resolved.id);
                    clearErrors("actionOwner");
                  })
                  .catch(() => {
                    setActionOwnerSelected(null);
                    field.onChange("");
                    setActionOwnerError("Unable to link this employee to a user account. Please try again.");
                  })
                  .finally(() => setActionOwnerResolving(false));
              }}
              loadingText="Searching…"
              noOptionsText={
                actionOwnerError ??
                "Type at least 2 characters of the employee's email to search"
              }
              slotProps={{
                paper: {
                  sx: {
                    backdropFilter: "none",
                    backgroundColor: "#fff",
                    "[data-color-scheme='dark'] &": {
                      backgroundColor: "#1e1e1e",
                    },
                  },
                },
                listbox: {
                  sx: {
                    "& .MuiAutocomplete-option:hover, & .MuiAutocomplete-option[data-focus='true'], & .MuiAutocomplete-option.Mui-focused": {
                      backgroundColor: "rgba(var(--oxygen-palette-primary-mainChannel) / 0.08)",
                    },
                    "& .MuiAutocomplete-option[aria-selected='true']": {
                      backgroundColor: "rgba(var(--oxygen-palette-primary-mainChannel) / 0.16)",
                    },
                    "& .MuiAutocomplete-option[aria-selected='true'].Mui-focused, & .MuiAutocomplete-option[aria-selected='true'][data-focus='true']": {
                      backgroundColor: "rgba(var(--oxygen-palette-primary-mainChannel) / 0.24)",
                    },
                  },
                },
              }}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label="Action Owner"
                  placeholder="Search by email"
                  error={!!fieldState.error || !!actionOwnerError}
                  helperText={
                    fieldState.error?.message ?? actionOwnerError ?? "Person responsible for executing the action plan."
                  }
                  onBlur={field.onBlur}
                />
              )}
            />
          )}
        />
      </Stack>

      {/* ── Action Plan ─────────────────────────────────────────────────────── */}
      <Stack gap={3}>
        <SectionHeader title="Action Plan" />

        {/* Action Plan Description */}
        <Controller
          name="actionPlanDescription"
          control={control}
          render={({ field, fieldState }) => (
            <TextField
              {...field}
              label="Action Plan Description"
              fullWidth
              multiline
              rows={3}
              placeholder="Summarise the overall approach for treating this risk…"
              error={!!fieldState.error}
              helperText={fieldState.error?.message ?? "High level description of the plan (Optional)"}
            />
          )}
        />

        {/* Action Steps */}
        <Box>
          <Typography variant="body2" fontWeight={500} color="text.primary" sx={{ mb: 1.5 }}>
            Action Steps
          </Typography>

          <Stack gap={1.5}>
            {fields.map((stepField, index) => (
              <Controller
                key={stepField.id}
                name={`actionSteps.${index}.description`}
                control={control}
                render={({ field, fieldState }) => (
                  <Box>
                    <Stack direction="row" gap={1} alignItems="flex-start">
                      <Typography
                        variant="body2"
                        fontWeight={600}
                        color="text.secondary"
                        sx={{ pt: 1.25, minWidth: 28, flexShrink: 0 }}
                      >
                        {index + 1}.
                      </Typography>
                      <TextField
                        {...field}
                        fullWidth
                        size="small"
                        placeholder={`Describe action step ${index + 1}…`}
                        onChange={(e) => {
                          field.onChange(e);
                          if (e.target.value) clearErrors(`actionSteps.${index}.description` as FieldPath<AddRiskFormValues>);
                        }}
                        error={!!fieldState.error}
                        helperText={fieldState.error?.message}
                      />
                      <IconButton
                        onClick={() => remove(index)}
                        disabled={fields.length === 1}
                        size="small"
                        sx={{ mt: 0.5, flexShrink: 0, color: "error.main" }}
                        aria-label={`Remove step ${index + 1}`}
                      >
                        <Trash2 size={16} />
                      </IconButton>
                    </Stack>
                  </Box>
                )}
              />
            ))}
          </Stack>

          <Button
            variant="outlined"
            size="small"
            startIcon={<Plus size={15} />}
            onClick={() => append({ description: "" })}
            sx={{ mt: 2 }}
          >
            Add Step
          </Button>
        </Box>
      </Stack>

      {/* ── Treatment & Progress ────────────────────────────────────────────── */}
      <Stack gap={3}>
        <SectionHeader title="Treatment & Progress" />

        {/* Treatment Strategy */}
        <Controller
          name="treatmentStrategy"
          control={control}
          render={({ field, fieldState }) => (
            <Box>
              <FieldLabel>Treatment Strategy</FieldLabel>
              <ComplexSelect
                {...field}
                fullWidth
                error={!!fieldState.error}
                displayEmpty
                onChange={(e) => {
                  field.onChange(e);
                  if (e.target.value) clearErrors("treatmentStrategy");
                }}
              >
                <ComplexSelect.MenuItem value="" disabled sx={{ display: "none" }}>
                  Select a strategy
                </ComplexSelect.MenuItem>
                {TREATMENT_STRATEGIES.map((s) => (
                  <ComplexSelect.MenuItem key={s.value} value={s.value}>
                    {s.label}
                  </ComplexSelect.MenuItem>
                ))}
              </ComplexSelect>
              {fieldState.error && (
                <FormHelperText error>{fieldState.error.message}</FormHelperText>
              )}
            </Box>
          )}
        />

        {/* Progress */}
        <Controller
          name="progress"
          control={control}
          render={({ field, fieldState }) => (
            <TextField
              {...field}
              label="Progress"
              fullWidth
              multiline
              rows={3}
              placeholder="Describe the current state of progress…"
              error={!!fieldState.error}
              helperText={fieldState.error?.message ?? "Current remediation progress (Optional)"}
            />
          )}
        />
      </Stack>

      {/* ── References ──────────────────────────────────────────────────────── */}
      <Stack gap={3}>
        <SectionHeader title="References" />

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            gap: 2,
            alignItems: "flex-start",
          }}
        >
          {/* Git Issue URL */}
          <Controller
            name="gitIssueUrl"
            control={control}
            render={({ field, fieldState }) => (
              <TextField
                {...field}
                label="Git Issue URL"
                fullWidth
                placeholder="https://github.com/org/repo/issues/123"
                error={!!fieldState.error}
                helperText={fieldState.error?.message ?? "Link to the tracking issue (Optional)"}
              />
            )}
          />

          {/* Email Subject */}
          <Controller
            name="emailSubject"
            control={control}
            render={({ field, fieldState }) => (
              <TextField
                {...field}
                onChange={(e) => {
                  field.onChange(e);
                  if (e.target.value) clearErrors("emailSubject");
                }}
                label="Email Subject"
                fullWidth
                placeholder="RE: Risk remediation for…"
                error={!!fieldState.error}
                helperText={fieldState.error?.message ?? "Subject line of the related email thread."}
              />
            )}
          />
        </Box>

        {/* Remarks */}
        <Controller
          name="remarks"
          control={control}
          render={({ field, fieldState }) => (
            <TextField
              {...field}
              label="Remarks"
              fullWidth
              multiline
              rows={3}
              placeholder="Any additional notes or context…"
              error={!!fieldState.error}
              helperText={fieldState.error?.message ?? "Any additional observations or context (Optional)"}
            />
          )}
        />
      </Stack>

      {/* ── Evidence Attachments ─────────────────────────────────────────────── */}
      {/* TODO: on submit, POST attachments to /api/v1/risks/{id}/evidence (backend endpoint not yet implemented) */}
      <Stack gap={3}>
        <SectionHeader title="Evidence Attachments" />
        <EvidenceAttachments
          value={evidenceAttachments ?? []}
          onChange={(updated) => setValue("evidenceAttachments", updated, { shouldDirty: true })}
          accept="image/*,.pdf"
        />
      </Stack>
    </Stack>
  );
}
