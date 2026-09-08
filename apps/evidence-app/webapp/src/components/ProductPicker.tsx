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
import { productsApi, frameworksApi, controlsApi, evidenceApi, submissionsApi, agentApi } from "../api/client";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog";
import ProductFormDialog, { type Product } from "./ProductFormDialog";
import { computeDeleteImpact } from "../utils/computeDeleteImpact";
import { useCurrentUser } from "../hooks/useCurrentUser";

type Framework = { id: number; product_id: number };
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
  value: number | "";
  onChange: (id: number | "") => void;
  label?: string;
  required?: boolean;
  disabled?: boolean;
  includeAll?: boolean;
  allLabel?: string;
  helperText?: string;
  size?: "small" | "medium";
  fullWidth?: boolean;
};

const SENTINEL_CREATE = -2;

export default function ProductPicker({
  value,
  onChange,
  label = "Product",
  required = false,
  disabled = false,
  includeAll = false,
  allLabel = "All Products",
  helperText,
  size = "medium",
  fullWidth = true,
}: Props) {
  const queryClient = useQueryClient();
  const { isAdmin } = useCurrentUser();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Product | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Product | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const { data: products = [] } = useQuery<Product[]>({
    queryKey: ["products"],
    queryFn: productsApi.list,
  });

  // Load dependent data only when delete is being considered, so we can show
  // accurate cascade impact in the confirm dialog.
  const { data: allFrameworks = [], isLoading: isFrameworksLoading } = useQuery<Framework[]>({
    queryKey: ["frameworks"],
    queryFn: () => frameworksApi.list(),
    enabled: !!deleteTarget,
  });
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
  // under this product.
  const { data: allTasks = [], isLoading: isTasksLoading } = useQuery<AgentTask[]>({
    queryKey: ["agent-tasks"],
    queryFn: () => agentApi.listTasks(500),
    enabled: !!deleteTarget,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => productsApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["frameworks"] });
      queryClient.invalidateQueries({ queryKey: ["controls"] });
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      if (deleteTarget && value === deleteTarget.id) onChange("");
      setDeleteTarget(null);
      setDeleteError(null);
    },
    onError: (err: any) => {
      setDeleteError(err?.response?.data?.detail || "Failed to delete product.");
    },
  });

  // Computed once per render and reused for both the impact list and the
  // warnings passed to the confirm dialog below.
  const deleteImpact = deleteTarget
    ? computeDeleteImpact({
        level: "product",
        targetId: deleteTarget.id,
        frameworks: allFrameworks,
        controls: allControls,
        evidence: allEvidence,
        submissions: allSubmissions,
        tasks: allTasks,
      })
    : { impact: [], warnings: [] };

  return (
    <>
      <FormControl fullWidth={fullWidth} required={required} disabled={disabled} size={size}>
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
          {includeAll && (
            <MenuItem value="">
              <em>{allLabel}</em>
            </MenuItem>
          )}
          {products.map((p) => (
            <MenuItem key={p.id} value={p.id} sx={{ pr: 1 }}>
              <Stack
                direction="row"
                alignItems="center"
                spacing={1}
                sx={{ width: "100%" }}
              >
                <Box sx={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>
                  {p.name}
                </Box>
                {isAdmin && (
                  <Tooltip title="Edit">
                    <IconButton
                      size="small"
                      aria-label="Edit product"
                      onMouseDown={(e) => e.stopPropagation()}
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditTarget(p);
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
                      aria-label="Delete product"
                      onMouseDown={(e) => e.stopPropagation()}
                      onClick={(e) => {
                        e.stopPropagation();
                        setDeleteError(null);
                        setDeleteTarget(p);
                      }}
                    >
                      <TrashIcon size={14} />
                    </IconButton>
                  </Tooltip>
                )}
              </Stack>
            </MenuItem>
          ))}
          {isAdmin && [
            <Divider key="div" />,
            <MenuItem key="create" value={SENTINEL_CREATE} sx={{ color: "primary.main" }}>
              <Stack direction="row" spacing={1} alignItems="center">
                <PlusIcon size={16} />
                <Typography variant="body2" fontWeight={600}>
                  Add new product...
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

      <ProductFormDialog
        open={createOpen}
        mode="create"
        onClose={() => setCreateOpen(false)}
        onSaved={(p) => {
          setCreateOpen(false);
          onChange(p.id);
        }}
      />

      <ProductFormDialog
        open={!!editTarget}
        mode="edit"
        product={editTarget ?? undefined}
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
          isFrameworksLoading ||
          isControlsLoading ||
          isEvidenceLoading ||
          isSubmissionsLoading ||
          isTasksLoading
        }
        entityType="product"
        entityName={deleteTarget?.name ?? ""}
        impact={deleteImpact.impact}
        warnings={deleteImpact.warnings}
        error={deleteError}
      />
    </>
  );
}
