import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AxiosError } from "axios";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Alert from "@mui/material/Alert";
import { PlusIcon, CircleCheckFilledIcon } from "@oxygen-ui/react-icons";
import { evidenceApi } from "../api/client";
import ControlPicker from "../components/ControlPicker";
import ProductPicker from "../components/ProductPicker";
import FrameworkPicker from "../components/FrameworkPicker";

export default function SubmitEvidence() {
  const queryClient = useQueryClient();
  const [productId, setProductId] = useState<number | "">("");
  const [frameworkId, setFrameworkId] = useState<number | "">("");
  const [controlId, setControlId] = useState<number | "">("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [success, setSuccess] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: evidenceApi.create,
    onSuccess: () => {
      setSuccess(true);
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      setTitle("");
      setDescription("");
      setFile(null);
      setProductId("");
      setFrameworkId("");
      setControlId("");
    },
  });

  const uploadError = mutation.isError
    ? (mutation.error as AxiosError<{ detail?: string }>)?.response?.data?.detail ||
      "Upload failed. Please check the file and try again."
    : null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!file) {
      setValidationError("Please choose a file to upload.");
      return;
    }
    if (!controlId) {
      setValidationError("Please select a control.");
      return;
    }
    setValidationError(null);
    setSuccess(false);
    const formData = new FormData();
    formData.append("title", title);
    formData.append("description", description);
    formData.append("control_id", String(controlId));
    formData.append("file", file);
    mutation.mutate(formData);
  };

  return (
    <Box sx={{ maxWidth: 640, mx: "auto" }}>
      <Box sx={{ textAlign: "center", mb: 4 }}>
        <Typography variant="h4" gutterBottom>
          Submit Evidence
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Upload a file and link it to a compliance control.
        </Typography>
      </Box>

      {success && (
        <Alert
          severity="success"
          icon={<CircleCheckFilledIcon size={18} />}
          sx={{ mb: 3 }}
        >
          Evidence submitted successfully.
        </Alert>
      )}

      {uploadError && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {uploadError}
        </Alert>
      )}

      {validationError && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {validationError}
        </Alert>
      )}

      <Paper variant="outlined" sx={{ p: { xs: 3, sm: 4 } }}>
        <Box component="form" onSubmit={handleSubmit}>
          <Stack spacing={2.5}>
            <ProductPicker
              value={productId}
              onChange={(id) => {
                setProductId(id);
                setFrameworkId("");
                setControlId("");
              }}
              required
              helperText="Pick the product this evidence belongs to."
            />

            <FrameworkPicker
              productId={productId}
              value={frameworkId}
              onChange={(id) => {
                setFrameworkId(id);
                setControlId("");
              }}
              required
            />

            <ControlPicker
              frameworkId={frameworkId}
              controlId={controlId}
              onControlChange={(id) => setControlId(id)}
              required
            />

            <TextField
              label="Title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Evidence title"
              required
              fullWidth
            />

            <TextField
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
              multiline
              rows={3}
              fullWidth
            />

            <Box>
              <Typography variant="caption" color="text.secondary" display="block" mb={0.75} fontWeight={600}>
                FILE *
              </Typography>
              <Button
                component="label"
                variant="outlined"
                fullWidth
                startIcon={<PlusIcon size={18} />}
                sx={{
                  py: 1.75,
                  borderStyle: "dashed",
                  borderColor: "divider",
                  color: file ? "text.primary" : "text.secondary",
                  justifyContent: "flex-start",
                  px: 2,
                  "&:hover": { borderStyle: "dashed", borderColor: "primary.main", backgroundColor: "rgba(255,115,0,0.04)" },
                }}
              >
                {file ? file.name : "Click to select a file"}
                <input
                  type="file"
                  hidden
                  onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                />
              </Button>
            </Box>

            <Button
              type="submit"
              variant="contained"
              disabled={mutation.isPending}
              size="large"
              sx={{ mt: 1, py: 1.25 }}
            >
              {mutation.isPending ? "Uploading..." : "Submit Evidence"}
            </Button>
          </Stack>
        </Box>
      </Paper>
    </Box>
  );
}
