import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import Box from "@mui/material/Box";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";
import Divider from "@mui/material/Divider";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { PlusIcon, PenToSquareIcon, TrashIcon } from "@oxygen-ui/react-icons";
import { frameworksApi, controlsApi, evidenceApi, submissionsApi, agentApi } from "../api/client";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog";
import FrameworkFormDialog, { type Framework } from "./FrameworkFormDialog";
import { computeDeleteImpact } from "../utils/computeDeleteImpact";
import { resolveHoverText } from "../utils/resolveHoverText";
import { useCurrentUser } from "../hooks/useCurrentUser";

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

  // Computed once per render and reused for both the impact list and the
  // warnings passed to the confirm dialog below.
  const deleteImpact = deleteTarget
    ? computeDeleteImpact({
        level: "framework",
        targetId: deleteTarget.id,
        frameworks: [],
        controls: allControls,
        evidence: allEvidence,
        submissions: allSubmissions,
        tasks: allTasks,
      })
    : { impact: [], warnings: [] };

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
                {/* Tooltip wraps this text block only, never the MenuItem
                    itself — Select reads the properties of its own menu
                    children directly, so wrapping a row can break selection. */}
                <Tooltip title={resolveHoverText(f)} placement="bottom-start">
                  <Box sx={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>
                    {f.name}
                  </Box>
                </Tooltip>
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
        impact={deleteImpact.impact}
        warnings={deleteImpact.warnings}
        error={deleteError}
      />
    </>
  );
}
