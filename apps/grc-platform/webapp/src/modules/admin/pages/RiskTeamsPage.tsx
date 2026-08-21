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
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil, Plus } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { createTeam, fetchAllTeams, updateTeam, type AdminTeam, type TeamPayload } from "../api/adminApi";
import { dialogPaperSx } from "../cardStyles";

// The DB's team_type enum still has three values (SOURCE_REGISTER, ASSIGNMENT,
// BOTH — see risk_schema.sql), but this console deliberately offers only two:
// real seed data is 7x BOTH and 2x ASSIGNMENT, zero teams have ever been
// SOURCE_REGISTER-only, so a third option nobody uses just invites confusion.
// "Register" here saves as BOTH, not a dedicated REGISTER value on the wire —
// SOURCE_REGISTER stays a valid, reachable enum value everywhere else
// (internal/risk/repository/entity/team.go's semantic filter still expands it),
// it's just not selectable from this form. Revisit if a genuine register-only,
// non-assignable team is ever needed — that's a one-line addition here, not a
// schema change.
const teamTypeOptions: { value: "BOTH" | "ASSIGNMENT"; label: string; hint: string }[] = [
  { value: "BOTH", label: "Register", hint: "Can be used as a risk's source register, and as an assignment target." },
  { value: "ASSIGNMENT", label: "Assignment", hint: "Assignment target only — cannot be a risk's source register." },
];

const teamTypeLabel = (t: AdminTeam["team_type"]): string =>
  t === "BOTH" ? "Register" : t === "ASSIGNMENT" ? "Assignment" : "Source Register";

export default function RiskTeamsPage(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [teams, setTeams] = useState<AdminTeam[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<AdminTeam | null>(null);
  const [name, setName] = useState("");
  const [code, setCode] = useState("");
  const [description, setDescription] = useState("");
  const [teamType, setTeamType] = useState<"BOTH" | "ASSIGNMENT">("BOTH");
  const [status, setStatus] = useState<"ACTIVE" | "INACTIVE">("ACTIVE");
  const [saving, setSaving] = useState(false);
  const [dialogError, setDialogError] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    setError(null);
    fetchAllTeams(authFetch)
      .then(setTeams)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load teams"))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const openAdd = () => {
    setEditing(null);
    setName("");
    setCode("");
    setDescription("");
    setTeamType("BOTH");
    setStatus("ACTIVE");
    setDialogError(null);
    setDialogOpen(true);
  };

  const openEdit = (team: AdminTeam) => {
    setEditing(team);
    setName(team.name);
    setCode(team.code ?? "");
    setDescription(team.description ?? "");
    // A pre-existing SOURCE_REGISTER-only team (none in real data today, but
    // the enum still permits one) has no matching option in this form — fall
    // back to displaying "Register" rather than rendering an unselectable
    // blank value. The Select is locked in this case (see isSourceRegister
    // below) and handleSave sends the real team_type unchanged, so this
    // display-only substitution never reaches the save payload.
    setTeamType(team.team_type === "ASSIGNMENT" ? "ASSIGNMENT" : "BOTH");
    setStatus(team.status === "INACTIVE" ? "INACTIVE" : "ACTIVE");
    setDialogError(null);
    setDialogOpen(true);
  };

  // This form has no way to represent SOURCE_REGISTER as a real selection
  // (see teamTypeOptions above) — so an edit must never let the two-option
  // selector's displayed fallback overwrite a team that's actually
  // SOURCE_REGISTER-only on save.
  const isSourceRegister = editing?.team_type === "SOURCE_REGISTER";
  const codeRequired = teamType === "BOTH" || isSourceRegister;

  const handleSave = async () => {
    if (!name.trim()) {
      setDialogError("Name is required.");
      return;
    }
    if (codeRequired && !code.trim()) {
      setDialogError("Code is required for a Register team.");
      return;
    }
    setSaving(true);
    setDialogError(null);
    try {
      const payload: TeamPayload = {
        name: name.trim(),
        code: code.trim() ? code.trim().toUpperCase() : null,
        description: description.trim(),
        team_type: isSourceRegister ? "SOURCE_REGISTER" : teamType,
        status,
      };
      if (editing) {
        await updateTeam(authFetch, editing.id, payload);
      } else {
        await createTeam(authFetch, payload);
      }
      setDialogOpen(false);
      load();
    } catch (e) {
      setDialogError(e instanceof Error ? e.message : "Failed to save team");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Box>
      <Stack direction="row" justifyContent="flex-end" sx={{ mb: 2 }}>
        <Button variant="contained" startIcon={<Plus size={14} />} onClick={openAdd}>
          Add Team
        </Button>
      </Stack>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 700 }}>Name</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Code</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Type</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
              <TableCell sx={{ fontWeight: 700 }} align="right">
                Actions
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                  <CircularProgress size={22} />
                </TableCell>
              </TableRow>
            )}
            {!loading && teams.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    No teams found.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              teams.map((team) => (
                <TableRow key={team.id} sx={team.status !== "ACTIVE" ? { opacity: 0.55 } : undefined}>
                  <TableCell sx={{ fontWeight: 600 }}>{team.name}</TableCell>
                  <TableCell>
                    {team.code ? (
                      <Chip size="small" label={team.code} variant="outlined" sx={{ fontFamily: "monospace" }} />
                    ) : (
                      <Typography variant="body2" color="text.secondary">
                        —
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>{teamTypeLabel(team.team_type)}</TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      label={team.status === "ACTIVE" ? "Active" : team.status === "INACTIVE" ? "Inactive" : "Removed"}
                      color={team.status === "ACTIVE" ? "success" : "default"}
                    />
                  </TableCell>
                  <TableCell align="right">
                    <IconButton size="small" onClick={() => openEdit(team)} aria-label="Edit">
                      <Pencil size={15} />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        maxWidth="sm"
        fullWidth
        PaperProps={{ sx: dialogPaperSx }}
      >
        <DialogTitle>{editing ? "Edit Risk Team" : "Add Risk Team"}</DialogTitle>
        {/* pt bumped above DialogContent's default — otherwise the first
            field's floating label (autoFocus Name) renders partly clipped
            against the content box's top edge. */}
        <DialogContent sx={{ minHeight: 380, pt: 3 }}>
          {dialogError && (
            <Alert severity="error" sx={{ mb: 2 }} onClose={() => setDialogError(null)}>
              {dialogError}
            </Alert>
          )}
          <TextField
            autoFocus
            fullWidth
            size="small"
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            helperText="The team or product name shown throughout the Risk Hub."
            sx={{ mb: 2.5 }}
          />
          <Stack direction="row" spacing={2} sx={{ mb: 2.5 }}>
            <TextField
              fullWidth
              size="small"
              label={codeRequired ? "Code *" : "Code"}
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              slotProps={{ htmlInput: { maxLength: 10, style: { textTransform: "uppercase" } } }}
              helperText={
                codeRequired
                  ? "Short abbreviation used to build generated risk codes, e.g. CHO."
                  : "Only needed for a Register team — this one doesn't require it."
              }
            />
            <FormControl fullWidth size="small" disabled={isSourceRegister}>
              <InputLabel id="team-type-label">Team Type</InputLabel>
              <Select
                labelId="team-type-label"
                label="Team Type"
                value={teamType}
                onChange={(e) => setTeamType(e.target.value as "BOTH" | "ASSIGNMENT")}
              >
                {teamTypeOptions.map((opt) => (
                  <MenuItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Stack>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: -1.5, mb: 2.5 }}>
            {isSourceRegister
              ? "Source Register-only — this type isn't editable from this console; saving keeps it unchanged."
              : teamTypeOptions.find((o) => o.value === teamType)?.hint}
          </Typography>
          <TextField
            fullWidth
            multiline
            minRows={2}
            size="small"
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            helperText="Shown to admins managing this list — not visible elsewhere in the app."
            sx={{ mb: 2.5 }}
          />
          <FormControl fullWidth size="small">
            <InputLabel id="team-status-label">Status</InputLabel>
            <Select
              labelId="team-status-label"
              label="Status"
              value={status}
              onChange={(e) => setStatus(e.target.value as "ACTIVE" | "INACTIVE")}
            >
              <MenuItem value="ACTIVE">Active</MenuItem>
              <MenuItem value="INACTIVE">Inactive</MenuItem>
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
            An inactive team stops appearing in Risk Hub pickers, but existing risks that reference it are unaffected.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
          <Button variant="contained" disabled={saving} onClick={handleSave}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
