import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
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
import Divider from "@mui/material/Divider";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { PlusIcon, PenToSquareIcon, TrashIcon } from "@oxygen-ui/react-icons";
import { frameworksApi, controlsApi, evidenceApi, submissionsApi, agentApi } from "../api/client";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog";
import { timeAgo } from "../utils/timeAgo";
import { useCurrentUser } from "../hooks/useCurrentUser";

type Framework = { id: number; product_id: number; name: string; description?: string | null };
type Control = { id: number; framework_id: number };
type Evidence = { id: number; control_id: number };
type Submission = { id: number; evidence_id: number; status: string };
type AgentTask = {
  status: string;
  control_id: number | null;
  started_at: string | null;
  user_email: string;
};

type Props = {
  productId: number | "";
  value: number | "";
  onChange: (id: number | "") => void;
  label?: string;
  required?: boolean;
  disabled?: boolean;
  placeholderOption?: string;
  helperText?: string;
};

const SENTINEL_CREATE = -2;

export default function FrameworkPicker({
  productId,
  value,
  onChange,
  label = "Framework",
  required = false,
  disabled = false,
  placeholderOption,
  helperText,
}: Props) {
  const queryClient = useQueryClient();
  const { isAdmin } = useCurrentUser();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Framework | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Framework | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const { data: frameworks = [] } = useQuery<Framework[]>({
    queryKey: ["frameworks", productId || undefined],
    queryFn: () => frameworksApi.list(productId ? Number(productId) : undefined),
    enabled: !!productId,
  });

  // Lazy-loaded for cascade-impact display
  const { data: allControls = [], isLoading: isControlsLoading } = useQuery<Control[]>({
    queryKey: ["controls"],
    queryFn: () => controlsApi.list(),
    enabled: !!deleteTarget,
  });
  const { data: allEvidence = [], isLoading: isEvidenceLoading } = useQuery<Evidence[]>({
    queryKey: ["evidence"],
    queryFn: evidenceApi.list,
    enabled: !!deleteTarget,
  });
  const { data: allSubmissions = [], isLoading: isSubmissionsLoading } = useQuery<Submission[]>({
    queryKey: ["submissions"],
    queryFn: submissionsApi.list,
    enabled: !!deleteTarget,
  });
  // Only fetched while the delete dialog is open, so we can warn about an
  // agent run that's still (or claims to be) in progress against a control
  // under this framework.
  const { data: allTasks = [], isLoading: isTasksLoading } = useQuery<AgentTask[]>({
    queryKey: ["agent-tasks"],
    queryFn: () => agentApi.listTasks(500),
    enabled: !!deleteTarget,
  });

  // ctrlIds covers every control under this framework, direct children only
  // (a framework has no grandchildren) — reused below for both the cascade
  // counts and the running-task warning, so the tree is only walked once.
  const cascadeImpact = (ctrlIds: number[]) => {
    const evIds = allEvidence.filter((e) => ctrlIds.includes(e.control_id)).map((e) => e.id);
    const subs = allSubmissions.filter((s) => evIds.includes(s.evidence_id));
    const approvedCount = subs.filter((s) => s.status === "approved").length;
    return [
      { label: "controls", count: ctrlIds.length },
      { label: "evidence records", count: evIds.length },
      { label: "submission records", count: subs.length },
      { label: "approved submissions", count: approvedCount },
    ];
  };

  // status = "running" isn't trustworthy on its own — a crashed Runner leaves
  // that row forever, and there's no heartbeat column. Showing how long ago
  // it started lets the Admin judge that instead of the system claiming it.
  const activeRunWarnings = (ctrlIds: number[]): string[] => {
    const activeRuns = allTasks.filter(
      (t) => t.status === "running" && t.control_id !== null && ctrlIds.includes(t.control_id)
    );
    if (activeRuns.length === 0) return [];
    return [
      `${activeRuns.length} agent run${activeRuns.length === 1 ? "" : "s"} marked as in progress against controls in this framework.`,
      ...activeRuns.map((t) => `Started ${timeAgo(t.started_at)} by ${t.user_email}.`),
      "If it is still running, deleting now will leave its evidence unlinked.",
    ];
  };

  const deleteMutation = useMutation({
    mutationFn: (id: number) => frameworksApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["frameworks"] });
      queryClient.invalidateQueries({ queryKey: ["controls"] });
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      if (deleteTarget && value === deleteTarget.id) onChange("");
      setDeleteTarget(null);
      setDeleteError(null);
    },
    onError: (err: any) => {
      setDeleteError(err?.response?.data?.detail || "Failed to delete framework.");
    },
  });

  const effectivelyDisabled = disabled || !productId;

  // Computed once per render and reused for both the cascade counts and the
  // running-task warning below.
  const deleteCtrlIds = deleteTarget
    ? allControls.filter((c) => c.framework_id === deleteTarget.id).map((c) => c.id)
    : [];

  return (
    <>
      <FormControl fullWidth required={required} disabled={effectivelyDisabled}>
        <InputLabel>{label}</InputLabel>
        <Select
          label={label}
          value={value}
          onChange={(e) => {
            const v = e.target.value;
            if (v === SENTINEL_CREATE) {
              setCreateOpen(true);
              return;
            }
            onChange((v === "" ? "" : Number(v)) as number | "");
          }}
        >
          {placeholderOption !== undefined && (
            <MenuItem value="">
              <em>{placeholderOption}</em>
            </MenuItem>
          )}
          {frameworks.map((f) => (
            <MenuItem key={f.id} value={f.id} sx={{ pr: 1 }}>
              <Stack direction="row" alignItems="center" spacing={1} sx={{ width: "100%" }}>
                <Box sx={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>
                  {f.name}
                </Box>
                {isAdmin && (
                  <Tooltip title="Edit">
                    <IconButton
                      size="small"
                      aria-label="Edit framework"
                      onMouseDown={(e) => e.stopPropagation()}
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditTarget(f);
                      }}
                    >
                      <PenToSquareIcon size={14} />
                    </IconButton>
                  </Tooltip>
                )}
                {isAdmin && (
                  <Tooltip title="Delete">
                    <IconButton
                      size="small"
                      color="error"
                      aria-label="Delete framework"
                      onMouseDown={(e) => e.stopPropagation()}
                      onClick={(e) => {
                        e.stopPropagation();
                        setDeleteError(null);
                        setDeleteTarget(f);
                      }}
                    >
                      <TrashIcon size={14} />
                    </IconButton>
                  </Tooltip>
                )}
              </Stack>
            </MenuItem>
          ))}
          {isAdmin && productId !== "" && [
            <Divider key="div" />,
            <MenuItem key="create" value={SENTINEL_CREATE} sx={{ color: "primary.main" }}>
              <Stack direction="row" spacing={1} alignItems="center">
                <PlusIcon size={16} />
                <Typography variant="body2" fontWeight={600}>
                  Add new framework...
                </Typography>
              </Stack>
            </MenuItem>,
          ]}
        </Select>
        {helperText && (
          <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, ml: 1.5 }}>
            {helperText}
          </Typography>
        )}
      </FormControl>

      <FrameworkFormDialog
        open={createOpen}
        mode="create"
        productId={productId === "" ? 0 : Number(productId)}
        onClose={() => setCreateOpen(false)}
        onSaved={(fw) => {
          setCreateOpen(false);
          onChange(fw.id);
        }}
      />

      <FrameworkFormDialog
        open={!!editTarget}
        mode="edit"
        productId={editTarget?.product_id ?? 0}
        framework={editTarget ?? undefined}
        onClose={() => setEditTarget(null)}
        onSaved={() => setEditTarget(null)}
      />

      <ConfirmDeleteDialog
        open={!!deleteTarget}
        onClose={() => {
          setDeleteTarget(null);
          setDeleteError(null);
        }}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        isPending={deleteMutation.isPending}
        impactLoading={
          isControlsLoading || isEvidenceLoading || isSubmissionsLoading || isTasksLoading
        }
        entityType="framework"
        entityName={deleteTarget?.name ?? ""}
        impact={deleteTarget ? cascadeImpact(deleteCtrlIds) : []}
        warnings={deleteTarget ? activeRunWarnings(deleteCtrlIds) : []}
        error={deleteError}
      />
    </>
  );
}

function FrameworkFormDialog({
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
