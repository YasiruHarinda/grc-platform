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
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Paper,
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
import { Pencil, Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { dialogPaperSx } from "../cardStyles";

interface ReferenceRow {
  id: number;
  name: string;
  description: string | null;
}

interface ReferencePayload {
  name: string;
  description: string;
}

interface SimpleReferenceCrudPageProps {
  addLabel: string;
  itemLabel: string;
  emptyLabel: string;
  nameHint: string;
  descriptionHint: string;
  fetchAll: (authFetch: ReturnType<typeof useAuthApiClient>) => Promise<ReferenceRow[]>;
  create: (authFetch: ReturnType<typeof useAuthApiClient>, payload: ReferencePayload) => Promise<ReferenceRow>;
  update: (authFetch: ReturnType<typeof useAuthApiClient>, id: number, payload: ReferencePayload) => Promise<void>;
  // Rejected (409) server-side when the row is still referenced by a risk —
  // neither table has ON DELETE RESTRICT, so that check happens in the
  // backend/entity, not here. The dialog just surfaces whatever message
  // comes back.
  del: (authFetch: ReturnType<typeof useAuthApiClient>, id: number) => Promise<void>;
}

// Shared list+dialog CRUD shape for Risk Categories and Compliance
// References — both are the same {name, description} table with no status
// column (neither is soft-deletable the way teams/users are), so this is one
// component parameterised by API functions rather than two near-identical
// copies of the same table/dialog markup.
export default function SimpleReferenceCrudPage({
  addLabel,
  itemLabel,
  emptyLabel,
  nameHint,
  descriptionHint,
  fetchAll,
  create,
  update,
  del,
}: SimpleReferenceCrudPageProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [rows, setRows] = useState<ReferenceRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ReferenceRow | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);
  const [dialogError, setDialogError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ReferenceRow | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    setError(null);
    fetchAll(authFetch)
      .then(setRows)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const openAdd = () => {
    setEditing(null);
    setName("");
    setDescription("");
    setDialogError(null);
    setDialogOpen(true);
  };

  const openEdit = (row: ReferenceRow) => {
    setEditing(row);
    setName(row.name);
    setDescription(row.description ?? "");
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
      const payload = { name: name.trim(), description: description.trim() };
      if (editing) {
        await update(authFetch, editing.id, payload);
      } else {
        await create(authFetch, payload);
      }
      setDialogOpen(false);
      load();
    } catch (e) {
      setDialogError(e instanceof Error ? e.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await del(authFetch, deleteTarget.id);
      setDeleteTarget(null);
      load();
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : "Failed to delete");
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Box>
      <Stack direction="row" justifyContent="flex-end" sx={{ mb: 2 }}>
        <Button variant="contained" startIcon={<Plus size={14} />} onClick={openAdd}>
          {addLabel}
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
              <TableCell sx={{ fontWeight: 700 }}>Description</TableCell>
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
            {!loading && rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={3} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    {emptyLabel}
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              rows.map((row) => (
                <TableRow key={row.id}>
                  <TableCell sx={{ fontWeight: 600 }}>{row.name}</TableCell>
                  <TableCell sx={{ color: "text.secondary" }}>{row.description || "—"}</TableCell>
                  <TableCell align="right">
                    <IconButton size="small" onClick={() => openEdit(row)} aria-label="Edit">
                      <Pencil size={15} />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => {
                        setDeleteError(null);
                        setDeleteTarget(row);
                      }}
                      aria-label="Delete"
                    >
                      <Trash2 size={15} />
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
        <DialogTitle>{editing ? "Edit" : addLabel}</DialogTitle>
        {/* minHeight for breathing room, pt bumped above the default —
            otherwise the first field's floating label (autoFocus Name)
            renders partly clipped against the content box's top edge. */}
        <DialogContent sx={{ minHeight: 300, pt: 3 }}>
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
            helperText={nameHint}
            sx={{ mb: 2.5 }}
          />
          <TextField
            fullWidth
            multiline
            minRows={2}
            size="small"
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            helperText={descriptionHint}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
          <Button variant="contained" disabled={saving} onClick={handleSave}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={!!deleteTarget}
        onClose={() => (deleting ? undefined : setDeleteTarget(null))}
        maxWidth="xs"
        fullWidth
        PaperProps={{ sx: dialogPaperSx }}
      >
        <DialogTitle>Delete {itemLabel}?</DialogTitle>
        <DialogContent>
          {deleteError && (
            <Alert severity="error" sx={{ mb: 2 }} onClose={() => setDeleteError(null)}>
              {deleteError}
            </Alert>
          )}
          <Typography variant="body2">
            This permanently removes <b>{deleteTarget?.name}</b>. This can't be undone. If it's still tagged on any
            risk, deletion is refused.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)} disabled={deleting}>
            Cancel
          </Button>
          <Button variant="contained" color="error" disabled={deleting} onClick={handleDelete}>
            {deleting ? "Deleting…" : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
