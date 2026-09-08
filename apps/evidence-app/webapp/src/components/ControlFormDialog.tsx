import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Box from "@mui/material/Box";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import Alert from "@mui/material/Alert";
import { PlusIcon, PenToSquareIcon } from "@oxygen-ui/react-icons";
import { controlsApi } from "../api/client";

export type Control = {
  id: number;
  framework_id: number;
  control_ref: string;
  title: string;
  description?: string | null;
};

/**
 * The single create/edit form for a Control. Used to live as its own
 * private component inside ControlPicker; pulled out here so the coming
 * Admin page can render the same form instead of growing a second one that
 * drifts from this one — see ticket #116.
 */
export default function ControlFormDialog({
  open,
  mode,
  frameworkId,
  control,
  initialText = "",
  onClose,
  onSaved,
}: {
  open: boolean;
  mode: "create" | "edit";
  frameworkId: number;
  control?: Control;
  initialText?: string;
  onClose: () => void;
  onSaved: (c: Control) => void;
}) {
  const queryClient = useQueryClient();
  const [controlRef, setControlRef] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    if (mode === "edit" && control) {
      setControlRef(control.control_ref);
      setTitle(control.title);
      setDescription(control.description ?? "");
    } else {
      setDescription("");
      const match = initialText.match(/^([^—\-:]+?)\s*[—\-:]\s*(.+)$/);
      if (match) {
        setControlRef(match[1].trim());
        setTitle(match[2].trim());
      } else {
        setControlRef("");
        setTitle(initialText);
      }
    }
  }, [open, mode, control, initialText]);

  const mutation = useMutation({
    mutationFn: (data: { control_ref: string; title: string; description?: string }) =>
      mode === "edit" && control
        ? controlsApi.update(control.id, data)
        : controlsApi.create({ framework_id: frameworkId, ...data }),
    onSuccess: (newControl: Control) => {
      queryClient.invalidateQueries({ queryKey: ["controls"] });
      onSaved(newControl);
    },
    onError: (err: any) => {
      setError(err?.response?.data?.detail || `Failed to ${mode} control. Try again.`);
    },
  });

  const handleSubmit = () => {
    setError(null);
    if (mode === "create" && !frameworkId) {
      setError("Please pick a framework first.");
      return;
    }
    if (!controlRef.trim()) {
      setError("Control reference is required (e.g. CC8.1, Req 9.3).");
      return;
    }
    if (!title.trim()) {
      setError("Title is required.");
      return;
    }
    mutation.mutate({
      control_ref: controlRef.trim(),
      title: title.trim(),
      description: description.trim() || undefined,
    });
  };

  const isEdit = mode === "edit";

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>
        <Stack direction="row" alignItems="center" spacing={1.5}>
          <Box sx={{ color: "primary.main", display: "flex" }}>
            {isEdit ? <PenToSquareIcon size={22} /> : <PlusIcon size={22} />}
          </Box>
          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.2 }}>
              {isEdit ? "Edit Control" : "Add a New Control"}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {isEdit ? "Update the reference, title, or description." : "Belongs to the currently selected framework."}
            </Typography>
          </Box>
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2.25} sx={{ pt: 1 }}>
          <TextField
            label="Control Reference"
            value={controlRef}
            onChange={(e) => setControlRef(e.target.value)}
            placeholder='e.g. "CC8.1", "Req 9.3", "§164.312(a)(1)"'
            required
            fullWidth
            helperText="The official identifier from the standard."
          />

          <TextField
            label="Title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Short human-readable name"
            required
            fullWidth
            helperText="What this control checks for."
          />

          <TextField
            label="Description (optional)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional longer explanation"
            multiline
            rows={2}
            fullWidth
          />

          {error && <Alert severity="error">{error}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 1.75 }}>
        <Button onClick={onClose} disabled={mutation.isPending}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} variant="contained" disabled={mutation.isPending}>
          {mutation.isPending ? (isEdit ? "Saving..." : "Creating...") : isEdit ? "Save Changes" : "Create Control"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
