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
import { frameworksApi } from "../api/client";

export type Framework = { id: number; product_id: number; name: string; description?: string | null };

/**
 * The single create/edit form for a Framework. Used to live as its own
 * private component inside FrameworkPicker; pulled out here so the coming
 * Admin page can render the same form instead of growing a second one that
 * drifts from this one — see ticket #116.
 */
export default function FrameworkFormDialog({
  open,
  mode,
  productId,
  framework,
  onClose,
  onSaved,
}: {
  open: boolean;
  mode: "create" | "edit";
  productId: number;
  framework?: Framework;
  onClose: () => void;
  onSaved: (fw: Framework) => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setName(framework?.name ?? "");
    setDescription(framework?.description ?? "");
    setError(null);
  }, [open, framework]);

  const mutation = useMutation({
    mutationFn: (data: { name: string; description?: string }) =>
      mode === "edit" && framework
        ? frameworksApi.update(framework.id, data)
        : frameworksApi.create({ product_id: productId, ...data }),
    onSuccess: (fw: Framework) => {
      queryClient.invalidateQueries({ queryKey: ["frameworks"] });
      onSaved(fw);
    },
    onError: (err: any) => {
      setError(err?.response?.data?.detail || `Failed to ${mode} framework.`);
    },
  });

  const handleSubmit = () => {
    setError(null);
    if (mode === "create" && !productId) {
      setError("Pick a product first before adding a framework.");
      return;
    }
    if (!name.trim()) {
      setError("Framework name is required.");
      return;
    }
    mutation.mutate({
      name: name.trim(),
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
              {isEdit ? "Edit Framework" : "Add a New Framework"}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Under the currently selected product.
            </Typography>
          </Box>
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2.25} sx={{ pt: 1 }}>
          <TextField
            label="Framework Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder='e.g. "SOC2", "PCI-DSS", "ISO 27001"'
            required
            fullWidth
            autoFocus
          />
          <TextField
            label="Description (optional)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What this framework covers."
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
          {mutation.isPending ? (isEdit ? "Saving..." : "Creating...") : isEdit ? "Save Changes" : "Create Framework"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
