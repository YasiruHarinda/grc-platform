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
import FrameworkFormDialog, { type Framework } from "../components/FrameworkFormDialog";
import { computeDeleteImpact } from "../utils/computeDeleteImpact";

// Same minimal shapes ProductPicker reads — kept structural so this page's
// queries satisfy computeDeleteImpact without a cast. Framework itself comes
// from FrameworkFormDialog now (it carries name and description too, which
// this page's Frameworks column needs to render rows); its extra fields
// don't stop it satisfying the narrower shape computeDeleteImpact expects.
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

/** The "pick a parent first" line a column shows when its own parent isn't
 * selected yet. The Frameworks column uses it only until a Product is
 * picked (see #118); the Controls column still shows it unconditionally,
 * since selecting a Framework doesn't fill it in until a later ticket. */
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

  const [selectedFrameworkId, setSelectedFrameworkId] = useState<number | null>(null);
  const [createFrameworkOpen, setCreateFrameworkOpen] = useState(false);
  const [editFrameworkTarget, setEditFrameworkTarget] = useState<Framework | null>(null);
  const [deleteFrameworkTarget, setDeleteFrameworkTarget] = useState<Framework | null>(null);
  const [deleteFrameworkError, setDeleteFrameworkError] = useState<string | null>(null);

  const {
    data: products = [],
    isLoading: isProductsLoading,
    isError: isProductsError,
    refetch: refetchProducts,
  } = useQuery<Product[]>({ queryKey: ["products"], queryFn: productsApi.list });

  // The Frameworks column itself: only the selected product's rows, kept
  // stale the moment a different product is picked because the product id
  // is part of the key — same convention FrameworkPicker uses for its own
  // filtered query.
  const {
    data: frameworks = [],
    isLoading: isFrameworksLoading,
    isError: isFrameworksError,
    refetch: refetchFrameworks,
  } = useQuery<Framework[]>({
    queryKey: ["frameworks", selectedProductId ?? undefined],
    queryFn: () => frameworksApi.list(selectedProductId ?? undefined),
    enabled: selectedProductId !== null,
  });

  // Loaded only while a delete is being considered, same as ProductPicker
  // and FrameworkPicker — so the cascade counts don't cost every page load,
  // only the moment an Admin actually considers deleting something. These
  // are unfiltered on purpose: a Framework's counts must cover ALL its
  // Controls, not just the ones belonging to the currently selected
  // Product, so this is the query the impact calculation uses — never the
  // product-filtered `frameworks` query above, which is for rendering the
  // column only.
  const { data: allFrameworks = [], isLoading: isAllFrameworksLoading } = useQuery<Framework[]>({
    queryKey: ["frameworks"],
    queryFn: () => frameworksApi.list(),
    enabled: !!deleteTarget,
  });
  const anyDeleteTarget = !!deleteTarget || !!deleteFrameworkTarget;
  const { data: allControls = [], isLoading: isControlsLoading } = useQuery<Control[]>({
    queryKey: ["controls"],
    queryFn: () => controlsApi.list(),
    enabled: anyDeleteTarget,
  });
  const { data: allEvidence = [], isLoading: isEvidenceLoading } = useQuery<Evidence[]>({
    queryKey: ["evidence"],
    queryFn: evidenceApi.list,
    enabled: anyDeleteTarget,
  });
  const { data: allSubmissions = [], isLoading: isSubmissionsLoading } = useQuery<Submission[]>({
    queryKey: ["submissions"],
    queryFn: submissionsApi.list,
    enabled: anyDeleteTarget,
  });
  // Only fetched while a delete dialog is open, so we can warn about an
  // agent run that's still (or claims to be) in progress against a control
  // under this product or framework.
  const { data: allTasks = [], isLoading: isTasksLoading } = useQuery<AgentTask[]>({
    queryKey: ["agent-tasks"],
    queryFn: () => agentApi.listTasks(500),
    enabled: anyDeleteTarget,
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
      // that no longer exists. The Framework selection goes with it, since
      // a Framework's Controls column would otherwise still point at a
      // Framework whose Product just disappeared.
      if (deleteTarget && selectedProductId === deleteTarget.id) {
        setSelectedProductId(null);
        setSelectedFrameworkId(null);
      }
      setDeleteTarget(null);
      setDeleteError(null);
    },
    onError: (err: unknown) => {
      const detail = isAxiosError(err) ? (err.response?.data as { detail?: string } | undefined)?.detail : undefined;
      setDeleteError(detail || "Failed to delete product.");
    },
  });

  const deleteFrameworkMutation = useMutation({
    mutationFn: (id: number) => frameworksApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["frameworks"] });
      queryClient.invalidateQueries({ queryKey: ["controls"] });
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      // Clears the Controls column's selection state too, once that column
      // has one — see #118. Today it's static, so there's nothing further
      // to reset.
      if (deleteFrameworkTarget && selectedFrameworkId === deleteFrameworkTarget.id) setSelectedFrameworkId(null);
      setDeleteFrameworkTarget(null);
      setDeleteFrameworkError(null);
    },
    onError: (err: unknown) => {
      const detail = isAxiosError(err) ? (err.response?.data as { detail?: string } | undefined)?.detail : undefined;
      setDeleteFrameworkError(detail || "Failed to delete framework.");
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

  // Framework-level impact never needs the frameworks list — only a
  // Product-level delete rolls frameworks up — so this passes an empty
  // array, same as FrameworkPicker's own delete flow.
  const deleteFrameworkImpact = deleteFrameworkTarget
    ? computeDeleteImpact({
        level: "framework",
        targetId: deleteFrameworkTarget.id,
        frameworks: [],
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
                    onClick={() => {
                      // Switching products clears the Framework selection —
                      // otherwise a Framework from the previous product
                      // would stay marked as selected under the new one.
                      setSelectedProductId(p.id);
                      setSelectedFrameworkId(null);
                    }}
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

        {/* Frameworks — filled once a Product is selected; see #118. */}
        <Paper variant="outlined" sx={{ p: 3, flex: 1, minWidth: 0 }}>
          <ColumnHeader
            title="Frameworks"
            onAdd={() => setCreateFrameworkOpen(true)}
            addDisabled={!selectedProductId}
            addDisabledReason="Pick a product first"
          />
          <Divider sx={{ mb: 2 }} />

          {selectedProductId === null ? (
            <PickParentFirst text="Pick a product first." />
          ) : isFrameworksError ? (
            <Alert
              severity="error"
              action={
                <Button color="inherit" size="small" onClick={() => refetchFrameworks()}>
                  Retry
                </Button>
              }
            >
              Couldn't load frameworks.
            </Alert>
          ) : isFrameworksLoading ? (
            <Box display="flex" justifyContent="center" py={4}>
              <CircularProgress size={28} />
            </Box>
          ) : frameworks.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ textAlign: "center", py: 4 }}>
              No frameworks under this product yet. Add one to get started.
            </Typography>
          ) : (
            <List disablePadding>
              {frameworks.map((f) => {
                const selected = selectedFrameworkId === f.id;
                return (
                  <ListItemButton
                    key={f.id}
                    selected={selected}
                    onClick={() => setSelectedFrameworkId(f.id)}
                    sx={{
                      borderRadius: 1,
                      mb: 0.5,
                      "&.Mui-selected": { bgcolor: "rgba(250,123,63,0.08)" },
                      "&.Mui-selected:hover": { bgcolor: "rgba(250,123,63,0.12)" },
                    }}
                  >
                    <ListItemText
                      primary={f.name}
                      secondary={f.description || undefined}
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
                          aria-label="Edit framework"
                          onClick={(e) => {
                            e.stopPropagation();
                            setEditFrameworkTarget(f);
                          }}
                        >
                          <PenToSquareIcon size={14} />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete">
                        <IconButton
                          size="small"
                          color="error"
                          aria-label="Delete framework"
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteFrameworkError(null);
                            setDeleteFrameworkTarget(f);
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
          isAllFrameworksLoading ||
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

      <FrameworkFormDialog
        open={createFrameworkOpen}
        mode="create"
        productId={selectedProductId ?? 0}
        onClose={() => setCreateFrameworkOpen(false)}
        onSaved={(fw) => {
          setCreateFrameworkOpen(false);
          setSelectedFrameworkId(fw.id);
        }}
      />

      <FrameworkFormDialog
        open={!!editFrameworkTarget}
        mode="edit"
        productId={editFrameworkTarget?.product_id ?? 0}
        framework={editFrameworkTarget ?? undefined}
        onClose={() => setEditFrameworkTarget(null)}
        onSaved={() => setEditFrameworkTarget(null)}
      />

      <ConfirmDeleteDialog
        open={!!deleteFrameworkTarget}
        onClose={() => {
          setDeleteFrameworkTarget(null);
          setDeleteFrameworkError(null);
        }}
        onConfirm={() => deleteFrameworkTarget && deleteFrameworkMutation.mutate(deleteFrameworkTarget.id)}
        isPending={deleteFrameworkMutation.isPending}
        impactLoading={isControlsLoading || isEvidenceLoading || isSubmissionsLoading || isTasksLoading}
        entityType="framework"
        entityName={deleteFrameworkTarget?.name ?? ""}
        impact={deleteFrameworkImpact.impact}
        warnings={deleteFrameworkImpact.warnings}
        error={deleteFrameworkError}
      />
    </Box>
  );
}
