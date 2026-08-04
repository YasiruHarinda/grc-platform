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
  AdapterDateFns,
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  DatePickers,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  FormHelperText,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { parseDateOnly, toDateOnlyString } from "@utils/dateTime";
import type { JSX } from "react";
import type * as React from "react";
import { resolveUserByEmail, searchEmployees } from "../../api/riskApi";
import type {
  ComplianceReference,
  EmployeeOption,
  RiskDetail,
  RiskScore,
  RiskTeam,
  UpdateRiskPayload,
  UserOption,
} from "../../api/riskApi";
import { TREATMENT_STRATEGIES } from "../add-risk/constants";
import { LEVEL_FALLBACK_COLORS } from "../dashboard/constants";
import { dialogPaperSx } from "../cardStyles";
import { useAuthApiClient } from "@hooks/useAuthApiClient";

// Minimum characters before searching — matches the backend's own floor.
const MIN_EMPLOYEE_SEARCH_LEN = 2;
const EMPLOYEE_SEARCH_DEBOUNCE_MS = 300;

const { DatePicker, LocalizationProvider } = DatePickers;

// ── Mode ──────────────────────────────────────────────────────────────────────
// "full"       — all fields editable (PENDING_RISK_OWNER_APPROVAL / PENDING_AMENDMENT)
// "restricted" — only implementation_date, action_steps, email_subject (post-approval)

export type EditMode = "full" | "restricted";

// ── Helpers ───────────────────────────────────────────────────────────────────

function SectionLabel({ children, restricted }: { children: React.ReactNode; restricted?: boolean }): JSX.Element {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
      <Typography variant="subtitle2" fontWeight={700}>
        {children}
      </Typography>
      {restricted && (
        <Chip label="Requires Re-approval" color="warning" size="small" />
      )}
    </Box>
  );
}

// ── Props ─────────────────────────────────────────────────────────────────────

interface EditRiskDialogProps {
  open: boolean;
  detail: RiskDetail;
  mode: EditMode;
  assignmentTeams: RiskTeam[];
  // Required for full mode only:
  users?: UserOption[];
  riskScores?: RiskScore[];
  complianceRefs?: ComplianceReference[];
  onClose: () => void;
  onSave: (payload: UpdateRiskPayload) => Promise<void>;
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function EditRiskDialog({
  open,
  detail,
  mode,
  assignmentTeams,
  users = [],
  riskScores = [],
  complianceRefs = [],
  onClose,
  onSave,
}: EditRiskDialogProps): JSX.Element {
  // ── All fields (full mode) ─────────────────────────────────────────────────
  const [riskTitle, setRiskTitle] = useState(detail.risk_title);
  const [riskDescription, setRiskDescription] = useState(detail.risk_description);
  const [impactDescription, setImpactDescription] = useState(detail.impact_description ?? "");
  const [riskIdentifiedDate, setRiskIdentifiedDate] = useState<Date | null>(
    parseDateOnly(detail.risk_identified_date),
  );
  const [identifiedByType, setIdentifiedByType] = useState(detail.identified_by_type ?? "");
  const [identifiedByName, setIdentifiedByName] = useState(detail.identified_by_name ?? "");
  // Not loaded from `detail` — RiskDetail never carries an email, only the
  // already-resolved name. Populated only when the user re-picks an employee
  // via the Autocomplete below; see the identifiedByChanged check in
  // handleSave for why an unset email here does not block saving.
  const [identifiedByEmail, setIdentifiedByEmail] = useState("");
  const [assignerId, setAssignerId] = useState<number | "">(detail.assigner_id || "");
  const [ownerId, setOwnerId] = useState<number | "">(detail.owner_id || "");
  const [selectedRefIds, setSelectedRefIds] = useState<number[]>(
    detail.compliance_references.map((r) => r.id),
  );
  const [grossScoreId, setGrossScoreId] = useState<number | null>(
    detail.gross_score?.id ?? null,
  );
  const [reassessmentDate, setReassessmentDate] = useState<Date | null>(
    parseDateOnly(detail.reassessment_date),
  );
  const [treatmentStrategy, setTreatmentStrategy] = useState(detail.treatment_strategy ?? "");
  const [assignmentTeamId, setAssignmentTeamId] = useState<number>(detail.assignment_team_id);
  const [actionOwnerId, setActionOwnerId] = useState<number | "">(
    detail.action_plan?.action_owner_id ?? "",
  );
  const [actionPlanDescription, setActionPlanDescription] = useState(
    detail.action_plan?.description ?? "",
  );
  const [progress, setProgress] = useState(detail.progress ?? "");
  const [gitIssueUrl, setGitIssueUrl] = useState(detail.git_issue_url ?? "");
  const [remarks, setRemarks] = useState(detail.remarks ?? "");

  // ── Fields available in both modes ────────────────────────────────────────
  const [implementationDate, setImplementationDate] = useState<Date | null>(
    parseDateOnly(detail.implementation_date),
  );
  // Steps keep their DB id so the backend can update in place and preserve
  // status/completed_date; newly added steps have no id.
  const [actionSteps, setActionSteps] = useState<{ id?: number; description: string }[]>(
    detail.action_plan?.steps.map((s) => ({ id: s.id, description: s.description })) ?? [],
  );
  const [emailSubject, setEmailSubject] = useState(detail.email_subject ?? "");

  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [apiError, setApiError] = useState("");

  // Employee search is live against the HR entity (never our own database).
  const authFetch = useAuthApiClient();
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

  // Action Owner can be any employee, not just an existing grc-platform user,
  // so — like Identified By (Employee) above — options are searched live
  // against the HR entity. Unlike that field, action_owner_id is a real FK,
  // so on selection we resolve the chosen employee to an internal user.id
  // (creating the user row on the fly if needed) via resolveUserByEmail.
  const [actionOwnerOptions, setActionOwnerOptions] = useState<EmployeeOption[]>([]);
  const [actionOwnerSelected, setActionOwnerSelected] = useState<EmployeeOption | null>(() => {
    const owner = users.find((u) => u.id === detail.action_plan?.action_owner_id);
    return owner ? { name: owner.display_name, email: owner.email } : null;
  });
  const [actionOwnerSearchLoading, setActionOwnerSearchLoading] = useState(false);
  const [actionOwnerResolving, setActionOwnerResolving] = useState(false);
  const [actionOwnerError, setActionOwnerError] = useState<string | null>(null);
  const actionOwnerDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Risk Owner is restricted to users already belonging (via risk_team_ids) to
  // this assignment team — source register isn't editable here (it's baked
  // into the immutable risk_code), so only assignmentTeamId is checked. The
  // risk's existing owner is always kept visible/selectable even if no
  // longer "eligible", so opening this dialog never silently invalidates
  // already-saved data — only an actual team change re-checks eligibility.
  const eligibleRiskOwners = users.filter((u) => u.risk_team_ids.includes(assignmentTeamId));
  const currentOwnerStillEligible = eligibleRiskOwners.some((u) => u.id === ownerId);
  const riskOwnerOptions = currentOwnerStillEligible
    ? eligibleRiskOwners
    : [...eligibleRiskOwners, ...users.filter((u) => u.id === ownerId)];
  const prevAssignmentTeamId = useRef(assignmentTeamId);
  useEffect(() => {
    if (prevAssignmentTeamId.current !== assignmentTeamId) {
      prevAssignmentTeamId.current = assignmentTeamId;
      setOwnerId((current) => {
        if (current === "") return current;
        const stillEligible = users.some((u) => u.id === current && u.risk_team_ids.includes(assignmentTeamId));
        return stillEligible ? current : "";
      });
    }
  }, [assignmentTeamId, users]);

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

  useEffect(() => {
    if (!open) return;
    setRiskTitle(detail.risk_title);
    setRiskDescription(detail.risk_description);
    setImpactDescription(detail.impact_description ?? "");
    setRiskIdentifiedDate(parseDateOnly(detail.risk_identified_date));
    setIdentifiedByType(detail.identified_by_type ?? "");
    setIdentifiedByName(detail.identified_by_name ?? "");
    setIdentifiedByEmail("");
    setAssignerId(detail.assigner_id || "");
    setOwnerId(detail.owner_id || "");
    setSelectedRefIds(detail.compliance_references.map((r) => r.id));
    setGrossScoreId(detail.gross_score?.id ?? null);
    setReassessmentDate(parseDateOnly(detail.reassessment_date));
    setTreatmentStrategy(detail.treatment_strategy ?? "");
    setAssignmentTeamId(detail.assignment_team_id);
    prevAssignmentTeamId.current = detail.assignment_team_id;
    setActionOwnerId(detail.action_plan?.action_owner_id ?? "");
    const currentActionOwner = users.find((u) => u.id === detail.action_plan?.action_owner_id);
    setActionOwnerSelected(currentActionOwner ? { name: currentActionOwner.display_name, email: currentActionOwner.email } : null);
    setActionPlanDescription(detail.action_plan?.description ?? "");
    setProgress(detail.progress ?? "");
    setGitIssueUrl(detail.git_issue_url ?? "");
    setRemarks(detail.remarks ?? "");
    setImplementationDate(parseDateOnly(detail.implementation_date));
    setActionSteps(detail.action_plan?.steps.map((s) => ({ id: s.id, description: s.description })) ?? []);
    setEmailSubject(detail.email_subject ?? "");
    setErrors({});
    setApiError("");
  }, [open, detail]);

  const validate = (): boolean => {
    const e: Record<string, string> = {};
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    if (mode === "full") {
      if (!riskTitle.trim()) e.riskTitle = "Risk title is required.";
      if (!riskDescription.trim()) e.riskDescription = "Risk description is required.";
      if (identifiedByType === "EMPLOYEE" && !identifiedByName.trim()) {
        e.identifiedByName = "Please select the employee who identified this risk.";
      }
      if ((identifiedByType === "EXTERNAL_PERSON" || identifiedByType === "TOOL") && !identifiedByName.trim()) {
        e.identifiedByName = "Please enter the name of who identified this risk.";
      }
      if (riskIdentifiedDate && riskIdentifiedDate > today) {
        e.riskIdentifiedDate = "Risk identified date cannot be in the future.";
      }
      if (reassessmentDate && reassessmentDate < today) {
        e.reassessmentDate = "Reassessment date cannot be in the past.";
      }
    }
    if (implementationDate && implementationDate < today) {
      e.implementationDate = "Implementation date cannot be in the past.";
    }
    if (!emailSubject.trim()) e.emailSubject = "Email subject is required.";
    actionSteps.forEach((s, i) => {
      if (!s.description.trim()) e[`step_${i}`] = "Step description cannot be empty.";
    });
    setErrors(e);
    return Object.keys(e).length === 0;
  };

  const handleSave = async () => {
    if (!validate()) return;
    setSubmitting(true);
    setApiError("");
    try {
      const payload: UpdateRiskPayload = {
        // Always sent (backend requires these)
        risk_title: mode === "full" ? riskTitle.trim() : detail.risk_title,
        risk_description: mode === "full" ? riskDescription.trim() : detail.risk_description,
        email_subject: emailSubject.trim(),
        implementation_date: toDateOnlyString(implementationDate),
        action_steps: actionSteps.length > 0
          ? actionSteps.map((s) => ({ id: s.id, description: s.description.trim() }))
          : undefined,
      };

      if (mode === "full") {
        payload.impact_description = impactDescription.trim() || undefined;
        payload.risk_identified_date = toDateOnlyString(riskIdentifiedDate);

        // Only sent when Identified By actually changed. The backend treats
        // an omitted identified_by_type as "leave it alone"; sending it
        // unconditionally would require identified_by_email on every save
        // (even ones that never touched this section), since that field is
        // never loaded from `detail` in the first place — see the state
        // declarations above.
        const identifiedByChanged =
          identifiedByType !== (detail.identified_by_type ?? "") ||
          identifiedByName.trim() !== (detail.identified_by_name ?? "");
        if (identifiedByChanged) {
          payload.identified_by_type = identifiedByType || undefined;
          payload.identified_by_name = identifiedByName.trim() || undefined;
          payload.identified_by_email =
            identifiedByType === "EMPLOYEE" ? identifiedByEmail || undefined : undefined;
        }

        payload.assigner_id = assignerId !== "" ? Number(assignerId) : undefined;
        payload.owner_id = ownerId !== "" ? Number(ownerId) : undefined;
        payload.compliance_reference_ids = selectedRefIds;
        payload.gross_score_id = grossScoreId ?? undefined;
        payload.reassessment_date = toDateOnlyString(reassessmentDate);
        payload.treatment_strategy = treatmentStrategy || undefined;
        payload.assignment_team_id = assignmentTeamId !== detail.assignment_team_id ? assignmentTeamId : undefined;
        payload.action_owner_id = actionOwnerId !== "" ? Number(actionOwnerId) : undefined;
        payload.action_plan_description = actionPlanDescription.trim() || undefined;
        payload.progress = progress.trim() || undefined;
        payload.git_issue_url = gitIssueUrl.trim() || undefined;
        payload.remarks = remarks.trim() || undefined;
      }

      await onSave(payload);
      onClose();
    } catch (e: unknown) {
      setApiError(e instanceof Error ? e.message : "Failed to save changes.");
    } finally {
      setSubmitting(false);
    }
  };

  const datepickerPaperSx = {
    backdropFilter: "none",
    backgroundColor: "#fff",
    "[data-color-scheme='dark'] &": { backgroundColor: "#1e1e1e" },
  };

  // ── Action steps (shared between both modes) ───────────────────────────────
  const actionStepsSection = (
    <Box>
      <SectionLabel>Action Steps</SectionLabel>
      <Stack gap={1.5}>
        {actionSteps.map((step, idx) => (
          <Box key={idx} sx={{ display: "flex", gap: 1, alignItems: "flex-start" }}>
            <Typography variant="body2" color="text.secondary" fontWeight={600} sx={{ mt: 1.5, minWidth: 24 }}>
              {idx + 1}.
            </Typography>
            <TextField
              size="small"
              fullWidth
              value={step.description}
              onChange={(e) => {
                const next = [...actionSteps];
                next[idx] = { ...next[idx], description: e.target.value };
                setActionSteps(next);
                if (errors[`step_${idx}`]) setErrors((p) => ({ ...p, [`step_${idx}`]: "" }));
              }}
              error={!!errors[`step_${idx}`]}
              helperText={errors[`step_${idx}`]}
              disabled={submitting}
            />
            <Tooltip title="Remove step">
              <IconButton
                size="small"
                onClick={() => setActionSteps(actionSteps.filter((_, i) => i !== idx))}
                disabled={submitting || actionSteps.length === 1}
                sx={{ mt: 0.25 }}
              >
                <Trash2 size={16} />
              </IconButton>
            </Tooltip>
          </Box>
        ))}
        <Button
          size="small"
          startIcon={<Plus size={14} />}
          onClick={() => setActionSteps([...actionSteps, { description: "" }])}
          disabled={submitting}
          sx={{ alignSelf: "flex-start" }}
        >
          Add Step
        </Button>
      </Stack>
    </Box>
  );

  return (
    <Dialog
      open={open}
      onClose={() => !submitting && onClose()}
      maxWidth="md"
      fullWidth
      PaperProps={{ sx: dialogPaperSx }}
    >
      {/* DialogTitle already renders an <h2>, so both children are spans —
          a nested <h6>/<p> inside a heading is invalid HTML and React warns
          about it. display:block keeps them on separate lines. */}
      <DialogTitle>
        <Typography component="span" variant="h6" fontWeight={700} display="block">
          Edit Risk
        </Typography>
        <Typography component="span" variant="caption" color="text.secondary" display="block">
          {detail.risk_code}
        </Typography>
      </DialogTitle>

      <DialogContent dividers>
        <Stack gap={3} sx={{ py: 1 }}>
          {apiError && <Alert severity="error">{apiError}</Alert>}

          {mode === "restricted" && (
            <Box>
              <Chip label="Requires Re-approval" color="warning" size="small" sx={{ mb: 0.5 }} />
              <Typography variant="caption" color="text.secondary" display="block">
                All three fields — implementation date, action steps, and email subject — require re-approval.
              </Typography>
            </Box>
          )}

          {/* ── RESTRICTED mode: only 3 fields ── */}
          {mode === "restricted" && (
            <>
              <Box>
                <SectionLabel>Implementation Date</SectionLabel>
                <LocalizationProvider dateAdapter={AdapterDateFns}>
                  <DatePicker
                    label="Implementation Date"
                    value={implementationDate}
                    onChange={(d) => { setImplementationDate(d); if (errors.implementationDate) setErrors((p) => ({ ...p, implementationDate: "" })); }}
                    disablePast
                    slotProps={{
                      desktopPaper: { sx: datepickerPaperSx },
                      textField: { fullWidth: true, disabled: submitting, error: !!errors.implementationDate, helperText: errors.implementationDate },
                    }}
                  />
                </LocalizationProvider>
              </Box>
              <Divider />
              {actionStepsSection}
              <Divider />
              <Box>
                <SectionLabel>Email Subject</SectionLabel>
                <TextField
                  label="Email Subject"
                  fullWidth
                  required
                  value={emailSubject}
                  onChange={(e) => { setEmailSubject(e.target.value); if (errors.emailSubject) setErrors((p) => ({ ...p, emailSubject: "" })); }}
                  error={!!errors.emailSubject}
                  helperText={errors.emailSubject}
                  disabled={submitting}
                />
              </Box>
            </>
          )}

          {/* ── FULL mode: all fields ── */}
          {mode === "full" && (
            <>
              {/* Basic Information */}
              <Box>
                <SectionLabel>Basic Information</SectionLabel>
                <Stack gap={2}>
                  <TextField
                    label="Risk Title"
                    fullWidth
                    required
                    value={riskTitle}
                    onChange={(e) => { setRiskTitle(e.target.value); if (errors.riskTitle) setErrors((p) => ({ ...p, riskTitle: "" })); }}
                    error={!!errors.riskTitle}
                    helperText={errors.riskTitle}
                    disabled={submitting}
                  />
                  <TextField
                    label="Risk Description"
                    fullWidth
                    required
                    multiline
                    rows={3}
                    value={riskDescription}
                    onChange={(e) => { setRiskDescription(e.target.value); if (errors.riskDescription) setErrors((p) => ({ ...p, riskDescription: "" })); }}
                    error={!!errors.riskDescription}
                    helperText={errors.riskDescription}
                    disabled={submitting}
                  />
                  <TextField
                    label="Impact Description"
                    fullWidth
                    multiline
                    rows={2}
                    value={impactDescription}
                    onChange={(e) => setImpactDescription(e.target.value)}
                    disabled={submitting}
                  />
                  <LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePicker
                      label="Risk Identified Date"
                      value={riskIdentifiedDate}
                      onChange={(d) => { setRiskIdentifiedDate(d); if (errors.riskIdentifiedDate) setErrors((p) => ({ ...p, riskIdentifiedDate: "" })); }}
                      disableFuture
                      slotProps={{ desktopPaper: { sx: datepickerPaperSx }, textField: { fullWidth: true, disabled: submitting, error: !!errors.riskIdentifiedDate, helperText: errors.riskIdentifiedDate } }}
                    />
                  </LocalizationProvider>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Identified By Type</InputLabel>
                    <Select
                      label="Identified By Type"
                      value={identifiedByType}
                      onChange={(e) => { setIdentifiedByType(e.target.value as string); setIdentifiedByName(""); }}
                    >
                      <MenuItem value="EMPLOYEE">Employee</MenuItem>
                      <MenuItem value="EXTERNAL_PERSON">External Person</MenuItem>
                      <MenuItem value="TOOL">Tool</MenuItem>
                    </Select>
                  </FormControl>
                  {identifiedByType === "EMPLOYEE" && (
                    <Autocomplete
                      options={employeeOptions}
                      loading={employeeSearchLoading}
                      filterOptions={(opts) => opts}
                      getOptionLabel={(option) => option.name}
                      isOptionEqualToValue={(option, value) => option.name === value.name}
                      value={identifiedByName ? { name: identifiedByName, email: "" } : null}
                      disabled={submitting}
                      onInputChange={(_, newInputValue, reason) => {
                        if (reason === "input") handleEmployeeInputChange(newInputValue);
                      }}
                      onChange={(_, newValue) => {
                        setIdentifiedByName(newValue?.name ?? "");
                        // The backend re-resolves identity from this email and
                        // ignores the name above on its own — see handleSave.
                        setIdentifiedByEmail(newValue?.email ?? "");
                        if (errors.identifiedByName) setErrors((p) => ({ ...p, identifiedByName: "" }));
                      }}
                      loadingText="Searching…"
                      noOptionsText={
                        employeeSearchError ??
                        "Type at least 2 characters of the employee's WSO2 email to search"
                      }
                      renderInput={(params) => (
                        <TextField
                          {...params}
                          label="Identified By (Employee)"
                          placeholder="Search by WSO2 email"
                          error={!!errors.identifiedByName || !!employeeSearchError}
                          helperText={errors.identifiedByName ?? employeeSearchError ?? undefined}
                        />
                      )}
                    />
                  )}
                  {(identifiedByType === "EXTERNAL_PERSON" || identifiedByType === "TOOL") && (
                    <TextField
                      label="Identified By (Name)"
                      fullWidth
                      value={identifiedByName}
                      onChange={(e) => setIdentifiedByName(e.target.value)}
                      disabled={submitting}
                      error={!!errors.identifiedByName}
                      helperText={errors.identifiedByName}
                    />
                  )}
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Risk Assigner</InputLabel>
                    <Select label="Risk Assigner" value={assignerId} onChange={(e) => setAssignerId(Number(e.target.value))}>
                      {users.map((u) => <MenuItem key={u.id} value={u.id}>{u.display_name}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth disabled={submitting} error={eligibleRiskOwners.length === 0}>
                    <InputLabel>Risk Owner</InputLabel>
                    <Select label="Risk Owner" value={ownerId} onChange={(e) => setOwnerId(Number(e.target.value))}>
                      {riskOwnerOptions.map((u) => <MenuItem key={u.id} value={u.id}>{u.display_name}</MenuItem>)}
                    </Select>
                    {eligibleRiskOwners.length === 0 && (
                      <FormHelperText error>
                        No users are assigned to this team yet. Contact an admin to assign team membership.
                      </FormHelperText>
                    )}
                  </FormControl>
                  {/* Compliance References */}
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Compliance References</InputLabel>
                    <Select
                      label="Compliance References"
                      multiple
                      value={selectedRefIds}
                      onChange={(e) => setSelectedRefIds(e.target.value as number[])}
                      renderValue={(selected) =>
                        (selected as number[])
                          .map((id) => complianceRefs.find((r) => r.id === id)?.name ?? id)
                          .join(", ")
                      }
                    >
                      {complianceRefs.map((ref) => (
                        <MenuItem key={ref.id} value={ref.id}>{ref.name}</MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Stack>
              </Box>

              <Divider />

              {/* Risk Assessment */}
              <Box>
                <SectionLabel>Risk Assessment</SectionLabel>
                <Stack gap={2}>
                  {/* Gross Score Matrix */}
                  <Box>
                    <Typography variant="caption" color="text.secondary" fontWeight={600} display="block" sx={{ mb: 1 }}>
                      Gross Risk Score
                    </Typography>
                    <Box sx={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 0.75, maxWidth: 300 }}>
                      {[3, 2, 1].map((likelihood) =>
                        [1, 2, 3].map((impact) => {
                          const score = riskScores.find((s) => s.likelihood === likelihood && s.impact === impact);
                          if (!score) return null;
                          const selected = grossScoreId === score.id;
                          return (
                            <Box
                              key={score.id}
                              onClick={() => !submitting && setGrossScoreId(score.id)}
                              sx={{
                                p: 1,
                                textAlign: "center",
                                borderRadius: 1,
                                cursor: submitting ? "default" : "pointer",
                                bgcolor: score.color_code,
                                opacity: selected ? 1 : 0.35,
                                border: selected ? "2px solid #000" : "2px solid transparent",
                                transform: selected ? "scale(1.06)" : "scale(1)",
                                transition: "all 0.1s",
                              }}
                            >
                              <Typography variant="caption" fontWeight={700} color="#fff">
                                {score.risk_rating}
                              </Typography>
                            </Box>
                          );
                        })
                      )}
                    </Box>
                    {grossScoreId && (() => {
                      const s = riskScores.find((sc) => sc.id === grossScoreId);
                      return s ? (
                        <Typography variant="caption" sx={{ mt: 0.5, display: "block", color: LEVEL_FALLBACK_COLORS[s.risk_level] ?? "text.secondary" }}>
                          L{s.likelihood} × I{s.impact} = {s.risk_rating} ({s.risk_level})
                        </Typography>
                      ) : null;
                    })()}
                  </Box>
                  <LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePicker
                      label="Implementation Date"
                      value={implementationDate}
                      onChange={(d) => { setImplementationDate(d); if (errors.implementationDate) setErrors((p) => ({ ...p, implementationDate: "" })); }}
                      disablePast
                      slotProps={{ desktopPaper: { sx: datepickerPaperSx }, textField: { fullWidth: true, disabled: submitting, error: !!errors.implementationDate, helperText: errors.implementationDate } }}
                    />
                  </LocalizationProvider>
                  <LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePicker
                      label="Reassessment Date"
                      value={reassessmentDate}
                      onChange={(d) => { setReassessmentDate(d); if (errors.reassessmentDate) setErrors((p) => ({ ...p, reassessmentDate: "" })); }}
                      disablePast
                      slotProps={{ desktopPaper: { sx: datepickerPaperSx }, textField: { fullWidth: true, disabled: submitting, error: !!errors.reassessmentDate, helperText: errors.reassessmentDate } }}
                    />
                  </LocalizationProvider>
                </Stack>
              </Box>

              <Divider />

              {/* Action Plan */}
              <Box>
                <SectionLabel>Action Plan</SectionLabel>
                <Stack gap={2}>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Treatment Strategy</InputLabel>
                    <Select label="Treatment Strategy" value={treatmentStrategy} onChange={(e) => setTreatmentStrategy(e.target.value as string)}>
                      {TREATMENT_STRATEGIES.map((s) => <MenuItem key={s.value} value={s.value}>{s.label}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Assignment Team</InputLabel>
                    <Select label="Assignment Team" value={assignmentTeamId} onChange={(e) => setAssignmentTeamId(Number(e.target.value))}>
                      {assignmentTeams.map((t) => <MenuItem key={t.id} value={t.id}>{t.name}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <Autocomplete
                    options={actionOwnerOptions}
                    loading={actionOwnerSearchLoading || actionOwnerResolving}
                    filterOptions={(opts) => opts}
                    getOptionLabel={(option) => option.name}
                    isOptionEqualToValue={(option, value) => option.email === value.email}
                    value={actionOwnerSelected}
                    disabled={submitting}
                    onInputChange={(_, newInputValue, reason) => {
                      if (reason === "input") handleActionOwnerInputChange(newInputValue);
                    }}
                    onChange={(_, newValue) => {
                      if (!newValue) {
                        setActionOwnerSelected(null);
                        setActionOwnerId("");
                        return;
                      }
                      setActionOwnerResolving(true);
                      resolveUserByEmail(authFetch, newValue)
                        .then((resolved) => {
                          setActionOwnerSelected(newValue);
                          setActionOwnerId(resolved.id);
                          if (errors.actionOwnerId) setErrors((p) => ({ ...p, actionOwnerId: "" }));
                        })
                        .catch(() => {
                          setActionOwnerSelected(null);
                          setActionOwnerId("");
                          setActionOwnerError("Unable to link this employee to a user account. Please try again.");
                        })
                        .finally(() => setActionOwnerResolving(false));
                    }}
                    loadingText="Searching…"
                    noOptionsText={
                      actionOwnerError ??
                      "Type at least 2 characters of the employee's email to search"
                    }
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        label="Action Owner"
                        placeholder="Search by email"
                        error={!!errors.actionOwnerId || !!actionOwnerError}
                        helperText={errors.actionOwnerId ?? actionOwnerError ?? undefined}
                      />
                    )}
                  />
                  <TextField
                    label="Action Plan Description"
                    fullWidth
                    multiline
                    rows={2}
                    value={actionPlanDescription}
                    onChange={(e) => setActionPlanDescription(e.target.value)}
                    disabled={submitting}
                  />
                </Stack>
              </Box>

              <Divider />

              {/* Action Steps */}
              {actionStepsSection}

              <Divider />

              {/* Other */}
              <Box>
                <SectionLabel>Other</SectionLabel>
                <Stack gap={2}>
                  <TextField
                    label="Email Subject"
                    fullWidth
                    required
                    value={emailSubject}
                    onChange={(e) => { setEmailSubject(e.target.value); if (errors.emailSubject) setErrors((p) => ({ ...p, emailSubject: "" })); }}
                    error={!!errors.emailSubject}
                    helperText={errors.emailSubject}
                    disabled={submitting}
                  />
                  <TextField
                    label="Progress"
                    fullWidth
                    multiline
                    rows={2}
                    value={progress}
                    onChange={(e) => setProgress(e.target.value)}
                    disabled={submitting}
                  />
                  <TextField
                    label="Git Issue URL"
                    fullWidth
                    value={gitIssueUrl}
                    onChange={(e) => setGitIssueUrl(e.target.value)}
                    disabled={submitting}
                  />
                  <TextField
                    label="Remarks"
                    fullWidth
                    multiline
                    rows={2}
                    value={remarks}
                    onChange={(e) => setRemarks(e.target.value)}
                    disabled={submitting}
                  />
                </Stack>
              </Box>
            </>
          )}
        </Stack>
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={() => !submitting && onClose()} disabled={submitting} color="inherit">
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={submitting} variant="contained">
          {submitting ? "Saving..." : "Save Changes"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
