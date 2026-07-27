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
  Card,
  CardActionArea,
  CardContent,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  MenuItem,
  Paper,
  Select,
  Step,
  StepLabel,
  Stepper,
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
import {
  ChevronLeft,
  ClipboardList,
  Copy,
  FileUp,
  Library,
  Pencil,
  Plus,
  Trash2,
} from "@wso2/oxygen-ui-icons-react";
import { useState, useRef, useEffect, type JSX, type ChangeEvent } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useGetControls } from "@modules/audit/api/useGetControls";
import { useGetFrameworkControls } from "@modules/audit/api/useGetFrameworkControls";
import { useGetFrameworks } from "@modules/audit/api/useGetFrameworks";
import { useGetProducts } from "@modules/audit/api/useGetProducts";
import { useGetUsers } from "@modules/audit/api/useGetUsers";
import { useGetTeams } from "@modules/audit/api/useGetTeams";
import { useCreateAudit } from "@modules/audit/api/useCreateAudit";
import { useCreateFramework } from "@modules/audit/api/useCreateFramework";
import { useCreateProduct } from "@modules/audit/api/useCreateProduct";
import { useCreateFrameworkControl } from "@modules/audit/api/useCreateFrameworkControl";
import { useBulkAddControls } from "@modules/audit/api/useBulkAddControls";
import type {
  AddControlRequest,
  AuditControl,
  AuditFramework,
  AuditFrameworkControl,
  AuditProduct,
  AuditTeam,
  ControlScope,
  ControlType,
  PopulationDetails,
  RequirementType,
} from "@modules/audit/types/audit";
import type { AuditUser } from "@modules/audit/types/user";

// ── Types ─────────────────────────────────────────────────────────────────────

type ControlSourceMode = "empty" | "copy" | "csv" | "template";

// The app's default theme (AcrylicOrangeTheme) makes `background.paper` an
// intentionally translucent color (~77% opacity: #ffffffc5 / #000000c5) and
// layers a backdrop blur on top, for its "frosted glass" look. That's fine
// for panels sitting over page content, but unreadable for a dropdown menu
// floating over a dense form or table — stripping just the blur isn't enough
// since the color underneath is still see-through. Force a fully opaque
// surface instead, matching the same fix already used for the DatePicker's
// popover elsewhere in the app (see BasicInformationStep.tsx).
const OPAQUE_DROPDOWN_PAPER_SX = {
  backdropFilter: "none",
  WebkitBackdropFilter: "none",
  backgroundColor: "#fff",
  backgroundImage: "none",
  "[data-color-scheme='dark'] &": { backgroundColor: "#1e1e1e" },
} as const;

// Autocomplete's dropdown Paper — pass as `slotProps={{ paper: DROPDOWN_PAPER_PROPS }}`.
const DROPDOWN_PAPER_PROPS = { sx: OPAQUE_DROPDOWN_PAPER_SX };

// Plain <Select> menus need the same override via MenuProps — the Menu's Paper
// is a separate slot from the field itself, so Autocomplete's fix doesn't cover it.
const SELECT_MENU_PROPS = {
  slotProps: { paper: DROPDOWN_PAPER_PROPS },
};

let _localIdCounter = 0;
const nextLocalId = () => String(++_localIdCounter);

interface PopulationDraft {
  description: string;
  dueDate: string;
  comments: string;
  /** Population-phase process owner — may differ from the control's process owner. */
  ownerId: number | null;
  /** Population-phase team — may differ from the control's team. */
  teamId: number | null;
}

function blankPopulation(): PopulationDraft {
  return { description: "", dueDate: "", comments: "", ownerId: null, teamId: null };
}

interface DraftControl {
  localId: string;
  // When set this draft is linked to a framework template row (source = COPIED).
  // Definition fields below are read-only display values; only assignments are editable.
  frameworkControlId?: number;
  controlSource: "MANUAL" | "COPIED" | "CSV";
  controlNumber: string;
  description: string;
  requirementType: RequirementType;
  controlType: ControlType;
  scope: ControlScope;
  evidenceRequirement: string;
  dueDate: string;
  ownerId: number | null;
  teamId: number | null;
  auditorId: number | null;
  population: PopulationDraft | null; // non-null for OE controls
}

function blankDraft(): DraftControl {
  return {
    localId: nextLocalId(),
    controlSource: "MANUAL",
    controlNumber: "",
    description: "",
    requirementType: "DESIGN",
    controlType: "NON_CONFIG",
    scope: "COMMON",
    evidenceRequirement: "",
    dueDate: "",
    ownerId: null,
    teamId: null,
    auditorId: null,
    population: null,
  };
}

function controlToDraft(c: AuditControl): DraftControl {
  return {
    localId: nextLocalId(),
    frameworkControlId: c.frameworkControlId ?? undefined,
    controlSource: "COPIED",
    controlNumber: c.controlNumber,
    description: c.description,
    requirementType: c.requirementType,
    controlType: c.controlType,
    scope: c.scope,
    evidenceRequirement: c.evidenceRequirement ?? "",
    dueDate: c.dueDate ?? "",
    ownerId: c.ownerId ?? null,
    teamId: c.teamId ?? null,
    auditorId: c.auditorId ?? null,
    population: c.requirementType === "OE" ? blankPopulation() : null,
  };
}

function draftToRequest(d: DraftControl): AddControlRequest {
  let population: PopulationDetails | null = null;
  if (d.requirementType === "OE" && d.population) {
    population = {
      description: d.population.description.trim(),
      dueDate: d.population.dueDate || null,
      comments: d.population.comments.trim() || null,
      ownerId: d.population.ownerId,
      teamId: d.population.teamId,
    };
  }
  // Template-linked: send only the FK + assignments; backend resolves definitions from the template.
  if (d.frameworkControlId) {
    return {
      frameworkControlId: d.frameworkControlId,
      controlSource: "COPIED" as const,
      requirementType: d.requirementType,
      controlType: d.controlType,
      scope: d.scope,
      dueDate: d.dueDate || null,
      ownerId: d.ownerId,
      teamId: d.teamId,
      auditorId: d.auditorId,
      population,
    };
  }
  return {
    controlNumber: d.controlNumber.trim(),
    description: d.description.trim(),
    requirementType: d.requirementType,
    controlType: d.controlType,
    scope: d.scope,
    evidenceRequirement: d.evidenceRequirement.trim() || null,
    dueDate: d.dueDate || null,
    ownerId: d.ownerId,
    teamId: d.teamId,
    auditorId: d.auditorId,
    controlSource: d.controlSource,
    population,
  };
}

// ── CSV parsing ───────────────────────────────────────────────────────────────

// Required CSV columns. Optional: requirement_type, control_type, scope, due_date.
// Columns for auditor_poc, process_owner, team are not supported via CSV
// because they require database IDs — set them manually after upload.
const CSV_REQUIRED_COLS = ["control_number", "description", "evidence_requirement"];

function parseCSV(text: string): DraftControl[] | string {
  const lines = text.trim().split(/\r?\n/);
  if (lines.length < 2) return "CSV must have a header row and at least one data row.";
  const headers = lines[0].split(",").map((h) => h.trim().toLowerCase().replace(/\s+/g, "_"));
  const missing = CSV_REQUIRED_COLS.filter((c) => !headers.includes(c));
  if (missing.length > 0) return `Missing required columns: ${missing.join(", ")}`;

  const idx = (name: string) => headers.indexOf(name);
  const has = (name: string) => idx(name) >= 0;
  const validReq = (v: string): v is RequirementType => v === "DESIGN" || v === "OE";
  const validCtl = (v: string): v is ControlType => v === "CONFIG" || v === "NON_CONFIG";
  const validScope = (v: string): v is ControlScope => v === "COMMON" || v === "PRODUCT_SPECIFIC";

  const drafts: DraftControl[] = [];
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line) continue;
    // Simple split — fields with commas must be quoted. Basic unquoting only.
    const cells = line.match(/(".*?"|[^,]+|(?<=,)(?=,)|(?<=,)$|^(?=,))/g)?.map((c) =>
      c.startsWith('"') ? c.slice(1, -1) : c.trim(),
    ) ?? line.split(",").map((c) => c.trim());

    const cn   = cells[idx("control_number")] ?? "";
    const desc = cells[idx("description")] ?? "";
    if (!cn || !desc) continue;

    // Optional columns — validate only when the column is present; fall back to defaults.
    const rawReq   = has("requirement_type") ? (cells[idx("requirement_type")] ?? "").toUpperCase() : "DESIGN";
    const rawCtl   = has("control_type")     ? (cells[idx("control_type")]     ?? "").toUpperCase() : "NON_CONFIG";
    const rawScope = has("scope")            ? (cells[idx("scope")]            ?? "").toUpperCase() : "COMMON";

    if (!validReq(rawReq))
      return `Row ${i + 1}: requirement_type must be DESIGN or OE, got "${rawReq}"`;
    if (!validCtl(rawCtl))
      return `Row ${i + 1}: control_type must be CONFIG or NON_CONFIG, got "${rawCtl}"`;
    if (!validScope(rawScope))
      return `Row ${i + 1}: scope must be COMMON or PRODUCT_SPECIFIC, got "${rawScope}"`;

    const pop: PopulationDraft | null = rawReq === "OE"
      ? {
          description: has("population_description") ? (cells[idx("population_description")] ?? "") : "",
          dueDate:     has("population_due_date")     ? (cells[idx("population_due_date")]    ?? "") : "",
          comments:    has("population_comments")     ? (cells[idx("population_comments")]    ?? "") : "",
          ownerId:     null,
          teamId:      null,
        }
      : null;

    drafts.push({
      localId: nextLocalId(),
      controlSource: "CSV",
      controlNumber: cn,
      description: desc,
      requirementType: rawReq,
      controlType: rawCtl,
      scope: rawScope,
      evidenceRequirement: cells[idx("evidence_requirement")] ?? "",
      dueDate: has("due_date") ? (cells[idx("due_date")] ?? "") : "",
      // Auditor POC / Process Owner / Team require database IDs — not supported via CSV.
      ownerId: null,
      teamId: null,
      auditorId: null,
      population: pop,
    });
  }
  if (drafts.length === 0) return "No valid rows found in CSV.";
  return drafts;
}

// ── Population details dialog ─────────────────────────────────────────────────

interface PopulationDialogProps {
  open: boolean;
  controlDraft: DraftControl;
  onClose: () => void;
  onChangePopulation: (p: PopulationDraft) => void;
  onChangeAuditor: (val: number | null) => void;
  users: AuditUser[];
  teams: AuditTeam[];
}

function PopulationDialog({
  open, controlDraft, onClose, onChangePopulation, onChangeAuditor, users, teams,
}: PopulationDialogProps): JSX.Element {
  const pop = controlDraft.population ?? blankPopulation();
  const paperProps = DROPDOWN_PAPER_PROPS;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      slotProps={{ paper: { sx: { backgroundImage: "none", borderRadius: 2 } } }}
    >
      <DialogTitle>
        Population Details - {controlDraft.controlNumber || "New Control"}
      </DialogTitle>
      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, pt: "16px !important" }}>

        {/* Population description */}
        <TextField
          label="Population Requirement"
          required
          multiline
          rows={3}
          fullWidth
          value={pop.description}
          onChange={(e) => onChangePopulation({ ...pop, description: e.target.value })}
          placeholder="Describe what records make up the population (e.g. all access review records for Jan–Dec 2026)"
        />

        {/* Due date */}
        <TextField
          label="Population Due Date"
          required
          type="date"
          fullWidth
          value={pop.dueDate}
          onChange={(e) => onChangePopulation({ ...pop, dueDate: e.target.value })}
          InputLabelProps={{ shrink: true }}
          helperText="When population must be submitted"
        />

        {/* Comments — separate row so the textarea has full width */}
        <TextField
          label="Comments (optional)"
          fullWidth
          multiline
          rows={2}
          value={pop.comments}
          onChange={(e) => onChangePopulation({ ...pop, comments: e.target.value })}
          placeholder="Any additional notes for the population submission"
        />

        <Divider />

        {/* Assignments — population phase */}
        <Typography variant="subtitle2" fontWeight={600}>Assignments</Typography>
        <Alert severity="info" sx={{ py: 0.5 }}>
          Process Owner and Team here are for the population phase and may differ from the evidence phase.
          Auditor POC is shared across both phases.
        </Alert>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          {/* Process Owner — stored on population; also auto-fills the control's owner */}
          <Autocomplete
            options={users.filter((u) => u.userType === "INTERNAL")}
            getOptionLabel={(u) => u.displayName}
            isOptionEqualToValue={(a, b) => a.id === b.id}
            value={users.find((u) => u.id === pop.ownerId) ?? null}
            onChange={(_e, val) => onChangePopulation({ ...pop, ownerId: val?.id ?? null })}
            slotProps={{ paper: paperProps }}
            renderInput={(params) => <TextField {...params} label="Process Owner (Population)" />}
          />
          {/* Auditor POC — shared with control */}
          <Autocomplete
            options={[...users].sort((a, b) => {
              if (a.userType === b.userType) return a.displayName.localeCompare(b.displayName);
              return a.userType === "EXTERNAL" ? -1 : 1;
            })}
            groupBy={(u) => u.userType === "EXTERNAL" ? "External Auditors" : "Internal"}
            getOptionLabel={(u) => u.displayName}
            isOptionEqualToValue={(a, b) => a.id === b.id}
            value={users.find((u) => u.id === controlDraft.auditorId) ?? null}
            onChange={(_e, val) => onChangeAuditor(val?.id ?? null)}
            slotProps={{ paper: paperProps }}
            renderInput={(params) => <TextField {...params} label="Auditor POC" />}
          />
          {/* Team — stored on population; also auto-fills the control's team */}
          <Select
            fullWidth
            displayEmpty
            MenuProps={SELECT_MENU_PROPS}
            inputProps={{ "aria-label": "Team (Population)" }}
            value={pop.teamId !== null ? String(pop.teamId) : ""}
            onChange={(e) => {
              const v = e.target.value as string;
              onChangePopulation({ ...pop, teamId: v === "" ? null : Number(v) });
            }}
            renderValue={(v) =>
              v === ""
                ? <span style={{ opacity: 0.5 }}>Team (Population)</span>
                : (teams.find((t) => String(t.id) === v)?.name ?? "")
            }
          >
            <MenuItem value="">None</MenuItem>
            {teams.map((t) => (
              <MenuItem key={t.id} value={String(t.id)}>{t.name}</MenuItem>
            ))}
          </Select>
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={onClose} sx={{ textTransform: "none" }}>Done</Button>
      </DialogActions>
    </Dialog>
  );
}

// ── Editable controls table ───────────────────────────────────────────────────

const FS = { fontSize: "0.8rem" } as const;

interface EditableControlsTableProps {
  drafts: DraftControl[];
  onChange: (drafts: DraftControl[]) => void;
  users: AuditUser[];
  teams: AuditTeam[];
}

function EditableControlsTable({ drafts, onChange, users, teams }: EditableControlsTableProps): JSX.Element {
  const [populationDialogId, setPopulationDialogId] = useState<string | null>(null);
  const dialogDraft = drafts.find((d) => d.localId === populationDialogId);

  function update<K extends keyof DraftControl>(localId: string, key: K, val: DraftControl[K]) {
    onChange(drafts.map((d) => (d.localId === localId ? { ...d, [key]: val } : d)));
  }

  function handleReqTypeChange(localId: string, newType: RequirementType) {
    onChange(drafts.map((d) => {
      if (d.localId !== localId) return d;
      return {
        ...d,
        requirementType: newType,
        // auto-init population when switching to OE; clear it when switching to DESIGN
        population: newType === "OE" ? (d.population ?? blankPopulation()) : null,
      };
    }));
  }

  function remove(localId: string) {
    onChange(drafts.filter((d) => d.localId !== localId));
  }

  function unlinkFromTemplate(localId: string) {
    onChange(drafts.map((d) =>
      d.localId === localId ? { ...d, frameworkControlId: undefined } : d,
    ));
  }

  const paperProps = DROPDOWN_PAPER_PROPS;

  return (
    <>
    <Paper variant="outlined" sx={{ borderRadius: 2 }}>
    <TableContainer sx={{ maxHeight: 420, overflowX: "auto" }}>
      <Table size="small" stickyHeader sx={{ minWidth: 1550 }}>
        <TableHead>
          <TableRow>
            <TableCell sx={{ fontWeight: 600, minWidth: 90 }}>Control No</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 180 }}>Description</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>Evidence Requirement</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 95 }}>Req. Type</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 130 }}>Population</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 115 }}>Control Type</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 130 }}>Scope</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 155 }}>Process Owner</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 155 }}>Auditor POC</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 130 }}>Team</TableCell>
            <TableCell sx={{ fontWeight: 600, minWidth: 108 }}>Due Date</TableCell>
            <TableCell sx={{ width: 40 }} />
          </TableRow>
        </TableHead>
        <TableBody>
          {drafts.length === 0 && (
            <TableRow>
              <TableCell colSpan={12} align="center" sx={{ py: 3 }}>
                <Typography variant="body2" color="text.secondary">
                  No controls yet - click "Add Row" to begin.
                </Typography>
              </TableCell>
            </TableRow>
          )}
          {drafts.map((d) => (
            <TableRow key={d.localId}>
              {/* Control # */}
              <TableCell>
                {d.frameworkControlId ? (
                  <Typography variant="body2" fontWeight={600} noWrap sx={FS}>{d.controlNumber}</Typography>
                ) : (
                  <TextField
                    value={d.controlNumber}
                    onChange={(e) => update(d.localId, "controlNumber", e.target.value)}
                    size="small"
                    variant="standard"
                    placeholder="CA-01"
                    inputProps={{ style: FS }}
                  />
                )}
              </TableCell>
              {/* Description */}
              <TableCell>
                {d.frameworkControlId ? (
                  <Typography variant="body2" color="text.secondary" sx={{ ...FS, maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={d.description}>{d.description}</Typography>
                ) : (
                  <TextField
                    value={d.description}
                    onChange={(e) => update(d.localId, "description", e.target.value)}
                    size="small"
                    variant="standard"
                    placeholder="Description"
                    fullWidth
                    inputProps={{ style: FS }}
                  />
                )}
              </TableCell>
              {/* Evidence Requirement */}
              <TableCell>
                {d.frameworkControlId ? (
                  <Typography variant="caption" color="text.disabled" sx={{ fontStyle: "italic" }}>from library</Typography>
                ) : (
                  <TextField
                    value={d.evidenceRequirement}
                    onChange={(e) => update(d.localId, "evidenceRequirement", e.target.value)}
                    size="small"
                    variant="standard"
                    placeholder="Requirement"
                    fullWidth
                    inputProps={{ style: FS }}
                  />
                )}
              </TableCell>
              {/* Req. Type */}
              <TableCell>
                {d.frameworkControlId ? (
                  <Chip label={d.requirementType} size="small" color={d.requirementType === "OE" ? "warning" : "default"} sx={{ height: 18, fontSize: "0.65rem" }} />
                ) : (
                  <Select
                    value={d.requirementType}
                    onChange={(e) => handleReqTypeChange(d.localId, e.target.value as RequirementType)}
                    size="small"
                    variant="standard"
                    MenuProps={SELECT_MENU_PROPS}
                    sx={{ ...FS }}
                  >
                    <MenuItem value="DESIGN">Design</MenuItem>
                    <MenuItem value="OE">OE</MenuItem>
                  </Select>
                )}
              </TableCell>
              {/* Population — only active for OE rows */}
              <TableCell>
                {d.requirementType === "OE" ? (
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Tooltip title="Edit population details">
                      <IconButton
                        size="small"
                        color={d.population?.description ? "primary" : "default"}
                        onClick={() => setPopulationDialogId(d.localId)}
                      >
                        <ClipboardList size={14} />
                      </IconButton>
                    </Tooltip>
                    <Typography
                      variant="caption"
                      color={d.population?.description ? "text.secondary" : "warning.main"}
                    >
                      {d.population?.description ? "Set" : "Not set"}
                    </Typography>
                  </Box>
                ) : (
                  <Typography variant="caption" color="text.disabled">—</Typography>
                )}
              </TableCell>
              {/* Control Type */}
              <TableCell>
                {d.frameworkControlId ? (
                  <Typography variant="caption" color="text.secondary" sx={FS}>{d.controlType === "CONFIG" ? "Config" : "Non-Config"}</Typography>
                ) : (
                  <Select
                    value={d.controlType}
                    onChange={(e) => update(d.localId, "controlType", e.target.value as ControlType)}
                    size="small"
                    variant="standard"
                    MenuProps={SELECT_MENU_PROPS}
                    sx={{ ...FS }}
                  >
                    <MenuItem value="CONFIG">Config</MenuItem>
                    <MenuItem value="NON_CONFIG">Non-Config</MenuItem>
                  </Select>
                )}
              </TableCell>
              {/* Scope */}
              <TableCell>
                {d.frameworkControlId ? (
                  <Typography variant="caption" color="text.secondary" sx={FS}>{d.scope === "COMMON" ? "Common" : "Product"}</Typography>
                ) : (
                  <Select
                    value={d.scope}
                    onChange={(e) => update(d.localId, "scope", e.target.value as ControlScope)}
                    size="small"
                    variant="standard"
                    MenuProps={SELECT_MENU_PROPS}
                    sx={{ ...FS }}
                  >
                    <MenuItem value="COMMON">Common</MenuItem>
                    <MenuItem value="PRODUCT_SPECIFIC">Product Specific</MenuItem>
                  </Select>
                )}
              </TableCell>
              {/* Process Owner — internal users only */}
              <TableCell>
                <Autocomplete
                  size="small"
                  options={users.filter((u) => u.userType === "INTERNAL")}
                  getOptionLabel={(u) => u.displayName}
                  isOptionEqualToValue={(a, b) => a.id === b.id}
                  value={users.find((u) => u.id === d.ownerId) ?? null}
                  onChange={(_e, val) => update(d.localId, "ownerId", val?.id ?? null)}
                  renderInput={(params) => (
                    <TextField
                      {...params}
                      variant="standard"
                      placeholder="Search…"
                      inputProps={{ ...params.inputProps, style: FS }}
                    />
                  )}
                  sx={{ minWidth: 135 }}
                  slotProps={{ paper: paperProps }}
                />
              </TableCell>
              {/* Auditor POC — external auditors first, then internal */}
              <TableCell>
                <Autocomplete
                  size="small"
                  options={[...users].sort((a, b) => {
                    if (a.userType === b.userType) return a.displayName.localeCompare(b.displayName);
                    return a.userType === "EXTERNAL" ? -1 : 1;
                  })}
                  groupBy={(u) => u.userType === "EXTERNAL" ? "External Auditors" : "Internal"}
                  getOptionLabel={(u) => u.displayName}
                  isOptionEqualToValue={(a, b) => a.id === b.id}
                  value={users.find((u) => u.id === d.auditorId) ?? null}
                  onChange={(_e, val) => update(d.localId, "auditorId", val?.id ?? null)}
                  renderInput={(params) => (
                    <TextField
                      {...params}
                      variant="standard"
                      placeholder="Search…"
                      inputProps={{ ...params.inputProps, style: FS }}
                    />
                  )}
                  sx={{ minWidth: 135 }}
                  slotProps={{ paper: paperProps, popper: { style: { minWidth: 220 } } }}
                />
              </TableCell>
              {/* Team */}
              <TableCell>
                <Select
                  value={d.teamId !== null ? String(d.teamId) : ""}
                  onChange={(e) => {
                    const v = e.target.value as string;
                    update(d.localId, "teamId", v === "" ? null : Number(v));
                  }}
                  size="small"
                  variant="standard"
                  displayEmpty
                  MenuProps={SELECT_MENU_PROPS}
                  sx={{ ...FS, minWidth: 110 }}
                >
                  <MenuItem value=""><em style={{ color: "#9e9e9e" }}>None</em></MenuItem>
                  {teams.map((t) => (
                    <MenuItem key={t.id} value={String(t.id)} sx={FS}>
                      {t.name}
                    </MenuItem>
                  ))}
                </Select>
              </TableCell>
              {/* Due Date — at the end */}
              <TableCell>
                <TextField
                  value={d.dueDate}
                  onChange={(e) => update(d.localId, "dueDate", e.target.value)}
                  type="date"
                  size="small"
                  variant="standard"
                  InputLabelProps={{ shrink: true }}
                  inputProps={{ style: FS }}
                />
              </TableCell>
              <TableCell>
                <Box sx={{ display: "flex" }}>
                  {d.frameworkControlId && (
                    <Tooltip title="Edit definition (unlinks from library template)">
                      <IconButton size="small" onClick={() => unlinkFromTemplate(d.localId)}>
                        <Pencil size={14} />
                      </IconButton>
                    </Tooltip>
                  )}
                  <Tooltip title="Remove row">
                    <IconButton size="small" color="error" onClick={() => remove(d.localId)}>
                      <Trash2 size={14} />
                    </IconButton>
                  </Tooltip>
                </Box>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
    </Paper>

    {/* Population details dialog — rendered outside the table to avoid z-index issues */}
    {dialogDraft && (
      <PopulationDialog
        open={Boolean(populationDialogId)}
        controlDraft={dialogDraft}
        onClose={() => setPopulationDialogId(null)}
        onChangePopulation={(p) => {
          // Auto-fill the control's owner/team from the population as a default.
          // They remain independently editable in the main table row.
          onChange(drafts.map((d) => {
            if (d.localId !== populationDialogId) return d;
            return {
              ...d,
              population: p,
              ownerId: p.ownerId ?? d.ownerId,
              teamId: p.teamId ?? d.teamId,
            };
          }));
        }}
        onChangeAuditor={(val) => {
          // Auditor POC is shared — updating it here updates the control directly.
          onChange(drafts.map((d) =>
            d.localId === populationDialogId ? { ...d, auditorId: val } : d,
          ));
        }}
        users={users}
        teams={teams}
      />
    )}
    </>
  );
}

// ── Source option card ────────────────────────────────────────────────────────

interface SourceCardProps {
  icon: JSX.Element;
  title: string;
  description: string;
  selected: boolean;
  onClick: () => void;
}

function SourceCard({ icon, title, description, selected, onClick }: SourceCardProps): JSX.Element {
  return (
    <Card
      variant="outlined"
      sx={{
        borderRadius: 2,
        borderColor: selected ? "primary.main" : "divider",
        borderWidth: selected ? 2 : 1,
        transition: "border-color 0.15s, box-shadow 0.15s",
        boxShadow: selected ? "0 0 0 3px rgba(25,118,210,0.15)" : "none",
        height: "100%",
      }}
    >
      <CardActionArea onClick={onClick} sx={{ p: 0, height: "100%" }}>
        <CardContent sx={{ display: "flex", alignItems: "flex-start", gap: 2 }}>
          <Box
            sx={{
              width: 44,
              height: 44,
              borderRadius: 2,
              // Fixed literal colors on purpose, not theme tokens: "primary.50" isn't
              // even a real shade in this app's palettes (only main/light/dark are
              // defined, so it silently rendered no background at all), and
              // "text.secondary" lightens in dark mode — paired with a light box,
              // that's a white icon on a white box. The icon should just always read
              // as a dark glyph on a light chip, in every theme and mode; selection is
              // already shown by the card's border/glow, so this box doesn't need to
              // change with it.
              bgcolor: "#f0f0f0",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#1a1a1a",
              flexShrink: 0,
            }}
          >
            {icon}
          </Box>
          <Box>
            <Typography variant="subtitle2" fontWeight={700}>
              {title}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25 }}>
              {description}
            </Typography>
          </Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );
}

// ── Step 1: Audit details ─────────────────────────────────────────────────────

interface Step1Props {
  name: string;
  nameConflict: boolean;
  framework: AuditFramework | null;
  product: AuditProduct | null;
  periodStart: string;
  periodEnd: string;
  scopeDescription: string;
  frameworks: AuditFramework[];
  products: AuditProduct[];
  loadingFrameworks: boolean;
  loadingProducts: boolean;
  errorFrameworks: boolean;
  errorProducts: boolean;
  onRefetchFrameworks: () => void;
  onRefetchProducts: () => void;
  onNameChange: (v: string) => void;
  onFrameworkChange: (v: AuditFramework | null) => void;
  onProductChange: (v: AuditProduct | null) => void;
  onPeriodStartChange: (v: string) => void;
  onPeriodEndChange: (v: string) => void;
  onScopeDescriptionChange: (v: string) => void;
}

const CREATE_FW_SENTINEL: AuditFramework = { id: -1, name: "＋ Create new framework…" };
const CREATE_PRODUCT_SENTINEL: AuditProduct = { id: -1, name: "＋ Create new product…" };

function Step1Form({
  name,
  nameConflict,
  framework,
  product,
  periodStart,
  periodEnd,
  scopeDescription,
  frameworks,
  products,
  loadingFrameworks,
  loadingProducts,
  errorFrameworks,
  errorProducts,
  onRefetchFrameworks,
  onRefetchProducts,
  onNameChange,
  onFrameworkChange,
  onProductChange,
  onPeriodStartChange,
  onPeriodEndChange,
  onScopeDescriptionChange,
}: Step1Props): JSX.Element {
  const createFramework = useCreateFramework();
  const createProduct = useCreateProduct();

  const [fwDialogOpen, setFwDialogOpen] = useState(false);
  const [newFwName, setNewFwName] = useState("");
  const [fwError, setFwError] = useState<string | null>(null);

  const [productDialogOpen, setProductDialogOpen] = useState(false);
  const [newProductName, setNewProductName] = useState("");
  const [productError, setProductError] = useState<string | null>(null);

  async function handleCreateFramework() {
    if (!newFwName.trim()) return;
    setFwError(null);
    try {
      const created = await createFramework.mutateAsync({ name: newFwName.trim() });
      onFrameworkChange(created);
      setFwDialogOpen(false);
      setNewFwName("");
    } catch (err) {
      setFwError(err instanceof Error ? err.message : "Failed to create framework.");
    }
  }

  async function handleCreateProduct() {
    if (!newProductName.trim()) return;
    setProductError(null);
    try {
      const created = await createProduct.mutateAsync({ name: newProductName.trim() });
      onProductChange(created);
      setProductDialogOpen(false);
      setNewProductName("");
    } catch (err) {
      setProductError(err instanceof Error ? err.message : "Failed to create product.");
    }
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <TextField
        label="Audit Name"
        required
        value={name}
        onChange={(e) => onNameChange(e.target.value)}
        fullWidth
        placeholder="e.g. SOC 2 Asgardeo 2026"
        error={nameConflict}
        helperText={
          nameConflict
            ? `An audit named "${name.trim()}" already exists, choose a different name.`
            : "This name is used to identify the audit in the system and reports."
        }
      />

      <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
      <Box>
        <Autocomplete
          options={[...frameworks, CREATE_FW_SENTINEL]}
          loading={loadingFrameworks}
          getOptionLabel={(f) => f.name}
          isOptionEqualToValue={(opt, val) => opt.id === val.id}
          filterOptions={(options, params) => {
            const real = options.filter((o) => o.id !== -1);
            const q = params.inputValue.toLowerCase();
            const filtered = q
              ? real.filter((o) => o.name.toLowerCase().includes(q))
              : real;
            return [...filtered, CREATE_FW_SENTINEL];
          }}
          value={framework}
          onChange={(_e, val) => {
            if (val && val.id === -1) {
              setNewFwName("");
              setFwError(null);
              setFwDialogOpen(true);
              return;
            }
            onFrameworkChange(val);
          }}
          slotProps={{ paper: DROPDOWN_PAPER_PROPS }}
          renderOption={(props, option) => {
            const { key, ...rest } = props as React.HTMLAttributes<HTMLLIElement> & { key?: React.Key };
            if (option.id === -1) {
              return (
                <li key={key} {...rest}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, color: "primary.main" }}>
                    <Plus size={14} />
                    <Typography variant="body2" fontWeight={600}>Create new framework</Typography>
                  </Box>
                </li>
              );
            }
            return (
              <li key={key} {...rest}>
                {option.name}
              </li>
            );
          }}
          renderInput={(params) => <TextField {...params} label="Framework" required />}
        />
        {errorFrameworks && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.5 }}>
            <Typography variant="caption" color="error">Failed to load frameworks.</Typography>
            <Button size="small" onClick={() => void onRefetchFrameworks()}>Retry</Button>
          </Box>
        )}
      </Box>

      <Box>
        <Autocomplete
          options={[...products, CREATE_PRODUCT_SENTINEL]}
          loading={loadingProducts}
          getOptionLabel={(p) => p.name}
          isOptionEqualToValue={(opt, val) => opt.id === val.id}
          filterOptions={(options, params) => {
            const real = options.filter((o) => o.id !== -1);
            const q = params.inputValue.toLowerCase();
            const filtered = q ? real.filter((o) => o.name.toLowerCase().includes(q)) : real;
            return [...filtered, CREATE_PRODUCT_SENTINEL];
          }}
          value={product}
          onChange={(_e, val) => {
            if (val && val.id === -1) {
              setNewProductName("");
              setProductError(null);
              setProductDialogOpen(true);
              return;
            }
            onProductChange(val);
          }}
          slotProps={{ paper: DROPDOWN_PAPER_PROPS }}
          renderOption={(props, option) => {
            const { key, ...rest } = props as React.HTMLAttributes<HTMLLIElement> & { key?: React.Key };
            if (option.id === -1) {
              return (
                <li key={key} {...rest}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, color: "primary.main" }}>
                    <Plus size={14} />
                    <Typography variant="body2" fontWeight={600}>Create new product</Typography>
                  </Box>
                </li>
              );
            }
            return <li key={key} {...rest}>{option.name}</li>;
          }}
          renderInput={(params) => <TextField {...params} label="Product / System" required />}
        />
        {errorProducts && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.5 }}>
            <Typography variant="caption" color="error">Failed to load products.</Typography>
            <Button size="small" onClick={() => void onRefetchProducts()}>Retry</Button>
          </Box>
        )}
      </Box>
      </Box>

      <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
        <TextField
          label="Period Start"
          required
          type="date"
          value={periodStart}
          onChange={(e) => onPeriodStartChange(e.target.value)}
          InputLabelProps={{ shrink: true }}
        />
        <TextField
          label="Period End"
          required
          type="date"
          value={periodEnd}
          onChange={(e) => onPeriodEndChange(e.target.value)}
          InputLabelProps={{ shrink: true }}
          inputProps={{ min: periodStart || undefined }}
          error={Boolean(periodStart && periodEnd && periodEnd < periodStart)}
          helperText={
            periodStart && periodEnd && periodEnd < periodStart
              ? "Period End cannot be before Period Start."
              : undefined
          }
        />
      </Box>

      <TextField
        label="Scope Description"
        value={scopeDescription}
        onChange={(e) => onScopeDescriptionChange(e.target.value)}
        multiline
        rows={3}
        fullWidth
        placeholder="Optional - describe what systems, processes, or criteria are in scope."
      />

      {/* Create Framework Dialog */}
      <Dialog open={fwDialogOpen} onClose={() => setFwDialogOpen(false)} fullWidth maxWidth="xs">
        <DialogTitle>Create New Framework</DialogTitle>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: "16px !important" }}>
          <TextField
            label="Framework Name"
            required
            fullWidth
            autoFocus
            value={newFwName}
            onChange={(e) => setNewFwName(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void handleCreateFramework(); }}
            placeholder="e.g. SOC 2"
          />
          {fwError && <Alert severity="error">{fwError}</Alert>}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setFwDialogOpen(false)} sx={{ textTransform: "none" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={() => void handleCreateFramework()}
            disabled={!newFwName.trim() || createFramework.isPending}
            startIcon={createFramework.isPending ? <CircularProgress size={14} /> : undefined}
            sx={{ textTransform: "none" }}
          >
            {createFramework.isPending ? "Creating…" : "Create"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Create Product Dialog */}
      <Dialog open={productDialogOpen} onClose={() => setProductDialogOpen(false)} fullWidth maxWidth="xs">
        <DialogTitle>Create New Product</DialogTitle>
        <DialogContent sx={{ pt: "16px !important" }}>
          <TextField
            label="Product / System Name"
            required
            fullWidth
            autoFocus
            value={newProductName}
            onChange={(e) => setNewProductName(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void handleCreateProduct(); }}
            placeholder="e.g. Identity Platform"
          />
          {productError && <Alert severity="error" sx={{ mt: 1.5 }}>{productError}</Alert>}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setProductDialogOpen(false)} sx={{ textTransform: "none" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={() => void handleCreateProduct()}
            disabled={!newProductName.trim() || createProduct.isPending}
            startIcon={createProduct.isPending ? <CircularProgress size={14} /> : undefined}
            sx={{ textTransform: "none" }}
          >
            {createProduct.isPending ? "Creating…" : "Create"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

// ── Step 2: Controls ──────────────────────────────────────────────────────────

interface Step2Props {
  source: ControlSourceMode;
  onSourceChange: (s: ControlSourceMode) => void;
  drafts: DraftControl[];
  onDraftsChange: (d: DraftControl[]) => void;
  copyAuditId: number | null;
  onCopyAuditIdChange: (id: number | null) => void;
  csvError: string | null;
  onCsvErrorChange: (e: string | null) => void;
  framework: AuditFramework | null;
}

function Step2Controls({
  source,
  onSourceChange,
  drafts,
  onDraftsChange,
  copyAuditId,
  onCopyAuditIdChange,
  csvError,
  onCsvErrorChange,
  framework,
}: Step2Props): JSX.Element {
  const { data: auditsData } = useGetAudits();
  const { data: sourceControlsData, isLoading: sourceControlsLoading } = useGetControls(
    copyAuditId ?? 0,
  );
  const { data: fwControlsData, isLoading: fwControlsLoading } = useGetFrameworkControls(
    source === "template" ? (framework?.id ?? null) : null,
  );
  const { data: usersData } = useGetUsers();
  const { data: teamsData } = useGetTeams();
  const users = usersData ?? [];
  const teams = teamsData ?? [];
  const fileInputRef = useRef<HTMLInputElement>(null);
  const createFrameworkControl = useCreateFrameworkControl(framework?.id ?? 0);
  const [savingAll, setSavingAll] = useState(false);
  const [saveToLibraryError, setSaveToLibraryError] = useState<string | null>(null);
  // Tracks which auditId we've already seeded into drafts, so that a
  // background refetch of sourceControlsData never overwrites user edits.
  const seededForAuditId = useRef<number | null>(null);
  // Set of selected framework control IDs for the template source.
  const selectedFwCtlIds = new Set(drafts.filter((d) => d.frameworkControlId).map((d) => d.frameworkControlId!));
  // Manually-added rows with enough filled in to be saved to the library.
  const pendingLibraryCount = drafts.filter(
    (d) => !d.frameworkControlId && d.controlNumber.trim() && d.description.trim(),
  ).length;

  // Manually-added rows (template source) are plain drafts until this runs.
  // It persists every complete, not-yet-linked row as a real framework
  // control in one batch, then re-links each to its new control — same
  // fate as picking one from the library checklist.
  async function saveAllToLibrary() {
    if (!framework) return;
    const pending = drafts.filter((d) => !d.frameworkControlId && d.controlNumber.trim() && d.description.trim());
    if (pending.length === 0) return;
    setSaveToLibraryError(null);
    setSavingAll(true);
    const results = await Promise.allSettled(pending.map((d) => createFrameworkControl.mutateAsync({
      controlNumber: d.controlNumber.trim(),
      description: d.description.trim(),
      evidenceRequirement: d.evidenceRequirement.trim() || null,
      requirementType: d.requirementType,
      controlType: d.controlType,
      scope: d.scope,
    })));
    const created = new Map(
      results.flatMap((r, i) => (r.status === "fulfilled" ? [[pending[i].localId, r.value] as const] : [])),
    );
    if (created.size > 0) {
      onDraftsChange(drafts.map((d) => {
        const fc = created.get(d.localId);
        return fc
          ? {
              ...d,
              frameworkControlId: fc.id,
              controlSource: "COPIED" as const,
              controlNumber: fc.controlNumber,
              description: fc.description,
              evidenceRequirement: fc.evidenceRequirement ?? "",
            }
          : d;
      }));
    }
    const failures = results.filter((r) => r.status === "rejected").length;
    if (failures > 0) {
      setSaveToLibraryError(`Failed to save ${failures} control${failures !== 1 ? "s" : ""} to the library. Fix and try again.`);
    }
    setSavingAll(false);
  }

  function createDraftControl(fc: AuditFrameworkControl): DraftControl {
    return {
      localId: nextLocalId(),
      frameworkControlId: fc.id,
      controlSource: "COPIED",
      controlNumber: fc.controlNumber,
      description: fc.description,
      requirementType: fc.requirementType as RequirementType,
      controlType: fc.controlType as ControlType,
      scope: fc.scope as ControlScope,
      evidenceRequirement: fc.evidenceRequirement ?? "",
      dueDate: "",
      ownerId: null,
      teamId: null,
      auditorId: null,
      population: fc.requirementType === "OE" ? blankPopulation() : null,
    };
  }

  function toggleFwControl(fc: AuditFrameworkControl) {
    if (selectedFwCtlIds.has(fc.id)) {
      onDraftsChange(drafts.filter((d) => d.frameworkControlId !== fc.id));
    } else {
      onDraftsChange([...drafts, createDraftControl(fc)]);
    }
  }

  useEffect(() => {
    if (source !== "copy") {
      seededForAuditId.current = null;
      return;
    }
    if (!sourceControlsData || copyAuditId === null) return;
    if (seededForAuditId.current === copyAuditId) return;
    seededForAuditId.current = copyAuditId;
    onDraftsChange(sourceControlsData.items.map(controlToDraft));
  }, [source, sourceControlsData, copyAuditId, onDraftsChange]);

  const allAudits = auditsData?.items ?? [];
  const sameFrameworkAudits = allAudits.filter(
    (a) => framework !== null && a.framework.id === framework.id,
  );

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const text = ev.target?.result;
      if (typeof text !== "string") return;
      const result = parseCSV(text);
      if (typeof result === "string") {
        onCsvErrorChange(result);
        onDraftsChange([]);
      } else {
        onCsvErrorChange(null);
        onDraftsChange(result);
      }
    };
    reader.readAsText(file);
    // reset so the same file can be re-selected after fixing
    e.target.value = "";
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Source cards */}
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 2 }}>
        <SourceCard
          icon={<Library size={22} />}
          title="From Framework Library"
          description="Pick controls from the framework's control library. Definitions are pre-filled."
          selected={source === "template"}
          onClick={() => {
            onSourceChange("template");
            onDraftsChange([]);
          }}
        />
        <SourceCard
          icon={<Copy size={22} />}
          title="Copy from Previous Audit"
          description="Import controls from an existing audit and edit before submitting."
          selected={source === "copy"}
          onClick={() => {
            onSourceChange("copy");
            onCopyAuditIdChange(null);
            onDraftsChange([]);
          }}
        />
        <SourceCard
          icon={<ClipboardList size={22} />}
          title="Start Empty"
          description="Add controls manually - control number, description, and all fields are yours to fill."
          selected={source === "empty"}
          onClick={() => {
            onSourceChange("empty");
            onDraftsChange([]);
          }}
        />
        <SourceCard
          icon={<FileUp size={22} />}
          title="Upload CSV"
          description="Upload a CSV with columns: control_number, description, evidence_requirement."
          selected={source === "csv"}
          onClick={() => {
            onSourceChange("csv");
            onDraftsChange([]);
            onCsvErrorChange(null);
          }}
        />
      </Box>

      {/* Template — framework control library checklist */}
      {source === "template" && (
        <Box>
          {!framework && (
            <Alert severity="warning" sx={{ py: 0.5 }}>
              Select a framework in Step 1 first - controls are specific to each framework.
            </Alert>
          )}
          {framework && fwControlsLoading && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <CircularProgress size={16} />
              <Typography variant="body2" color="text.secondary">
                Loading {framework.name} controls…
              </Typography>
            </Box>
          )}
          {framework && !fwControlsLoading && (fwControlsData ?? []).length === 0 && (
            <Alert severity="info" sx={{ py: 0.5 }}>
              No controls in the {framework.name} library yet - add rows below to get started.
            </Alert>
          )}
          {framework && !fwControlsLoading && (fwControlsData ?? []).length > 0 && (
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
                <Typography variant="subtitle2" fontWeight={600}>
                  {framework.name} - {fwControlsData!.length} controls
                </Typography>
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button
                    size="small"
                    variant="outlined"
                    sx={{ textTransform: "none" }}
                    onClick={() => {
                      const toAdd = (fwControlsData ?? []).filter((fc) => !selectedFwCtlIds.has(fc.id));
                      onDraftsChange([...drafts, ...toAdd.map(createDraftControl)]);
                    }}
                  >
                    Select All
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    sx={{ textTransform: "none" }}
                    onClick={() => onDraftsChange(drafts.filter((d) => !d.frameworkControlId))}
                  >
                    Clear All
                  </Button>
                </Box>
              </Box>
              <Paper variant="outlined" sx={{ borderRadius: 2, maxHeight: 400, overflowY: "auto" }}>
                <Table size="small" stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell padding="checkbox" />
                      <TableCell sx={{ fontWeight: 600, width: 90 }}>Control #</TableCell>
                      <TableCell sx={{ fontWeight: 600 }}>Description</TableCell>
                      <TableCell sx={{ fontWeight: 600, width: 80 }}>Type</TableCell>
                      <TableCell sx={{ fontWeight: 600, width: 80 }}>Scope</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {(fwControlsData ?? []).map((fc) => {
                      const checked = selectedFwCtlIds.has(fc.id);
                      return (
                        <TableRow
                          key={fc.id}
                          hover
                          onClick={() => toggleFwControl(fc)}
                          sx={checked
                            ? {
                                cursor: "pointer",
                                bgcolor: "primary.50",
                                // primary.50 is a fixed light swatch — the default text
                                // color lightens in dark mode, so pair it with a dark
                                // override or the row text disappears against it.
                                "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
                              }
                            : { cursor: "pointer" }}
                        >
                          <TableCell padding="checkbox">
                            <Checkbox checked={checked} size="small" />
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2" fontWeight={600} noWrap>
                              {fc.controlNumber}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2" sx={{ maxWidth: 400 }}>
                              {fc.description}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Chip
                              label={fc.requirementType}
                              size="small"
                              color={fc.requirementType === "OE" ? "warning" : "default"}
                              sx={{ height: 20, fontSize: "0.65rem" }}
                            />
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption" color="text.secondary">
                              {fc.scope === "COMMON" ? "Common" : "Product"}
                            </Typography>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </Paper>
              {selectedFwCtlIds.size > 0 && (
                <Typography variant="caption" color="primary.main" sx={{ mt: 1, display: "block" }}>
                  {selectedFwCtlIds.size} control{selectedFwCtlIds.size !== 1 ? "s" : ""} selected. Assign owners and due dates in the table below.
                </Typography>
              )}
            </Box>
          )}
        </Box>
      )}

      {/* Copy — audit selector */}
      {source === "copy" && (
        <Box>
          <Select
            fullWidth
            displayEmpty
            MenuProps={SELECT_MENU_PROPS}
            inputProps={{ "aria-label": "Source audit" }}
            value={copyAuditId !== null ? String(copyAuditId) : ""}
            onChange={(e) => {
              const v = e.target.value as string;
              onCopyAuditIdChange(v === "" ? null : Number(v));
            }}
            renderValue={(v) =>
              v === ""
                ? <span style={{ opacity: 0.5 }}>Select source audit</span>
                : (sameFrameworkAudits.find((a) => String(a.id) === v)?.name ?? "")
            }
          >
            {sameFrameworkAudits.length === 0 ? (
              <MenuItem disabled value="">
                {framework ? "No previous audits for this framework" : "Select a framework first"}
              </MenuItem>
            ) : (
              sameFrameworkAudits.map((a) => (
                <MenuItem key={a.id} value={String(a.id)}>
                  {a.name}
                </MenuItem>
              ))
            )}
          </Select>
          {copyAuditId && sourceControlsLoading && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 2 }}>
              <CircularProgress size={16} />
              <Typography variant="body2" color="text.secondary">
                Loading controls…
              </Typography>
            </Box>
          )}
        </Box>
      )}

      {/* CSV — file input */}
      {source === "csv" && (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
          <Alert severity="info" sx={{ py: 0.5 }}>
            <strong>Required columns:</strong> control_number, description, evidence_requirement<br />
            <strong>Optional columns:</strong> requirement_type (DESIGN/OE), control_type, scope, due_date<br />
            <strong>OE population columns (optional):</strong> population_description, population_due_date, population_comments<br />
            Process Owner, Auditor POC, and Team must be set manually after upload.
          </Alert>
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv,text/csv"
            style={{ display: "none" }}
            onChange={handleFileChange}
          />
          <Button
            variant="outlined"
            startIcon={<FileUp size={16} />}
            onClick={() => fileInputRef.current?.click()}
            sx={{ textTransform: "none", alignSelf: "flex-start" }}
          >
            Choose CSV File
          </Button>
          {csvError && (
            <Alert severity="error">
              {csvError}
            </Alert>
          )}
        </Box>
      )}

      {/* Editable table — shown for empty source always, for template once a
          framework is selected (so "Add Row" is available even when the
          library is empty), or once drafts are populated (copy/csv). */}
      {(source === "empty" || (source === "template" && framework) || drafts.length > 0)
        && !sourceControlsLoading && !fwControlsLoading && (
          <Box>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 1,
              }}
            >
              <Typography variant="subtitle2" fontWeight={600}>
                Controls ({drafts.length})
              </Typography>
              <Box sx={{ display: "flex", gap: 1 }}>
                {source === "template" && pendingLibraryCount > 0 && (
                  <Button
                    size="small"
                    variant="contained"
                    startIcon={savingAll ? <CircularProgress size={14} color="inherit" /> : <Library size={14} />}
                    onClick={() => void saveAllToLibrary()}
                    disabled={savingAll}
                    sx={{ textTransform: "none" }}
                  >
                    {savingAll ? "Saving…" : `Save ${pendingLibraryCount} to Library`}
                  </Button>
                )}
                {source !== "csv" && (
                  <Button
                    size="small"
                    startIcon={<Plus size={14} />}
                    onClick={() => onDraftsChange([...drafts, blankDraft()])}
                    sx={{ textTransform: "none" }}
                  >
                    Add Row
                  </Button>
                )}
              </Box>
            </Box>
            {source === "template" && saveToLibraryError && (
              <Alert severity="error" sx={{ mb: 1 }} onClose={() => setSaveToLibraryError(null)}>
                {saveToLibraryError}
              </Alert>
            )}
            <EditableControlsTable drafts={drafts} onChange={onDraftsChange} users={users} teams={teams} />
          </Box>
        )}
    </Box>
  );
}

// ── Step 3: Review ────────────────────────────────────────────────────────────

interface Step3Props {
  name: string;
  framework: AuditFramework | null;
  product: AuditProduct | null;
  periodStart: string;
  periodEnd: string;
  scopeDescription: string;
  drafts: DraftControl[];
  users: AuditUser[];
  teams: AuditTeam[];
}

function Step3Review({
  name,
  framework,
  product,
  periodStart,
  periodEnd,
  scopeDescription,
  drafts,
  users,
  teams,
}: Step3Props): JSX.Element {
  const userById = (id: number | null) => users.find((u) => u.id === id);
  const teamById = (id: number | null) => teams.find((t) => t.id === id);
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Paper variant="outlined" sx={{ borderRadius: 2, p: 2.5 }}>
        <Typography variant="subtitle1" fontWeight={700} mb={1.5}>
          Audit Details
        </Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
          <Box>
            <Typography variant="caption" color="text.secondary">
              Name
            </Typography>
            <Typography variant="body2" fontWeight={600}>
              {name}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">
              Framework
            </Typography>
            <Typography variant="body2">
              {framework ? framework.name : "—"}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">
              Product / System
            </Typography>
            <Typography variant="body2">{product?.name ?? "—"}</Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary">
              Audit Period
            </Typography>
            <Typography variant="body2">
              {periodStart} → {periodEnd}
            </Typography>
          </Box>
          {scopeDescription && (
            <Box sx={{ gridColumn: "span 2" }}>
              <Typography variant="caption" color="text.secondary">
                Scope Description
              </Typography>
              <Typography variant="body2">{scopeDescription}</Typography>
            </Box>
          )}
        </Box>
      </Paper>

      <Paper variant="outlined" sx={{ borderRadius: 2, p: 2.5 }}>
        <Typography variant="subtitle1" fontWeight={700} mb={1.5}>
          Controls to add ({drafts.length})
        </Typography>
        {drafts.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No controls — you can add them later via the Control Settings panel.
          </Typography>
        ) : (
          <TableContainer sx={{ maxHeight: 360 }}>
            <Table size="small" stickyHeader>
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Control #</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Description</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Evidence Requirement</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Req. Type</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Population</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Control Type</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Scope</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Process Owner</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Auditor POC</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Team</TableCell>
                  <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Due Date</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {drafts.map((d) => {
                  const owner = userById(d.ownerId);
                  const auditor = userById(d.auditorId);
                  const team = teamById(d.teamId);
                  return (
                    <TableRow key={d.localId}>
                      <TableCell sx={{ whiteSpace: "nowrap" }}>{d.controlNumber || "—"}</TableCell>
                      <TableCell sx={{ minWidth: 180 }}>{d.description || "—"}</TableCell>
                      <TableCell sx={{ minWidth: 160 }}>{d.evidenceRequirement || "—"}</TableCell>
                      <TableCell>{d.requirementType}</TableCell>
                      <TableCell sx={{ minWidth: 160 }}>
                        {d.requirementType === "OE" && d.population ? (
                          <Typography variant="caption" display="block" noWrap title={d.population.description}>
                            {d.population.description || <em style={{ color: "#9e9e9e" }}>Not set</em>}
                          </Typography>
                        ) : (
                          <Typography variant="caption" color="text.disabled">—</Typography>
                        )}
                      </TableCell>
                      <TableCell>{d.controlType}</TableCell>
                      <TableCell sx={{ whiteSpace: "nowrap" }}>
                        {d.scope === "COMMON" ? "Common" : "Product Specific"}
                      </TableCell>
                      <TableCell sx={{ whiteSpace: "nowrap" }}>
                        {owner?.displayName ?? "—"}
                      </TableCell>
                      <TableCell sx={{ whiteSpace: "nowrap" }}>
                        {auditor?.displayName ?? "—"}
                      </TableCell>
                      <TableCell sx={{ whiteSpace: "nowrap" }}>{team?.name ?? "—"}</TableCell>
                      <TableCell sx={{ whiteSpace: "nowrap" }}>{d.dueDate || "—"}</TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Paper>
    </Box>
  );
}

// ── CreateAuditPage ───────────────────────────────────────────────────────────

const STEPS = ["Audit Details", "Add Controls", "Review & Submit"];

export default function CreateAuditPage(): JSX.Element {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const preselectedFrameworkId = searchParams.get("framework") ? Number(searchParams.get("framework")) : null;

  const { data: frameworksData, isLoading: loadingFrameworks, isError: errorFrameworks, refetch: refetchFrameworks } = useGetFrameworks();
  const { data: productsData, isLoading: loadingProducts, isError: errorProducts, refetch: refetchProducts } = useGetProducts();
  const { data: usersData } = useGetUsers();
  const { data: teamsData } = useGetTeams();
  // Names must be unique (the name doubles as the audit's evidence storage
  // folder) — the backend enforces this at submit time, but checking against
  // the already-loaded audit list here catches it on Step 1 instead of after
  // the auditor has filled in the whole wizard.
  const { data: auditsForNameCheck } = useGetAudits();

  const createAudit = useCreateAudit();
  const bulkAdd = useBulkAddControls();

  // Step 1 state
  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  // Tracks whether the auditor has typed into the Audit Name field — once true,
  // the framework/product/period auto-compose effect below stops overwriting it.
  const [nameEdited, setNameEdited] = useState(false);
  const [framework, setFramework] = useState<AuditFramework | null>(null);
  const [product, setProduct] = useState<AuditProduct | null>(null);
  const [periodStart, setPeriodStart] = useState("");
  const [periodEnd, setPeriodEnd] = useState("");
  const [scopeDescription, setScopeDescription] = useState("");

  // Audit Name is also the top-level Azure evidence folder (see
  // Human-Readable-Evidence-Blob-Paths design), so it is pre-filled from
  // "{Framework} {Product} {Year}" as soon as all three are picked — the
  // auditor can still edit it before submitting, and the backend enforces the
  // same path-safe/uniqueness rules regardless of where the value came from.
  useEffect(() => {
    if (nameEdited || !framework || !product || periodStart.length < 4) return;
    const year = periodStart.slice(0, 4);
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setName(`${framework.name} ${product.name} ${year}`);
  }, [framework, product, periodStart, nameEdited]);

  // Step 2 state
  const [source, setSource] = useState<ControlSourceMode>("empty");
  const [copyAuditId, setCopyAuditId] = useState<number | null>(null);
  const [drafts, setDrafts] = useState<DraftControl[]>([]);
  const [csvError, setCsvError] = useState<string | null>(null);

  // Submit state
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [step2Attempted, setStep2Attempted] = useState(false);
  // Holds the audit id after a successful createAudit call so that retrying
  // after a bulkAdd failure skips re-creation and avoids duplicate audits.
  const createdAuditIdRef = useRef<number | null>(null);

  const frameworks = frameworksData ?? [];
  const products = productsData ?? [];

  // Pre-select framework when navigating from a framework card (e.g. ?framework=2)
  useEffect(() => {
    if (preselectedFrameworkId && framework === null && frameworks.length > 0) {
      const fw = frameworks.find((f) => f.id === preselectedFrameworkId);
      // eslint-disable-next-line react-hooks/set-state-in-effect
      if (fw) setFramework(fw);
    }
  }, [frameworks, preselectedFrameworkId, framework]);

  const nameConflict = name.trim().length > 0 && (auditsForNameCheck?.items ?? []).some(
    (a) => a.name.toLowerCase() === name.trim().toLowerCase(),
  );

  const step1Valid =
    name.trim().length > 0 &&
    !nameConflict &&
    framework !== null &&
    product !== null &&
    periodStart.length > 0 &&
    periodEnd.length > 0 &&
    periodEnd >= periodStart;

  // Step 2 → 3: every draft row must be complete (blank rows are not allowed).
  const draftErrors: string[] = drafts
    .flatMap((d) => {
      const errs: string[] = [];
      const label = d.controlNumber.trim() || "(unnamed)";
      // Template-linked drafts skip definition column checks — those come from the library.
      if (!d.frameworkControlId) {
        if (!d.controlNumber.trim())       errs.push(`${label}: Control Number is required`);
        if (!d.description.trim())         errs.push(`${label}: Description is required`);
        if (!d.evidenceRequirement.trim()) errs.push(`${label}: Evidence Requirement is required`);
      }
      if (!d.dueDate)                    errs.push(`${label}: Due Date is required`);
      if (d.requirementType === "OE") {
        if (!d.population?.description.trim()) errs.push(`${label}: Population Requirement is required`);
        if (!d.population?.dueDate)            errs.push(`${label}: Population Due Date is required`);
      }
      return errs;
    });
  const step2Valid = draftErrors.length === 0;

  async function handleSubmit() {
    if (!framework || !product) return;
    setSubmitError(null);

    try {
      // Use the ref so a retry after a failed bulkAdd skips re-creation.
      if (createdAuditIdRef.current === null) {
        const audit = await createAudit.mutateAsync({
          name: name.trim(),
          frameworkId: framework.id,
          productId: product.id,
          periodStart,
          periodEnd,
          scopeDescription: scopeDescription.trim() || null,
        });
        createdAuditIdRef.current = audit.id;
      }

      const auditId = createdAuditIdRef.current;
      const validDrafts = drafts.filter((d) => d.controlNumber.trim() && d.description.trim());
      if (validDrafts.length > 0) {
        await bulkAdd.mutateAsync({
          auditId,
          controls: validDrafts.map(draftToRequest),
        });
      }

      void navigate(`/audit/audits/${auditId}`);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Failed to create audit. Please try again.");
    }
  }

  const isSubmitting = createAudit.isPending || bulkAdd.isPending;

  return (
    <Box sx={{ p: { xs: 2, sm: 3 }, maxWidth: 1500, mx: "auto" }}>
      {/* Back */}
      <Button
        startIcon={<ChevronLeft size={16} />}
        onClick={() => void navigate("/audit/audits")}
        sx={{ mb: 2, textTransform: "none", color: "text.secondary", pl: 0 }}
      >
        Audits
      </Button>

      <Typography variant="h5" fontWeight={700} mb={3}>
        Create New Audit
      </Typography>

      {/* Stepper */}
      <Stepper activeStep={step} sx={{ mb: 4 }}>
        {STEPS.map((label) => (
          <Step key={label}>
            <StepLabel>{label}</StepLabel>
          </Step>
        ))}
      </Stepper>

      {/* Step content */}
      <Paper variant="outlined" sx={{ p: 3, borderRadius: 2, mb: 3 }}>
        {step === 0 && (
          <Step1Form
            name={name}
            nameConflict={nameConflict}
            framework={framework}
            product={product}
            periodStart={periodStart}
            periodEnd={periodEnd}
            scopeDescription={scopeDescription}
            frameworks={frameworks}
            products={products}
            loadingFrameworks={loadingFrameworks}
            loadingProducts={loadingProducts}
            errorFrameworks={errorFrameworks}
            errorProducts={errorProducts}
            onRefetchFrameworks={refetchFrameworks}
            onRefetchProducts={refetchProducts}
            onNameChange={(v) => { setNameEdited(true); setName(v); }}
            onFrameworkChange={(v) => { setFramework(v); setCopyAuditId(null); setDrafts([]); }}
            onProductChange={setProduct}
            onPeriodStartChange={setPeriodStart}
            onPeriodEndChange={setPeriodEnd}
            onScopeDescriptionChange={setScopeDescription}
          />
        )}

        {step === 1 && (
          <Step2Controls
            source={source}
            onSourceChange={setSource}
            drafts={drafts}
            onDraftsChange={setDrafts}
            copyAuditId={copyAuditId}
            onCopyAuditIdChange={setCopyAuditId}
            csvError={csvError}
            onCsvErrorChange={setCsvError}
            framework={framework}
          />
        )}

        {step === 2 && (
          <Step3Review
            name={name}
            framework={framework}
            product={product}
            periodStart={periodStart}
            periodEnd={periodEnd}
            scopeDescription={scopeDescription}
            drafts={drafts}
            users={usersData ?? []}
            teams={teamsData ?? []}
          />
        )}
      </Paper>

      {/* Submit error */}
      {submitError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {submitError}
        </Alert>
      )}

      {/* Step 2 validation errors */}
      {step === 1 && step2Attempted && draftErrors.length > 0 && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          <strong>Fix the following before proceeding:</strong>
          <ul style={{ margin: "4px 0 0", paddingLeft: 20 }}>
            {draftErrors.map((e) => <li key={e}>{e}</li>)}
          </ul>
        </Alert>
      )}

      {/* Navigation */}
      <Divider sx={{ mb: 2 }} />
      <Box sx={{ display: "flex", justifyContent: "space-between" }}>
        <Button
          variant="outlined"
          onClick={() => {
            if (step === 0) { void navigate("/audit/audits"); return; }
            setStep2Attempted(false);
            setStep(step - 1);
          }}
          disabled={isSubmitting}
          sx={{ textTransform: "none" }}
        >
          {step === 0 ? "Cancel" : "Back"}
        </Button>

        {step < 2 ? (
          <Button
            variant="contained"
            onClick={() => {
              if (step === 1) {
                setStep2Attempted(true);
                if (!step2Valid) return;
              }
              setStep(step + 1);
            }}
            disabled={step === 0 && !step1Valid}
            sx={{ textTransform: "none" }}
          >
            Next
          </Button>
        ) : (
          <Button
            variant="contained"
            onClick={() => void handleSubmit()}
            disabled={isSubmitting}
            startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
            sx={{ textTransform: "none" }}
          >
            {isSubmitting ? "Creating…" : "Create Audit"}
          </Button>
        )}
      </Box>
    </Box>
  );
}
