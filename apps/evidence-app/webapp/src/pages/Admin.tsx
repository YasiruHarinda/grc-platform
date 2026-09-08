import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { isAxiosError } from "axios";
import Box from "@mui/material/Box";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import ListItemText from "@mui/material/ListItemText";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import Button from "@mui/material/Button";
import Alert from "@mui/material/Alert";
import CircularProgress from "@mui/material/CircularProgress";
import { PlusIcon, PenToSquareIcon, TrashIcon } from "@oxygen-ui/react-icons";
import { productsApi, frameworksApi, controlsApi, evidenceApi, submissionsApi, agentApi } from "../api/client";
import ConfirmDeleteDialog from "../components/ConfirmDeleteDialog";
import ProductFormDialog, { type Product } from "../components/ProductFormDialog";
import { computeDeleteImpact } from "../utils/computeDeleteImpact";

// Same minimal shapes ProductPicker reads — kept structural so this page's
// queries satisfy computeDeleteImpact without a cast.
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

/** Heading + Add button shared by all three columns. */
function ColumnHeader({
  title,
  onAdd,
  addDisabled,
  addDisabledReason,
}: {
  title: string;
  onAdd?: () => void;
  addDisabled?: boolean;
  addDisabledReason?: string;
}) {
  const button = (
    <Button size="small" startIcon={<PlusIcon size={16} />} onClick={onAdd} disabled={addDisabled}>
      Add
    </Button>
  );
  return (
    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
      <Typography variant="h6" fontWeight={700}>
        {title}
      </Typography>
      {addDisabled && addDisabledReason ? (
        <Tooltip title={addDisabledReason}>
          <span>{button}</span>
        </Tooltip>
      ) : (
        button
      )}
    </Stack>
  );
}

/** The "pick a parent first" line the Frameworks and Controls columns show
 * whenever nothing is selected — which, in this ticket, is always, since
 * neither column is wired up to a selection yet. See ticket #117; the
 * follow-up tickets make selecting a Product or Framework fill these in. */
function PickParentFirst({ text }: { text: string }) {
  return (
    <Typography variant="body2" color="text.secondary" sx={{ textAlign: "center", py: 4 }}>
      {text}
    </Typography>
  );
}

export default function Admin() {
  const queryClient = useQueryClient();
  const [selectedProductId, setSelectedProductId] = useState<number | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Product | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Product | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const {
    data: products = [],
    isLoading: isProductsLoading,
    isError: isProductsError,
    refetch: refetchProducts,
  } = useQuery<Product[]>({ queryKey: ["products"], queryFn: productsApi.list });

  // Loaded only while the delete dialog is open, same as ProductPicker —
  // so the cascade counts don't cost every page load, only the moment an
  // Admin actually considers deleting something.
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
      // Clear the selection if the deleted product was the selected one, so
      // the columns to the right stop implying they belong to something
      // that no longer exists.
      if (deleteTarget && selectedProductId === deleteTarget.id) setSelectedProductId(null);
      setDeleteTarget(null);
      setDeleteError(null);
    },
    onError: (err: unknown) => {
      const detail = isAxiosError(err) ? (err.response?.data as { detail?: string } | undefined)?.detail : undefined;
      setDeleteError(detail || "Failed to delete product.");
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
    <Box>
      <Typography variant="h4" gutterBottom>
        Admin
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Add, rename and remove the Products, Frameworks and Controls used across the app.
      </Typography>

      <Stack direction={{ xs: "column", md: "row" }} spacing={3}>
        {/* Products */}
        <Paper variant="outlined" sx={{ p: 3, flex: 1, minWidth: 0 }}>
          <ColumnHeader title="Products" onAdd={() => setCreateOpen(true)} />
          <Divider sx={{ mb: 2 }} />

          {isProductsError ? (
            <Alert
              severity="error"
              action={
                <Button color="inherit" size="small" onClick={() => refetchProducts()}>
                  Retry
                </Button>
              }
            >
              Couldn't load products.
            </Alert>
          ) : isProductsLoading ? (
            <Box display="flex" justifyContent="center" py={4}>
              <CircularProgress size={28} />
            </Box>
          ) : products.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ textAlign: "center", py: 4 }}>
              No products yet. Add one to get started.
            </Typography>
          ) : (
            <List disablePadding>
              {products.map((p) => {
                const selected = selectedProductId === p.id;
                return (
                  <ListItemButton
                    key={p.id}
                    selected={selected}
                    onClick={() => setSelectedProductId(p.id)}
                    sx={{
                      borderRadius: 1,
                      mb: 0.5,
                      "&.Mui-selected": { bgcolor: "rgba(250,123,63,0.08)" },
                      "&.Mui-selected:hover": { bgcolor: "rgba(250,123,63,0.12)" },
                    }}
                  >
                    <ListItemText
                      primary={p.name}
                      secondary={p.description || undefined}
                      primaryTypographyProps={{ fontWeight: selected ? 600 : 400, noWrap: true }}
                      secondaryTypographyProps={{ noWrap: true }}
                      sx={{ mr: 1, minWidth: 0 }}
                    />
                    <Stack
                      direction="row"
                      spacing={0.5}
                      sx={{ flexShrink: 0 }}
                      onMouseDown={(e) => e.stopPropagation()}
                    >
                      <Tooltip title="Edit">
                        <IconButton
                          size="small"
                          aria-label="Edit product"
                          onClick={(e) => {
                            e.stopPropagation();
                            setEditTarget(p);
                          }}
                        >
                          <PenToSquareIcon size={14} />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete">
                        <IconButton
                          size="small"
                          color="error"
                          aria-label="Delete product"
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteError(null);
                            setDeleteTarget(p);
                          }}
                        >
                          <TrashIcon size={14} />
                        </IconButton>
                      </Tooltip>
                    </Stack>
                  </ListItemButton>
                );
              })}
            </List>
          )}
        </Paper>

        {/* Frameworks — static in this ticket; a later ticket fills this in
            once a Product is selected. */}
        <Paper variant="outlined" sx={{ p: 3, flex: 1, minWidth: 0 }}>
          <ColumnHeader title="Frameworks" addDisabled addDisabledReason="Pick a product first" />
          <Divider sx={{ mb: 2 }} />
          <PickParentFirst text="Pick a product first." />
        </Paper>

        {/* Controls — static in this ticket; a later ticket fills this in
            once a Framework is selected. */}
        <Paper variant="outlined" sx={{ p: 3, flex: 1, minWidth: 0 }}>
          <ColumnHeader title="Controls" addDisabled addDisabledReason="Pick a framework first" />
          <Divider sx={{ mb: 2 }} />
          <PickParentFirst text="Pick a framework first." />
        </Paper>
      </Stack>

      <ProductFormDialog
        open={createOpen}
        mode="create"
        onClose={() => setCreateOpen(false)}
        onSaved={(p) => {
          setCreateOpen(false);
          setSelectedProductId(p.id);
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
    </Box>
  );
}
