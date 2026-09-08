import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { isAxiosError } from "axios";
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
import { productsApi } from "../api/client";

export type Product = { id: number; name: string; description?: string | null };

/**
 * The single create/edit form for a Product. Used to live as its own
 * private component inside ProductPicker; pulled out here so the coming
 * Admin page can render the same form instead of growing a second one that
 * drifts from this one — see ticket #116.
 */
export default function ProductFormDialog({
  open,
  mode,
  product,
  onClose,
  onSaved,
}: {
  open: boolean;
  mode: "create" | "edit";
  product?: Product;
  onClose: () => void;
  onSaved: (p: Product) => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setName(product?.name ?? "");
    setDescription(product?.description ?? "");
    setError(null);
  }, [open, product]);

  const mutation = useMutation({
    mutationFn: (data: { name: string; description?: string }) =>
      mode === "edit" && product
        ? productsApi.update(product.id, data)
        : productsApi.create(data),
    onSuccess: (p: Product) => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      onSaved(p);
    },
    onError: (err: unknown) => {
      const detail = isAxiosError(err) ? (err.response?.data as { detail?: string } | undefined)?.detail : undefined;
      setError(detail || `Failed to ${mode} product.`);
    },
  });

  const handleSubmit = () => {
    setError(null);
    if (!name.trim()) {
      setError("Product name is required.");
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
              {isEdit ? "Edit Product" : "Add a New Product"}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              A product groups its own set of compliance frameworks.
            </Typography>
          </Box>
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2.25} sx={{ pt: 1 }}>
          <TextField
            label="Product Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder='e.g. "WSO2 Identity Server", "Asgardeo", "Choreo"'
            required
            fullWidth
            autoFocus
          />
          <TextField
            label="Description (optional)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Short description of what this product is."
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
          {mutation.isPending ? (isEdit ? "Saving..." : "Creating...") : isEdit ? "Save Changes" : "Create Product"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
