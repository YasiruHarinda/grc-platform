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
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Drawer,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil, Plus, Trash2, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useGetControls } from "@modules/audit/api/useGetControls";
import { useGetUsers } from "@modules/audit/api/useGetUsers";
import { useGetTeams } from "@modules/audit/api/useGetTeams";
import { useAddControl } from "@modules/audit/api/useAddControl";
import { useUpdateControl } from "@modules/audit/api/useUpdateControl";
import { useDeleteControl } from "@modules/audit/api/useDeleteControl";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import type {
  AddControlRequest,
  AuditControl,
  AuditTeam,
  ControlScope,
  ControlType,
  PopulationDetails,
  RequirementType,
  UpdateControlRequest,
} from "@modules/audit/types/audit";
import type { AuditUser } from "@modules/audit/types/user";

// Local YYYY-MM-DD for today, so a due date can't be set in the past.
function todayISO(): string {
  const d = new Date();
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  return `${yyyy}-${mm}-${dd}`;
}

// ── Control form state (used for both Add and Edit dialogs) ──────────────────

interface ControlFormState {
  controlNumber: string;
  description: string;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
  evidenceRequirement: string;
  dueDate: string;
  owner: AuditUser | null;
  team: AuditTeam | null;
  auditor: AuditUser | null;
  populationDescription: string;
  populationDueDate: string;
  populationComments: string;
  populationOwner: AuditUser | null;
  populationTeam: AuditTeam | null;
}

const EMPTY_FORM: ControlFormState = {
  controlNumber: "",
  description: "",
  requirementType: "DESIGN",
  controlType: "NON_CONFIG",
  scope: "COMMON",
  evidenceRequirement: "",
  dueDate: "",
  owner: null,
  team: null,
  auditor: null,
  populationDescription: "",
  populationDueDate: "",
  populationComments: "",
  populationOwner: null,
  populationTeam: null,
};

function controlToForm(c: AuditControl, users: AuditUser[], teams: AuditTeam[]): ControlFormState {
  return {
    controlNumber: c.controlNumber,
    description: c.description,
    requirementType: c.requirementType,
    controlType: c.controlType,
    scope: c.scope,
    evidenceRequirement: c.evidenceRequirement ?? "",
    dueDate: c.dueDate ?? "",
    owner: users.find((u) => u.id === c.ownerId) ?? null,
    team: teams.find((t) => t.id === c.teamId) ?? null,
    auditor: users.find((u) => u.id === c.auditorId) ?? null,
    populationDescription: c.populationDescription ?? "",
    populationDueDate: c.populationDueDate ?? "",
    populationComments: c.populationComments ?? "",
    populationOwner: users.find((u) => u.displayName === c.populationOwnerName) ?? null,
    populationTeam: teams.find((t) => t.name === c.populationTeamName) ?? null,
  };
}

// ── ControlFormDialog ────────────────────────────────────────────────────────

interface ControlFormDialogProps {
  open: boolean;
  title: string;
  initialValues: ControlFormState;
  users: AuditUser[];
  teams: AuditTeam[];
  isSaving: boolean;
  error: string | null;
  editMode?: boolean;
  onSave: (form: ControlFormState) => void;
  onClose: () => void;
}

function ControlFormDialog({
  open,
  title,
  initialValues,
  users,
  teams,
  isSaving,
  error,
  editMode = false,
  onSave,
  onClose,
}: ControlFormDialogProps): JSX.Element {
  const [form, setForm] = useState<ControlFormState>(initialValues);

  // Reset form when dialog opens with new initial values
  const handleOpen = () => setForm(initialValues);

  const set = <K extends keyof ControlFormState>(key: K, val: ControlFormState[K]) =>
    setForm((prev) => ({ ...prev, [key]: val }));

  const isOE = form.requirementType === "OE";
  // A due date can't be moved into the past — but editing a control that's
  // already overdue must not be blocked as long as its due date is left
  // untouched (only a newly-picked date is checked against today). An empty
  // field is caught separately by isValid below (via "required"), not here —
  // otherwise the past-date message would show before the user has typed
  // anything.
  const dueDateInPast =
    form.dueDate.length > 0 &&
    form.dueDate < todayISO() &&
    !(editMode && form.dueDate === initialValues.dueDate);
  const dueDateValid = form.dueDate.length > 0 && !dueDateInPast;
  const populationDueDateInPast =
    form.populationDueDate.length > 0 &&
    form.populationDueDate < todayISO() &&
    !(editMode && form.populationDueDate === initialValues.populationDueDate);
  const populationDueDateValid = form.populationDueDate.length > 0 && !populationDueDateInPast;
  const isValid =
    form.controlNumber.trim().length > 0 &&
    form.description.trim().length > 0 &&
    form.evidenceRequirement.trim().length > 0 &&
    dueDateValid &&
    (!isOE || (form.populationDescription.trim().length > 0 && populationDueDateValid));

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      TransitionProps={{ onEnter: handleOpen }}
    >
      <DialogTitle sx={{ fontWeight: 700 }}>{title}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2} sx={{ pt: 1 }}>
          {error && <Alert severity="error">{error}</Alert>}

          <Stack direction="row" spacing={2}>
            <TextField
              label="Control Number"
              required
              value={form.controlNumber}
              onChange={(e) => set("controlNumber", e.target.value)}
              size="small"
              sx={{ width: 160, flexShrink: 0 }}
              InputProps={{ readOnly: editMode }}
            />
            <TextField
              label="Description"
              required
              value={form.description}
              onChange={(e) => set("description", e.target.value)}
              size="small"
              fullWidth
              multiline
              maxRows={3}
            />
          </Stack>

          <Stack direction="row" spacing={2}>
            <FormControl size="small" required sx={{ flex: 1 }}>
              <InputLabel>Req. Type</InputLabel>
              <Select
                label="Req. Type"
                value={form.requirementType}
                onChange={(e) => set("requirementType", e.target.value as RequirementType)}
                disabled={editMode}
              >
                <MenuItem value="DESIGN">Design</MenuItem>
                <MenuItem value="OE">OE</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" required sx={{ flex: 1 }}>
              <InputLabel>Control Type</InputLabel>
              <Select
                label="Control Type"
                value={form.controlType}
                onChange={(e) => set("controlType", e.target.value as ControlType)}
              >
                <MenuItem value="CONFIG">Config</MenuItem>
                <MenuItem value="NON_CONFIG">Non-Config</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" required sx={{ flex: 1 }}>
              <InputLabel>Scope</InputLabel>
              <Select
                label="Scope"
                value={form.scope}
                onChange={(e) => set("scope", e.target.value as ControlScope)}
              >
                <MenuItem value="COMMON">Common</MenuItem>
                <MenuItem value="PRODUCT_SPECIFIC">Product Specific</MenuItem>
              </Select>
            </FormControl>
          </Stack>

          <Stack direction="row" spacing={2}>
            <Autocomplete
              options={users.filter((u) => u.userType === "INTERNAL")}
              getOptionLabel={(u) => u.displayName}
              value={form.owner}
              onChange={(_e, val) => set("owner", val)}
              size="small"
              sx={{ flex: 1 }}
              renderInput={(params) => <TextField {...params} label="Process Owner" />}
            />
            <Autocomplete
              options={[...users].sort((a, b) => {
                if (a.userType === b.userType) return a.displayName.localeCompare(b.displayName);
                return a.userType === "EXTERNAL" ? -1 : 1;
              })}
              groupBy={(u) => u.userType === "EXTERNAL" ? "External Auditors" : "Internal"}
              getOptionLabel={(u) => u.displayName}
              value={form.auditor}
              onChange={(_e, val) => set("auditor", val)}
              size="small"
              sx={{ flex: 1 }}
              renderInput={(params) => <TextField {...params} label="Auditor" />}
            />
          </Stack>

          <Autocomplete
            options={teams}
            getOptionLabel={(t) => t.name}
            value={form.team}
            onChange={(_e, val) => set("team", val)}
            size="small"
            fullWidth
            renderInput={(params) => <TextField {...params} label="Team" />}
          />

          <TextField
            label="Due Date"
            type="date"
            required
            value={form.dueDate}
            onChange={(e) => set("dueDate", e.target.value)}
            size="small"
            InputLabelProps={{ shrink: true }}
            inputProps={{ min: todayISO() }}
            error={dueDateInPast}
            helperText={dueDateInPast ? "Due Date cannot be in the past" : " "}
          />

          <TextField
            label="Evidence Requirement"
            required
            value={form.evidenceRequirement}
            onChange={(e) => set("evidenceRequirement", e.target.value)}
            size="small"
            multiline
            rows={3}
            fullWidth
          />

          {isOE && (
            <>
              <Divider>
                <Typography variant="caption" color="text.secondary" fontWeight={600}>
                  Population Details
                </Typography>
              </Divider>

              <TextField
                label="Population Requirement"
                required
                value={form.populationDescription}
                onChange={(e) => set("populationDescription", e.target.value)}
                size="small"
                multiline
                rows={3}
                fullWidth
                placeholder="Describe what records make up the population"
              />

              <TextField
                label="Population Due Date"
                type="date"
                required
                value={form.populationDueDate}
                onChange={(e) => set("populationDueDate", e.target.value)}
                size="small"
                InputLabelProps={{ shrink: true }}
                fullWidth
                inputProps={{ min: todayISO() }}
                error={populationDueDateInPast}
                helperText={populationDueDateInPast ? "Population Due Date cannot be in the past" : " "}
              />

              <TextField
                label="Population Comments"
                value={form.populationComments}
                onChange={(e) => set("populationComments", e.target.value)}
                size="small"
                multiline
                rows={2}
                fullWidth
                placeholder="Any additional notes for the population submission"
              />

              <Stack direction="row" spacing={2}>
                <Autocomplete
                  options={users.filter((u) => u.userType === "INTERNAL")}
                  getOptionLabel={(u) => u.displayName}
                  value={form.populationOwner}
                  onChange={(_e, val) => set("populationOwner", val)}
                  size="small"
                  sx={{ flex: 1 }}
                  renderInput={(params) => <TextField {...params} label="Population Process Owner" />}
                />
                <Autocomplete
                  options={teams}
                  getOptionLabel={(t) => t.name}
                  value={form.populationTeam}
                  onChange={(_e, val) => set("populationTeam", val)}
                  size="small"
                  sx={{ flex: 1 }}
                  renderInput={(params) => <TextField {...params} label="Population Team" />}
                />
              </Stack>
            </>
          )}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose} variant="outlined" disabled={isSaving}>
          Cancel
        </Button>
        <Button
          onClick={() => onSave(form)}
          variant="contained"
          disabled={!isValid || isSaving}
          startIcon={isSaving ? <CircularProgress size={14} /> : undefined}
        >
          {isSaving ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// ── Delete confirm dialog ────────────────────────────────────────────────────

interface DeleteDialogProps {
  open: boolean;
  control: AuditControl | null;
  isDeleting: boolean;
  error: string | null;
  onConfirm: () => void;
  onClose: () => void;
}

function DeleteDialog({
  open,
  control,
  isDeleting,
  error,
  onConfirm,
  onClose,
}: DeleteDialogProps): JSX.Element {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontWeight: 700 }}>Remove control?</DialogTitle>
      <DialogContent>
        {error && <Alert severity="error" sx={{ mb: 1 }}>{error}</Alert>}
        <Typography variant="body2">
          Remove <strong>{control?.controlNumber}</strong>  {control?.description}?
          This cannot be undone.
        </Typography>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose} variant="outlined" disabled={isDeleting}>
          Cancel
        </Button>
        <Button
          onClick={onConfirm}
          variant="contained"
          color="error"
          disabled={isDeleting}
          startIcon={isDeleting ? <CircularProgress size={14} /> : undefined}
        >
          {isDeleting ? "Removing…" : "Remove"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// ── ControlSettingsPanel (main export) ──────────────────────────────────────

interface ControlSettingsPanelProps {
  auditId: number;
  open: boolean;
  onClose: () => void;
}

export default function ControlSettingsPanel({
  auditId,
  open,
  onClose,
}: ControlSettingsPanelProps): JSX.Element {
  const { can } = useAuditPrivileges();
  const canManage = can(AuditPrivilege.ManageControls);

  const { data: controlsData, isLoading: controlsLoading } = useGetControls(auditId);
  const { data: users = [] } = useGetUsers();
  const { data: teams = [] } = useGetTeams();

  const addMutation = useAddControl();
  const updateMutation = useUpdateControl();
  const deleteMutation = useDeleteControl();

  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [editingControl, setEditingControl] = useState<AuditControl | null>(null);
  const [deletingControl, setDeletingControl] = useState<AuditControl | null>(null);
  const [mutationError, setMutationError] = useState<string | null>(null);

  const controls = controlsData?.items ?? [];

  function handleAdd(form: ControlFormState) {
    setMutationError(null);
    const population: PopulationDetails | null =
      form.requirementType === "OE"
        ? {
            description: form.populationDescription.trim(),
            dueDate: form.populationDueDate || null,
            comments: form.populationComments.trim() || null,
            ownerId: form.populationOwner?.id ?? null,
            teamId: form.populationTeam?.id ?? null,
          }
        : null;
    const req: AddControlRequest = {
      controlNumber: form.controlNumber.trim(),
      description: form.description.trim(),
      requirementType: form.requirementType,
      controlType: form.controlType,
      scope: form.scope,
      evidenceRequirement: form.evidenceRequirement.trim() || null,
      dueDate: form.dueDate || null,
      ownerId: form.owner?.id ?? null,
      teamId: form.team?.id ?? null,
      auditorId: form.auditor?.id ?? null,
      controlSource: 'MANUAL' as const,
      population,
    };
    addMutation.mutate(
      { auditId, req },
      {
        onSuccess: () => setAddDialogOpen(false),
        onError: (e) => setMutationError(e instanceof Error ? e.message : "Failed to add control"),
      },
    );
  }

  function handleEdit(form: ControlFormState) {
    if (!editingControl) return;
    setMutationError(null);
    const population: PopulationDetails | null =
      form.requirementType === "OE"
        ? {
            description: form.populationDescription.trim(),
            dueDate: form.populationDueDate || null,
            comments: form.populationComments.trim() || null,
            ownerId: form.populationOwner?.id ?? null,
            teamId: form.populationTeam?.id ?? null,
          }
        : null;
    const req: UpdateControlRequest = {
      description: form.description.trim(),
      controlType: form.controlType,
      scope: form.scope,
      evidenceRequirement: form.evidenceRequirement.trim() || null,
      dueDate: form.dueDate || null,
      ownerId: form.owner?.id ?? null,
      teamId: form.team?.id ?? null,
      auditorId: form.auditor?.id ?? null,
      population,
    };
    updateMutation.mutate(
      { auditId, controlId: editingControl.id, req },
      {
        onSuccess: () => setEditingControl(null),
        onError: (e) => setMutationError(e instanceof Error ? e.message : "Failed to update control"),
      },
    );
  }

  function handleDelete() {
    if (!deletingControl) return;
    setMutationError(null);
    deleteMutation.mutate(
      { auditId, controlId: deletingControl.id },
      {
        onSuccess: () => setDeletingControl(null),
        onError: (e) => setMutationError(e instanceof Error ? e.message : "Failed to remove control"),
      },
    );
  }

  return (
    <>
      <Drawer
        anchor="right"
        open={open}
        onClose={onClose}
        PaperProps={{ sx: { width: { xs: "100%", sm: 700 } } }}
      >
        {/* Header */}
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            px: 3,
            py: 2,
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          <Typography variant="h6" fontWeight={700}>
            Manage Controls
          </Typography>
          <Stack direction="row" spacing={1} alignItems="center">
            {canManage && (
              <Button
                variant="contained"
                size="small"
                startIcon={<Plus size={14} />}
                onClick={() => {
                  setMutationError(null);
                  setAddDialogOpen(true);
                }}
                sx={{ textTransform: "none" }}
              >
                Add Control
              </Button>
            )}
            <Tooltip title="Close">
              <IconButton onClick={onClose} size="small">
                <X size={18} />
              </IconButton>
            </Tooltip>
          </Stack>
        </Box>

        {/* Body */}
        <Box sx={{ overflow: "auto", flex: 1, p: 2.5 }}>
          {controlsLoading ? (
            <Paper variant="outlined" sx={{ borderRadius: 2, p: 2.5 }}>
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} variant="rectangular" height={48} sx={{ mb: 1, borderRadius: 1 }} />
              ))}
            </Paper>
          ) : controls.length === 0 ? (
            <Paper variant="outlined" sx={{ borderRadius: 2, p: 4, textAlign: "center" }}>
              <Typography variant="body2" color="text.secondary">
                No controls yet. Click "Add Control" to get started.
              </Typography>
            </Paper>
          ) : (
            <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden" }}>
              <TableContainer>
                <Table size="small" stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 600, width: 90, bgcolor: "action.hover" }}>No.</TableCell>
                      <TableCell sx={{ fontWeight: 600, bgcolor: "action.hover" }}>Description</TableCell>
                      <TableCell sx={{ fontWeight: 600, width: 90, bgcolor: "action.hover" }}>Req. Type</TableCell>
                      {canManage && (
                        <TableCell sx={{ fontWeight: 600, width: 80, bgcolor: "action.hover" }} align="right">
                          Actions
                        </TableCell>
                      )}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {controls.map((c) => (
                      <TableRow key={c.id} hover>
                        <TableCell>
                          <Typography variant="body2" fontWeight={600} noWrap>
                            {c.controlNumber}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Typography variant="body2" sx={{ lineHeight: 1.5 }}>
                            {c.description}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption">{c.requirementType === "OE" ? "OE" : "Design"}</Typography>
                        </TableCell>
                        {canManage && (
                          <TableCell align="right">
                            <Stack direction="row" spacing={0.5} justifyContent="flex-end">
                              <Tooltip title="Edit">
                                <IconButton
                                  size="small"
                                  onClick={() => {
                                    setMutationError(null);
                                    setEditingControl(c);
                                  }}
                                >
                                  <Pencil size={14} />
                                </IconButton>
                              </Tooltip>
                              <Tooltip title="Remove">
                                <IconButton
                                  size="small"
                                  color="error"
                                  onClick={() => {
                                    setMutationError(null);
                                    setDeletingControl(c);
                                  }}
                                >
                                  <Trash2 size={14} />
                                </IconButton>
                              </Tooltip>
                            </Stack>
                          </TableCell>
                        )}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            </Paper>
          )}
        </Box>

        {/* Footer */}
        <Divider />
        <Box sx={{ px: 3, py: 1.5 }}>
          <Typography variant="caption" color="text.secondary">
            {controls.length} control{controls.length !== 1 ? "s" : ""}
          </Typography>
        </Box>
      </Drawer>

      {/* Add dialog */}
      <ControlFormDialog
        open={addDialogOpen}
        title="Add Control"
        initialValues={EMPTY_FORM}
        users={users}
        teams={teams}
        isSaving={addMutation.isPending}
        error={mutationError}
        onSave={handleAdd}
        onClose={() => setAddDialogOpen(false)}
      />

      {/* Edit dialog */}
      <ControlFormDialog
        open={editingControl !== null}
        title={`Edit ${editingControl?.controlNumber ?? ""}`}
        initialValues={editingControl ? controlToForm(editingControl, users, teams) : EMPTY_FORM}
        users={users}
        teams={teams}
        isSaving={updateMutation.isPending}
        error={mutationError}
        editMode
        onSave={handleEdit}
        onClose={() => setEditingControl(null)}
      />

      {/* Delete confirm dialog */}
      <DeleteDialog
        open={deletingControl !== null}
        control={deletingControl}
        isDeleting={deleteMutation.isPending}
        error={mutationError}
        onConfirm={handleDelete}
        onClose={() => setDeletingControl(null)}
      />
    </>
  );
}
