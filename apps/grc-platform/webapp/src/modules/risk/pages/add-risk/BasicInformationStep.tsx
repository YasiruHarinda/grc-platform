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
import { fetchRiskAssignerCandidates, searchEmployees } from "../../api/riskApi";
import type { ComplianceReference, EmployeeOption, RiskCategory, RiskTeam, UserOption } from "../../api/riskApi";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";

// Minimum characters before searching — matches the backend's own floor
// (GET /api/v1/risks/employees/search ignores shorter queries) so we don't fire
// requests that would just come back empty.
const MIN_EMPLOYEE_SEARCH_LEN = 2;
const EMPLOYEE_SEARCH_DEBOUNCE_MS = 300;

const { DatePicker, LocalizationProvider } = DatePickers;

// `required` renders the asterisk convention users expect on a form: the field
// must be filled before the step will submit. It mirrors the `rules.required`
// on the same Controller — keep the two in step, or the form will either
// promise something it doesn't enforce or enforce something it didn't warn about.
function FieldLabel({ children, required }: { children: ReactNode; required?: boolean }): JSX.Element {
  return (
    <Typography
      variant="body2"
      fontWeight={500}
      color="text.primary"
      sx={{ display: "block", mb: 1 }}
    >
      {children}
      {required && (
        // Inherits the label's colour rather than fixing one: the form sits on a
        // dark card in dark mode and a light one otherwise, so a hard-coded
        // colour would be invisible in one of them.
        <Box component="span" aria-hidden="true" sx={{ color: "inherit", ml: 0.4 }}>
          *
        </Box>
      )}
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
  riskCategories: RiskCategory[];
}

export default function BasicInformationStep({
  riskSequenceId,
  sourceRegisterTeams,
  complianceRefs,
  riskCategories,
}: BasicInformationStepProps): JSX.Element {
  const { control, clearErrors, setValue } = useFormContext<AddRiskFormValues>();
  const authFetch = useAuthApiClient();

  const year             = useWatch({ control, name: "year" });
  const quarter          = useWatch({ control, name: "quarter" });
  const sourceRegister   = useWatch({ control, name: "sourceRegister" });
  const identifiedByType = useWatch({ control, name: "identifiedByType" });

  // Risk Assigned To is restricted to users who already hold RISK_CREATE in
  // the chosen source register — the same grant handleCreateRisk itself
  // checks, so a candidate offered here can never 403 on submit. Was
  // previously every platform user, unfiltered by role or register — see
  // ActionPlanStep's identical pattern for Risk Owner / Management Approver,
  // which this mirrors. Fetched fresh whenever the source register changes,
  // rather than filtered from a static list, since eligibility is
  // scope-dependent.
  const assignerTeamIds = typeof sourceRegister === "number" ? [sourceRegister] : [];
  const [riskAssignerCandidates, setRiskAssignerCandidates] = useState<UserOption[]>([]);

  // The signed-in caller's own identity, from GET /me/profile — not decoded
  // from an ID token client-side, since mock-auth mode has no real Asgardeo
  // session to decode one from. Fetched once, independent of source register:
  // the register dropdown itself already only offers registers the caller
  // holds RISK_CREATE in (see AddRisk.tsx's fetchSourceRegisterTeams call), so
  // whichever one they pick, they are necessarily an eligible assigner there —
  // there is nothing to wait for.
  const [me, setMe] = useState<{ id: number; name: string } | null>(null);
  useEffect(() => {
    authFetch(`${BACKEND_BASE_URL}/api/v1/me/profile`)
      .then((res) =>
        res.ok
          ? (res.json() as Promise<{ user_id?: number; first_name?: string; last_name?: string }>)
          : null,
      )
      .then((profile) => {
        if (profile?.user_id == null) return setMe(null);
        const name = `${profile.first_name ?? ""} ${profile.last_name ?? ""}`.trim();
        setMe({ id: profile.user_id, name: name || "Me" });
      })
      .catch(() => setMe(null));
  }, [authFetch]);

  // Seeds the picker with just the caller, so it has a name to show and a
  // valid default the instant the page loads — before any register is picked
  // and before the real, possibly-larger candidate list has loaded. Guarded
  // on assignerTeamIds being empty so this can never fire after a register is
  // already chosen: the /me/profile fetch and the register-scoped fetch below
  // are independent requests with no ordering guarantee, and without this
  // guard a slow /me/profile response landing after the real list had already
  // loaded would silently clobber it back down to just the caller.
  useEffect(() => {
    // email/risk_team_ids are unused by this picker (only display_name
    // renders) — empty placeholders, since this entry is only ever transient,
    // replaced the moment the real, register-scoped fetch below resolves.
    if (me && assignerTeamIds.length === 0) {
      setRiskAssignerCandidates([{ id: me.id, display_name: me.name, email: "", risk_team_ids: [] }]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only re-seed when "me" resolves; assignerTeamIds is read for its value at that render, not a re-fire trigger — the fetch effect below owns re-fetching on register change
  }, [me]);

  useEffect(() => {
    if (assignerTeamIds.length === 0) return; // nothing chosen yet — keep the seeded self-entry
    let cancelled = false;
    fetchRiskAssignerCandidates(authFetch, assignerTeamIds)
      .then((list) => { if (!cancelled) setRiskAssignerCandidates(list); })
      .catch(() => { if (!cancelled) setRiskAssignerCandidates([]); });
    // Cancelled on the next register change (or unmount), so an earlier,
    // slower request can never land after a later one and overwrite its
    // correct, more current candidate list with a stale one.
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only re-fetch when the source register changes
  }, [assignerTeamIds.join(","), authFetch]);

  // Clear a previously-selected assignee if changing the source register
  // makes them no longer eligible — avoids submitting a value that's
  // silently stale relative to the visible options. Defaults to the caller
  // themselves as soon as their own id is known, independent of the
  // candidate list — see the fetch above for why that's always safe here.
  const assignedBy = useWatch({ control, name: "assignedBy" });
  useEffect(() => {
    if (assignedBy !== "" && !riskAssignerCandidates.some((u) => u.id === assignedBy)) {
      setValue("assignedBy", "", { shouldDirty: false });
      return;
    }
    if (assignedBy === "" && me !== null) {
      setValue("assignedBy", me.id, { shouldDirty: false });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only re-check when the candidate list or "me" changes
  }, [riskAssignerCandidates, me]);

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
                  <FieldLabel required>Year</FieldLabel>
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
                  <FieldLabel required>Quarter</FieldLabel>
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
                  <FieldLabel required>Source Register</FieldLabel>
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
                required
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
                required
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

          {/* Risk Category (single-select) */}
          <Controller
            name="riskCategory"
            control={control}
            rules={{ required: "Risk category is required" }}
            render={({ field, fieldState }) => (
              <Box>
                <FieldLabel required>Risk Category</FieldLabel>
                <ComplexSelect
                  {...field}
                  fullWidth
                  error={!!fieldState.error}
                  displayEmpty
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("riskCategory");
                  }}
                >
                  <ComplexSelect.MenuItem value="" disabled sx={{ display: "none" }}>
                    Select a risk category
                  </ComplexSelect.MenuItem>
                  {riskCategories.map((cat) => (
                    <ComplexSelect.MenuItem key={cat.id} value={cat.id}>
                      {cat.name}
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                {fieldState.error && (
                  <FormHelperText error>{fieldState.error.message}</FormHelperText>
                )}
              </Box>
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
                <FormLabel sx={{ fontWeight: 500 }}>
                  Risk Identified By
                  <Box component="span" aria-hidden="true" sx={{ color: "inherit", ml: 0.4 }}>
                    *
                  </Box>
                </FormLabel>
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
                  <FieldLabel required>Select Employee</FieldLabel>
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
                  required
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
                  required
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
                <FieldLabel required>Risk Identified Date</FieldLabel>
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
                <FieldLabel required>Risk Assigned To</FieldLabel>
                <ComplexSelect
                  {...field}
                  fullWidth
                  error={!!fieldState.error}
                  onChange={(e) => {
                    field.onChange(e);
                    if (e.target.value) clearErrors("assignedBy");
                  }}
                >
                  {riskAssignerCandidates.map((u) => (
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
