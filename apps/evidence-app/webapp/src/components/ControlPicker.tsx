import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Autocomplete, { createFilterOptions } from "@mui/material/Autocomplete";
import TextField from "@mui/material/TextField";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Stack from "@mui/material/Stack";
import Chip from "@mui/material/Chip";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { PlusIcon, PenToSquareIcon, TrashIcon } from "@oxygen-ui/react-icons";
import { controlsApi, evidenceApi, submissionsApi, agentApi } from "../api/client";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog";
import ControlFormDialog, { type Control } from "./ControlFormDialog";
import { computeDeleteImpact } from "../utils/computeDeleteImpact";
import { resolveHoverText } from "../utils/resolveHoverText";
import { useCurrentUser } from "../hooks/useCurrentUser";

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

  // The hover box content for a Control, shared by the option rows below and
  // by the field itself once a Control is selected. Two lines, not one
  // joined with a separator: the spec (#129) is explicit that no wording is
  // invented around the record's own values.
  const hoverContent = (control: Control) => (
    <Stack spacing={0.25} sx={{ py: 0.25 }}>
      <Typography variant="caption" fontWeight={600}>
        {control.control_ref}
      </Typography>
      <Typography variant="caption">{resolveHoverText(control)}</Typography>
    </Stack>
  );

  // Computed once per render and reused for both the impact list and the
  // warnings passed to the confirm dialog below.
  const deleteImpact = deleteTarget
    ? computeDeleteImpact({
        level: "control",
        targetId: deleteTarget.id,
        frameworks: [],
        controls: [],
        evidence: allEvidence,
        submissions: allSubmissions,
        tasks: allTasks,
      })
    : { impact: [], warnings: [] };

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
                {/* Tooltip wraps this text block only, never the <li> row
                    itself — see the comment on the Edit and Delete buttons
                    below for why the row can't take a handler like this. */}
                <Tooltip title={hoverContent(option as Control)} placement="bottom-start">
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
                </Tooltip>
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
        renderInput={(params) => {
          const field = (
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
                    ? "Don't see your control? Just type it. You can create it on the fly."
                    : "Search by control reference or title."))
              }
            />
          );
          // Only wrapped once a Control is picked, so the closed field can
          // still be read on hover. This isn't a listbox row, so wrapping it
          // doesn't risk the mouse down problem noted above.
          return selected ? <Tooltip title={hoverContent(selected)}>{field}</Tooltip> : field;
        }}
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
        impact={deleteImpact.impact}
        warnings={deleteImpact.warnings}
        error={deleteError}
      />
    </>
  );
}
