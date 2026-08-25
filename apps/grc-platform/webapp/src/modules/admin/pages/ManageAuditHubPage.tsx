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
import {
  createAuditTeam,
  fetchAllAuditTeams,
  updateAuditTeam,
  type AdminAuditTeam,
  type AuditTeamPayload,
} from "../api/adminApi";
import { dialogPaperSx } from "../cardStyles";

// Audit Teams only, for now — frameworks/products stay inline in the Create
// Audit flow, same stubbed-for-later scope this page started with. audit_team
// itself is simpler than risk_team (just name + status, no type/code), so
// this form is a smaller version of RiskTeamsPage's, not a different shape.
export default function ManageAuditHubPage(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [teams, setTeams] = useState<AdminAuditTeam[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<AdminAuditTeam | null>(null);
  const [name, setName] = useState("");
  const [status, setStatus] = useState<"ACTIVE" | "INACTIVE">("ACTIVE");
  const [saving, setSaving] = useState(false);
  const [dialogError, setDialogError] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    setError(null);
    fetchAllAuditTeams(authFetch)
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
    setStatus("ACTIVE");
    setDialogError(null);
    setDialogOpen(true);
  };

  const openEdit = (team: AdminAuditTeam) => {
    setEditing(team);
    setName(team.name);
    setStatus(team.status);
    setDialogError(null);
    setDialogOpen(true);
  };

  const handleSave = async () => {
    if (!name.trim()) {
      setDialogError("Name is required.");
      return;
    }
    setSaving(true);
    setDialogError(null);
    try {
      const payload: AuditTeamPayload = { name: name.trim(), status };
      if (editing) {
        await updateAuditTeam(authFetch, editing.id, payload);
      } else {
        await createAuditTeam(authFetch, payload);
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
      <Stack direction="row" justifyContent="space-between" alignItems="flex-end" sx={{ mb: 2 }}>
        <Box>
          <Typography variant="h4" fontWeight={700} sx={{ mb: 0.5 }}>
            Manage Audit Hub
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Audit teams - who a control's evidence gets assigned to, and what an Internal Team or Management grant
            can be scoped to.
          </Typography>
        </Box>
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
              <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
              <TableCell sx={{ fontWeight: 700 }} align="right">
                Actions
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && (
              <TableRow>
                <TableCell colSpan={3} align="center" sx={{ py: 4 }}>
                  <CircularProgress size={22} />
                </TableCell>
              </TableRow>
            )}
            {!loading && teams.length === 0 && (
              <TableRow>
                <TableCell colSpan={3} align="center" sx={{ py: 4 }}>
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
                    <Chip
                      size="small"
                      label={team.status === "ACTIVE" ? "Active" : "Inactive"}
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
        <DialogTitle>{editing ? "Edit Audit Team" : "Add Audit Team"}</DialogTitle>
        <DialogContent sx={{ minHeight: 180, pt: 3 }}>
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
            helperText="The team name shown throughout the Audit Hub."
            sx={{ mb: 2.5 }}
          />
          <FormControl fullWidth size="small">
            <InputLabel id="audit-team-status-label">Status</InputLabel>
            <Select
              labelId="audit-team-status-label"
              label="Status"
              value={status}
              onChange={(e) => setStatus(e.target.value as "ACTIVE" | "INACTIVE")}
            >
              <MenuItem value="ACTIVE">Active</MenuItem>
              <MenuItem value="INACTIVE">Inactive</MenuItem>
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
             An inactive team stops appearing in Audit Hub pickers, but existing controls that reference it are
            unaffected.
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
