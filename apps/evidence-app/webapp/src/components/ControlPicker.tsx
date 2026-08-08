import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Autocomplete, { createFilterOptions } from "@mui/material/Autocomplete";
import TextField from "@mui/material/TextField";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Stack from "@mui/material/Stack";
import Chip from "@mui/material/Chip";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Alert from "@mui/material/Alert";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { PlusIcon, PenToSquareIcon, TrashIcon } from "@oxygen-ui/react-icons";
import { controlsApi, evidenceApi, submissionsApi, agentApi } from "../api/client";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog";
import { timeAgo } from "../utils/timeAgo";
import { useCurrentUser } from "../hooks/useCurrentUser";

type Control = {
  id: number;
  framework_id: number;
  control_ref: string;
  title: string;
  description?: string | null;
};

type Evidence = { id: number; control_id: number };
type Submission = { id: number; evidence_id: number; status: string };
type AgentTask = {
  status: string;
  control_id: number | null;
  started_at: string | null;
  user_email: string;
};

type ControlOption = Control & { isCreate?: false } | {
  isCreate: true;
  inputValue: string;
  id: -1;
  framework_id: -1;
  control_ref: string;
  title: string;
};

const filter = createFilterOptions<ControlOption>({
  stringify: (o) => ("isCreate" in o && o.isCreate ? "" : `${o.control_ref} ${o.title}`),
});

type Props = {
  frameworkId: number | "";
  controlId: number | "";
  onControlChange: (id: number | "") => void;
  required?: boolean;
  disabled?: boolean;
  label?: string;
  helperText?: string;
};

export default function ControlPicker({
  frameworkId,
  controlId,
  onControlChange,
  required = false,
  disabled = false,
  label = "Control",
  helperText,
}: Props) {
  const queryClient = useQueryClient();
  const { isAdmin } = useCurrentUser();
  const [createOpen, setCreateOpen] = useState(false);
  const [createInitialText, setCreateInitialText] = useState("");
  const [editTarget, setEditTarget] = useState<Control | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Control | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const { data: controls = [], isLoading } = useQuery<Control[]>({
    queryKey: ["controls", frameworkId || undefined],
    queryFn: () => controlsApi.list(frameworkId || undefined),
    enabled: !!frameworkId,
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
  // agent run that's still (or claims to be) in progress against it.
  const { data: allTasks = [], isLoading: isTasksLoading } = useQuery<AgentTask[]>({
    queryKey: ["agent-tasks"],
    queryFn: () => agentApi.listTasks(500),
    enabled: !!deleteTarget,
  });

  const cascadeImpact = (ctrlId: number) => {
    const evIds = allEvidence.filter((e) => e.control_id === ctrlId).map((e) => e.id);
    const subs = allSubmissions.filter((s) => evIds.includes(s.evidence_id));
    const approvedCount = subs.filter((s) => s.status === "approved").length;
    return [
      { label: "evidence records", count: evIds.length },
      { label: "submission records", count: subs.length },
      { label: "approved submissions", count: approvedCount },
    ];
  };

  // status = "running" isn't trustworthy on its own — a crashed Runner leaves
  // that row forever, and there's no heartbeat column. Showing how long ago
  // it started lets the Admin judge that instead of the system claiming it.
  const activeRunWarnings = (ctrlId: number): string[] => {
    const activeRuns = allTasks.filter((t) => t.status === "running" && t.control_id === ctrlId);
    if (activeRuns.length === 0) return [];
    return [
      `${activeRuns.length} agent run${activeRuns.length === 1 ? "" : "s"} marked as in progress against this control.`,
      ...activeRuns.map((t) => `Started ${timeAgo(t.started_at)} by ${t.user_email}.`),
      "If it is still running, deleting now will leave its evidence unlinked.",
    ];
  };

  const deleteMutation = useMutation({
    mutationFn: (id: number) => controlsApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["controls"] });
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      if (deleteTarget && controlId === deleteTarget.id) onControlChange("");
      setDeleteTarget(null);
      setDeleteError(null);
    },
    onError: (err: any) => {
      setDeleteError(err?.response?.data?.detail || "Failed to delete control.");
    },
  });

  const selected: Control | null = controls.find((c) => c.id === controlId) || null;
  const effectivelyDisabled = disabled || !frameworkId;

  return (
    <>
      <Autocomplete<ControlOption, false, false, false>
        value={selected as ControlOption | null}
        onChange={(_, value) => {
          if (value && "isCreate" in value && value.isCreate) {
            setCreateInitialText(value.inputValue);
            setCreateOpen(true);
            return;
          }
          onControlChange(value && "id" in value && value.id !== -1 ? value.id : "");
        }}
        options={controls as ControlOption[]}
        loading={isLoading}
        disabled={effectivelyDisabled}
        filterOptions={(options, params) => {
          const filtered = filter(options, params);
          if (isAdmin && params.inputValue.trim() !== "") {
            const exact = options.some(
              (o) =>
                !("isCreate" in o && o.isCreate) &&
                `${o.control_ref} — ${o.title}`.toLowerCase() ===
                  params.inputValue.trim().toLowerCase()
            );
            if (!exact) {
              filtered.push({
                isCreate: true,
                inputValue: params.inputValue.trim(),
                id: -1,
                framework_id: -1,
                control_ref: "",
                title: params.inputValue.trim(),
              });
            }
          }
          return filtered;
        }}
        getOptionLabel={(option) => {
          if (typeof option === "string") return option;
          if ("isCreate" in option && option.isCreate) return option.inputValue;
          return `${option.control_ref} — ${option.title}`;
        }}
        isOptionEqualToValue={(option, value) =>
          "id" in option && "id" in value && option.id === value.id
        }
        renderOption={(props, option) => {
          if ("isCreate" in option && option.isCreate) {
            return (
              <li {...props} key="__create__">
                <Stack
                  direction="row"
                  spacing={1.25}
                  alignItems="center"
                  sx={{ color: "primary.main", py: 0.5 }}
                >
                  <PlusIcon size={16} />
                  <Typography variant="body2" fontWeight={600}>
                    Create new control: "{option.inputValue}"
                  </Typography>
                </Stack>
              </li>
            );
          }
          return (
            <li {...props} key={option.id}>
              <Stack direction="row" alignItems="center" spacing={1} sx={{ width: "100%", py: 0.4 }}>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Chip
                      label={option.control_ref}
                      size="small"
                      variant="outlined"
                      sx={{ fontWeight: 600, fontFamily: "monospace", height: 22 }}
                    />
                    <Typography variant="body2" fontWeight={500} sx={{ overflow: "hidden", textOverflow: "ellipsis" }}>
                      {option.title}
                    </Typography>
                  </Stack>
                  {option.description && (
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{
                        display: "block",
                        mt: 0.25,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        maxWidth: 460,
                      }}
                    >
                      {option.description}
                    </Typography>
                  )}
                </Box>
                {/* No onMouseDown handler on these two buttons, on purpose.
                    Autocomplete's own listbox needs that event so it can call
                    preventDefault and stop the search box losing focus. Swallow
                    it and the box blurs, the dropdown closes on blur, the button
                    unmounts before mouse-up, and the browser never fires a
                    click at all — the icons look alive but do nothing. Selecting
                    the row is already prevented by stopPropagation on the click
                    below, which is all these handlers ever needed.
                    Product and Framework are unaffected: a Select menu stays
                    open when focus moves inside it. */}
                {isAdmin && (
                  <Tooltip title="Edit">
                    <IconButton
                      size="small"
                      aria-label="Edit control"
                      onClick={(e) => {
                        e.stopPropagation();
                        e.preventDefault();
                        setEditTarget(option as Control);
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
                      aria-label="Delete control"
                      onClick={(e) => {
                        e.stopPropagation();
                        e.preventDefault();
                        setDeleteError(null);
                        setDeleteTarget(option as Control);
                      }}
                    >
                      <TrashIcon size={14} />
                    </IconButton>
                  </Tooltip>
                )}
              </Stack>
            </li>
          );
        }}
        renderInput={(params) => (
          <TextField
            {...params}
            label={label}
            required={required}
            placeholder={
              effectivelyDisabled
                ? "Pick a framework first"
                : isAdmin
                  ? "Type to search or create new..."
                  : "Search controls..."
            }
            helperText={
              helperText ??
              (!effectivelyDisabled &&
                (isAdmin
                  ? "Don't see your control? Just type it — you can create it on the fly."
                  : "Search by control reference or title."))
            }
          />
        )}
        fullWidth
      />

      <ControlFormDialog
        open={createOpen}
        mode="create"
        frameworkId={frameworkId === "" ? 0 : Number(frameworkId)}
        initialText={createInitialText}
        onClose={() => setCreateOpen(false)}
        onSaved={(c) => {
          setCreateOpen(false);
          onControlChange(c.id);
        }}
      />

      <ControlFormDialog
        open={!!editTarget}
        mode="edit"
        frameworkId={editTarget?.framework_id ?? 0}
        control={editTarget ?? undefined}
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
        impactLoading={isEvidenceLoading || isSubmissionsLoading || isTasksLoading}
        entityType="control"
        entityName={
          deleteTarget ? `${deleteTarget.control_ref} — ${deleteTarget.title}` : ""
        }
        impact={deleteTarget ? cascadeImpact(deleteTarget.id) : []}
        warnings={deleteTarget ? activeRunWarnings(deleteTarget.id) : []}
        error={deleteError}
      />
    </>
  );
}

function ControlFormDialog({
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
