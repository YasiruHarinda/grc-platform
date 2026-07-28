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

import { useCallback, useRef, useState } from "react";
import { Controller, useFormContext, useWatch } from "react-hook-form";
import {
  AdapterDateFns,
  Autocomplete,
  Box,
  ComplexSelect,
  DatePickers,
  Divider,
  FormControl,
  FormControlLabel,
  FormHelperText,
  FormLabel,
  Radio,
  RadioGroup,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX, ReactNode } from "react";
import type { AddRiskFormValues } from "./types";
import { QUARTERS, YEAR_OPTIONS } from "./constants";
import { searchEmployees } from "../../api/riskApi";
import type { ComplianceReference, EmployeeOption, RiskTeam, UserOption } from "../../api/riskApi";
import { useAuthApiClient } from "@hooks/useAuthApiClient";

// Minimum characters before searching — matches the backend's own floor
// (GET /api/v1/employees/search ignores shorter queries) so we don't fire
// requests that would just come back empty.
const MIN_EMPLOYEE_SEARCH_LEN = 2;
const EMPLOYEE_SEARCH_DEBOUNCE_MS = 300;

const { DatePicker, LocalizationProvider } = DatePickers;

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

interface BasicInformationStepProps {
  riskSequenceId: number | null;
  sourceRegisterTeams: RiskTeam[];
  complianceRefs: ComplianceReference[];
  users: UserOption[];
}

export default function BasicInformationStep({
  riskSequenceId,
  sourceRegisterTeams,
  complianceRefs,
  users,
}: BasicInformationStepProps): JSX.Element {
  const { control, clearErrors, setValue } = useFormContext<AddRiskFormValues>();
  const authFetch = useAuthApiClient();

  const year             = useWatch({ control, name: "year" });
  const quarter          = useWatch({ control, name: "quarter" });
  const sourceRegister   = useWatch({ control, name: "sourceRegister" });
  const identifiedByType = useWatch({ control, name: "identifiedByType" });

  // Employee search is live against the HR entity (never our own database),
  // so — unlike the other dropdowns — options aren't fetched once up front.
  const [employeeOptions, setEmployeeOptions] = useState<EmployeeOption[]>([]);
  const [employeeSearchLoading, setEmployeeSearchLoading] = useState(false);
  const [employeeSearchError, setEmployeeSearchError] = useState<string | null>(null);
  const employeeSearchDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  const runEmployeeSearch = useCallback((query: string) => {
    if (query.trim().length < MIN_EMPLOYEE_SEARCH_LEN) {
      setEmployeeOptions([]);
      setEmployeeSearchError(null);
      return;
    }
    setEmployeeSearchLoading(true);
    setEmployeeSearchError(null);
    searchEmployees(authFetch, query)
      .then(setEmployeeOptions)
      .catch(() => {
        setEmployeeOptions([]);
        setEmployeeSearchError("Unable to reach the employee directory. Please try again.");
      })
      .finally(() => setEmployeeSearchLoading(false));
  }, [authFetch]);

  const handleEmployeeInputChange = (value: string): void => {
    if (employeeSearchDebounce.current) clearTimeout(employeeSearchDebounce.current);
    employeeSearchDebounce.current = setTimeout(() => runEmployeeSearch(value), EMPLOYEE_SEARCH_DEBOUNCE_MS);
  };

  const selectedTeam = typeof sourceRegister === "number"
    ? sourceRegisterTeams.find(t => t.id === sourceRegister) ?? null
    : null;
  const teamCode = selectedTeam?.code ?? null;

  const seqSuffix = riskSequenceId !== null
    ? String(riskSequenceId).padStart(4, "0")
    : "####";

  const riskCodePreview =
    year && quarter && teamCode
      ? `${year}-${teamCode}-${quarter}-${seqSuffix}`
      : "YEAR-REGISTER-QUARTER-####";

  return (
    <LocalizationProvider dateAdapter={AdapterDateFns}>
      <Stack gap={4}>

        {/* ── Risk Identification ────────────────────────────── */}
        <Stack gap={3}>
          <SectionHeader title="Risk Identification" />

          {/* Year | Quarter | Source Register */}
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 2fr" },
              gap: 2,
              alignItems: "flex-start",
            }}
          >
            {/* Year */}
            <Controller
              name="year"
              control={control}
              rules={{ required: "Year is required" }}
              render={({ field, fieldState }) => (
                <Box>
                  <FieldLabel>Year</FieldLabel>
                  <ComplexSelect
                    {...field}
                    fullWidth
                    error={!!fieldState.error}
                    onChange={(e) => {
                      field.onChange(e);
                      if (e.target.value) clearErrors("year");
                    }}
                  >
                    {YEAR_OPTIONS.map((y) => (
                      <ComplexSelect.MenuItem key={y} value={y}>
                        {y}
                      </ComplexSelect.MenuItem>
                    ))}
                  </ComplexSelect>
                  {fieldState.error && (
                    <FormHelperText error>{fieldState.error.message}</FormHelperText>
                  )}
                </Box>
              )}
            />

            {/* Quarter */}
            <Controller
              name="quarter"
              control={control}
              rules={{ required: "Quarter is required" }}
              render={({ field, fieldState }) => (
                <Box>
                  <FieldLabel>Quarter</FieldLabel>
                  <ComplexSelect
                    {...field}
                    fullWidth
                    error={!!fieldState.error}
                    onChange={(e) => {
                      field.onChange(e);
                      if (e.target.value) clearErrors("quarter");
                    }}
                  >
                    {QUARTERS.map((q) => (
                      <ComplexSelect.MenuItem key={q.value} value={q.value}>
                        {q.label}
                      </ComplexSelect.MenuItem>
                    ))}
                  </ComplexSelect>
                  {fieldState.error && (
                    <FormHelperText error>{fieldState.error.message}</FormHelperText>
                  )}
                </Box>
              )}
            />

            {/* Source Register */}
            <Controller
              name="sourceRegister"
              control={control}
              rules={{ required: "Please select a source register" }}
              render={({ field, fieldState }) => (
                <Box>
                  <FieldLabel>Source Register</FieldLabel>
                  <ComplexSelect
                    {...field}
                    fullWidth
                    error={!!fieldState.error}
                    displayEmpty
                    onChange={(e) => {
                      field.onChange(e);
                      if (e.target.value) clearErrors("sourceRegister");
                    }}
                  >
                    <ComplexSelect.MenuItem value="" disabled sx={{ display: "none" }}>
                      Select a register
                    </ComplexSelect.MenuItem>
                    {sourceRegisterTeams.map((team) => (
                      <ComplexSelect.MenuItem key={team.id} value={team.id}>
                        {team.name}{team.code ? ` (${team.code})` : ""}
                      </ComplexSelect.MenuItem>
                    ))}
                  </ComplexSelect>
                  {fieldState.error && (
                    <FormHelperText error>{fieldState.error.message}</FormHelperText>
                  )}
                </Box>
              )}
            />
          </Box>

          {/* Risk Code (auto-generated preview — not a user-editable field) */}
          <Box
            sx={{
              px: 2,
              py: 1.5,
              borderRadius: 1,
              bgcolor: "action.hover",
              border: "1px solid",
              borderColor: "divider",
            }}
          >
            <Typography variant="caption" color="text.secondary" display="block" gutterBottom>
              Risk Code
            </Typography>
            <Typography
              variant="body1"
              fontFamily="monospace"
              fontWeight={600}
              color={year && quarter && teamCode ? "text.primary" : "text.disabled"}
            >
              {riskCodePreview}
            </Typography>
          </Box>
        </Stack>

        {/* ── Risk Details ───────────────────────────────────── */}
        <Stack gap={3}>
          <SectionHeader title="Risk Details" />

          {/* Risk Title */}
          <Controller
            name="riskTitle"
            control={control}
            rules={{
              required: "Risk title is required",
              maxLength: { value: 500, message: "Title must be 500 characters or fewer" },
            }}
            render={({ field, fieldState }) => (
              <TextField
                {...field}
                onChange={(e) => {
                  field.onChange(e);
                  if (e.target.value) clearErrors("riskTitle");
                }}
                label="Risk Title"
                fullWidth
                error={!!fieldState.error}
                helperText={fieldState.error?.message}
                slotProps={{ htmlInput: { maxLength: 500 } }}
              />
            )}
          />

          {/* Risk Description */}
          <Controller
            name="riskDescription"
            control={control}
            rules={{ required: "Risk description is required" }}
            render={({ field, fieldState }) => (
              <TextField
                {...field}
                onChange={(e) => {
                  field.onChange(e);
                  if (e.target.value) clearErrors("riskDescription");
                }}
                label="Risk Description"
                fullWidth
                multiline
                rows={4}
                error={!!fieldState.error}
                helperText={fieldState.error?.message}
              />
            )}
          />

          {/* Security Compliance Reference (multi-select toggle buttons) */}
          <Controller
            name="complianceReferences"
            control={control}
            render={({ field }) => (
              <FormControl>
                <FormLabel sx={{ mb: 1.5, fontWeight: 500 }}>
                  Security Compliance Reference
                  <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                    (select all that apply)
                  </Typography>
                </FormLabel>
                <ToggleButtonGroup
                  value={field.value}
                  onChange={(_, newValues: number[]) =>
                    field.onChange(newValues ?? [])
                  }
                  aria-label="Security compliance references"
                  sx={{ flexWrap: "wrap", gap: 1 }}
                >
                  {complianceRefs.map((ref) => (
                    <ToggleButton
                      key={ref.id}
                      value={ref.id}
                      size="small"
                      sx={{
                        borderRadius: "20px !important",
                        px: 2,
                        border: "1px solid !important",
                        "&.Mui-selected": {
                          backgroundColor: "primary.main",
                          color: "#fff",
                          borderColor: "primary.main !important",
                        },
                        "&.Mui-selected:hover": {
                          backgroundColor: "primary.dark",
                        },
                      }}
                    >
                      {ref.name}
                    </ToggleButton>
                  ))}
                </ToggleButtonGroup>
              </FormControl>
            )}
          />
        </Stack>

        {/* ── Identification & Assignment ────────────────────── */}
        <Stack gap={3}>
          <SectionHeader title="Identification & Assignment" />

          {/* Risk Identified By */}
          <Controller
            name="identifiedByType"
            control={control}
            rules={{ required: "Please select who identified this risk" }}
            render={({ field, fieldState }) => (
              <FormControl error={!!fieldState.error}>
                <FormLabel sx={{ fontWeight: 500 }}>Risk Identified By</FormLabel>
                <RadioGroup
                  name={field.name}
                  value={field.value}
                  onChange={(e) => field.onChange(e.target.value)}
                  onBlur={field.onBlur}
                  row
                  sx={{ mt: 1, gap: 1 }}
                >
                  <FormControlLabel value="EMPLOYEE"        control={<Radio />} label="Employee" />
                  <FormControlLabel value="EXTERNAL_PERSON" control={<Radio />} label="External Person" />
                  <FormControlLabel value="TOOL"            control={<Radio />} label="Tool" />
                </RadioGroup>
                {fieldState.error && (
                  <FormHelperText>{fieldState.error.message}</FormHelperText>
                )}
              </FormControl>
            )}
          />

          {/* Conditional: Employee search (identified_by_name VARCHAR) — options are
               fetched live from the HR entity service by email substring, never from
               our own database. On lookup failure the field stays required/blocked
               rather than falling back to free text. */}
          {identifiedByType === "EMPLOYEE" && (
            <Controller
              name="identifiedByName"
              control={control}
              rules={{ required: "Please select the employee who identified this risk" }}
              render={({ field, fieldState }) => (
                <Box>
                  <FieldLabel>Select Employee</FieldLabel>
                  <Autocomplete
                    options={employeeOptions}
                    loading={employeeSearchLoading}
                    filterOptions={(opts) => opts}
                    getOptionLabel={(option) => option.name}
                    isOptionEqualToValue={(option, value) => option.name === value.name}
                    value={field.value ? { name: field.value, email: "" } : null}
                    onInputChange={(_, newInputValue, reason) => {
                      if (reason === "input") handleEmployeeInputChange(newInputValue);
                    }}
                    onChange={(_, newValue) => {
                      field.onChange(newValue?.name ?? "");
                      // The backend re-resolves identity from this email and
                      // ignores the name above on its own — see types.ts.
                      setValue("identifiedByEmail", newValue?.email ?? "");
                      if (newValue) clearErrors("identifiedByName");
                    }}
                    loadingText="Searching…"
                    noOptionsText={
                      employeeSearchError ??
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
                    }}
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        fullWidth
                        placeholder="Search by email"
                        error={!!fieldState.error || !!employeeSearchError}
                        helperText={
                          fieldState.error?.message ?? employeeSearchError ?? undefined
                        }
                        onBlur={field.onBlur}
                      />
                    )}
                  />
                </Box>
              )}
            />
          )}

          {/* Conditional: External person name (identified_by_name VARCHAR) */}
          {identifiedByType === "EXTERNAL_PERSON" && (
            <Controller
              name="identifiedByName"
              control={control}
              rules={{ required: "Please enter the name of the person who identified this risk" }}
              render={({ field, fieldState }) => (
                <TextField
                  {...field}
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("identifiedByName");
                  }}
                  label="Name of the person who identified"
                  fullWidth
                  error={!!fieldState.error}
                  helperText={fieldState.error?.message}
                />
              )}
            />
          )}

          {/* Conditional: Tool name (identified_by_name VARCHAR) */}
          {identifiedByType === "TOOL" && (
            <Controller
              name="identifiedByName"
              control={control}
              rules={{ required: "Please enter the name of the tool" }}
              render={({ field, fieldState }) => (
                <TextField
                  {...field}
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("identifiedByName");
                  }}
                  label="Name of the tool"
                  fullWidth
                  error={!!fieldState.error}
                  helperText={fieldState.error?.message}
                />
              )}
            />
          )}

          {/* Risk Identified Date */}
          <Controller
            name="riskIdentifiedDate"
            control={control}
            rules={{ required: "Risk identified date is required" }}
            render={({ field, fieldState }) => (
              <Box>
                <FieldLabel>Risk Identified Date</FieldLabel>
                <DatePicker
                  value={field.value}
                  onChange={(newValue) => {
                    field.onChange(newValue);
                    if (newValue) clearErrors("riskIdentifiedDate");
                  }}
                  disableFuture
                  sx={{ width: "100%" }}
                  slotProps={{
                    desktopPaper: {
                      sx: {
                        backdropFilter: "none",
                        backgroundColor: "#fff",
                        "[data-color-scheme='dark'] &": {
                          backgroundColor: "#1e1e1e",
                        },
                      },
                    },
                    textField: {
                      fullWidth: true,
                      error: !!fieldState.error,
                      helperText: fieldState.error?.message,
                      onBlur: field.onBlur,
                    },
                  }}
                />
              </Box>
            )}
          />

          {/* Risk Assigned To — intentionally labelled "Assigned To" in the UI for user clarity,
               even though the form field is `assignedBy` and the backend column is `assigner_id`.
               The schema name was kept as designed; only the display label differs. */}
          <Controller
            name="assignedBy"
            control={control}
            rules={{ required: "Please select an assignee" }}
            render={({ field, fieldState }) => (
              <Box>
                <FieldLabel>Risk Assigned To</FieldLabel>
                <ComplexSelect
                  {...field}
                  fullWidth
                  error={!!fieldState.error}
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("assignedBy");
                  }}
                >
                  {users.map((u) => (
                    <ComplexSelect.MenuItem key={u.id} value={u.id}>
                      {u.display_name}
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                <FormHelperText error={!!fieldState.error}>
                  {fieldState.error?.message ?? "Change if submitting on behalf of someone else."}
                </FormHelperText>
              </Box>
            )}
          />
        </Stack>

      </Stack>
    </LocalizationProvider>
  );
}
