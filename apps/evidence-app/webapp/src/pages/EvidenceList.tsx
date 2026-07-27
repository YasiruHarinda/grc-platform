import { useMemo, useState } from "react";
import { getFileUrl } from "../api/client";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AxiosError } from "axios";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import Paper from "@mui/material/Paper";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Button from "@mui/material/Button";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import Link from "@mui/material/Link";
import Stack from "@mui/material/Stack";
import Chip from "@mui/material/Chip";
import Tooltip from "@mui/material/Tooltip";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import TextField from "@mui/material/TextField";
import Snackbar from "@mui/material/Snackbar";
import Alert from "@mui/material/Alert";
import {
  DocumentIcon,
  TrashIcon,
  BoltIcon,
  CircleUserIcon,
  XMarkIcon,
  ClockAsteriskIcon,
  CircleCheckFilledIcon,
  ArrowUpRightFromSquareIcon,
  DrawingPencilIcon,
} from "@oxygen-ui/react-icons";
import { evidenceApi, frameworksApi, controlsApi, productsApi, submissionsApi } from "../api/client";
import { useCurrentUser } from "../hooks/useCurrentUser";

type Product = { id: number; name: string };
type Framework = { id: number; name: string; product_id: number };
type Control = { id: number; framework_id: number; control_ref: string; title: string };
type EvidenceFile = {
  id: number;
  file_name: string;
  file_url: string;
  subtask?: string | null;
};
type Evidence = {
  id: number;
  title: string;
  description?: string | null;
  file_name: string;
  file_url: string;
  control_id: number;
  created_at: string;
  created_by: string;
  files?: EvidenceFile[];
};
type Submission = {
  id: number;
  evidence_id: number;
  submitted_by: string;
  status: string;
  notes?: string | null;
  submitted_at: string;
};
type SourceFilter = "all" | "ai-agent" | "manual";
type StatusFilter = "all" | "pending" | "approved" | "rejected";

function statusChipProps(status: string) {
  const map = {
    pending: { color: "warning" as const, label: "Pending", Icon: ClockAsteriskIcon },
    approved: { color: "success" as const, label: "Approved", Icon: CircleCheckFilledIcon },
    rejected: { color: "error" as const, label: "Rejected", Icon: XMarkIcon },
  };
  return map[status as keyof typeof map] ?? { color: "default" as const, label: status, Icon: ClockAsteriskIcon };
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { dateStyle: "medium" });
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { timeStyle: "short" });
}

function relativeTime(iso: string): string {
  const date = new Date(iso);
  const diffMs = Date.now() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  const diffHr = Math.floor(diffMs / 3600000);
  const diffDay = Math.floor(diffMs / 86400000);
  if (diffMin < 1) return "Just now";
  if (diffMin < 60) return `${diffMin} min ago`;
  if (diffHr < 24) return `${diffHr} hour${diffHr > 1 ? "s" : ""} ago`;
  if (diffDay === 1) return "Yesterday";
  if (diffDay < 7) return `${diffDay} days ago`;
  if (diffDay < 30) {
    const w = Math.floor(diffDay / 7);
    return `${w} week${w > 1 ? "s" : ""} ago`;
  }
  return date.toLocaleDateString();
}

const STATUS_OPTIONS = ["pending", "approved", "rejected"] as const;

function StatCard({ label, value, accent }: { label: string; value: number; accent?: string }) {
  return (
    <Paper variant="outlined" sx={{ p: 2.25, flex: 1, minWidth: { xs: 140, sm: 160 } }}>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ textTransform: "uppercase", letterSpacing: "0.04em", fontWeight: 600 }}
      >
        {label}
      </Typography>
      <Typography variant="h4" fontWeight={700} sx={{ mt: 0.25, color: accent ?? "text.primary" }}>
        {value}
      </Typography>
    </Paper>
  );
}

export default function EvidenceList() {
  const queryClient = useQueryClient();
  const { user, isAdmin } = useCurrentUser();
  const [productId, setProductId] = useState<number | "">("");
  const [frameworkId, setFrameworkId] = useState<number | "">("");
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  // Gallery modal state
  const [galleryEvidenceId, setGalleryEvidenceId] = useState<number | null>(null);
  const [pendingDeleteFileId, setPendingDeleteFileId] = useState<number | null>(null);
  // Rename dialog state
  const [renameTarget, setRenameTarget] = useState<{ id: number; currentText: string } | null>(null);
  const [renameValue, setRenameValue] = useState("");

  const { data: products = [] } = useQuery<Product[]>({
    queryKey: ["products"],
    queryFn: productsApi.list,
  });
  const { data: allFrameworks = [] } = useQuery<Framework[]>({
    queryKey: ["frameworks"],
    queryFn: () => frameworksApi.list(),
  });
  const { data: allControls = [] } = useQuery<Control[]>({
    queryKey: ["controls"],
    queryFn: () => controlsApi.list(),
  });
  const { data: evidence = [], isLoading } = useQuery<Evidence[]>({
    queryKey: ["evidence"],
    queryFn: evidenceApi.list,
  });
  const { data: submissions = [] } = useQuery<Submission[]>({
    queryKey: ["submissions"],
    queryFn: submissionsApi.list,
  });

  const deleteMutation = useMutation({
    mutationFn: evidenceApi.delete,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      setPendingDeleteId(null);
    },
    onError: (err: AxiosError<{ detail?: string }>) => {
      setActionError(err?.response?.data?.detail || "Failed to delete evidence.");
      setPendingDeleteId(null);
    },
  });

  const renameMutation = useMutation({
    mutationFn: ({ id, description }: { id: number; description: string }) =>
      evidenceApi.rename(id, description),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      setRenameTarget(null);
    },
    onError: (err: AxiosError<{ detail?: string }>) => {
      setActionError(err?.response?.data?.detail || "Failed to rename evidence.");
      setRenameTarget(null);
    },
  });

  const deleteFileMutation = useMutation({
    mutationFn: evidenceApi.deleteFile,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evidence"] });
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
      setPendingDeleteFileId(null);
    },
    onError: (err: AxiosError<{ detail?: string }>) => {
      setActionError(err?.response?.data?.detail || "Failed to delete screenshot.");
      setPendingDeleteFileId(null);
    },
  });

  const updateStatusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: string }) => submissionsApi.updateStatus(id, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["submissions"] });
    },
    onError: (err: AxiosError<{ detail?: string }>) => {
      setActionError(err?.response?.data?.detail || "Failed to update status.");
    },
  });

  const controlById = useMemo(() => {
    const m = new Map<number, Control>();
    allControls.forEach((c) => m.set(c.id, c));
    return m;
  }, [allControls]);

  const frameworkById = useMemo(() => {
    const m = new Map<number, Framework>();
    allFrameworks.forEach((f) => m.set(f.id, f));
    return m;
  }, [allFrameworks]);

  const productById = useMemo(() => {
    const m = new Map<number, Product>();
    products.forEach((p) => m.set(p.id, p));
    return m;
  }, [products]);

  const latestSubmissionByEvidence = useMemo(() => {
    const m = new Map<number, Submission>();
    submissions.forEach((s) => {
      const existing = m.get(s.evidence_id);
      if (!existing || s.id > existing.id) m.set(s.evidence_id, s);
    });
    return m;
  }, [submissions]);

  const visibleFrameworks = useMemo(
    () =>
      productId === ""
        ? allFrameworks
        : allFrameworks.filter((f) => f.product_id === Number(productId)),
    [allFrameworks, productId]
  );

  const enriched = useMemo(() => {
    return evidence.map((e) => {
      const ctrl = controlById.get(e.control_id);
      const fw = ctrl ? frameworkById.get(ctrl.framework_id) : null;
      const product = fw ? productById.get(fw.product_id) : null;
      const isAI = typeof e.title === "string" && e.title.startsWith("AI Agent:");
      const submission = latestSubmissionByEvidence.get(e.id);
      return { ...e, _control: ctrl, _framework: fw, _product: product, _isAI: isAI, _submission: submission };
    });
  }, [evidence, controlById, frameworkById, productById, latestSubmissionByEvidence]);

  const filtered = useMemo(() => {
    return enriched.filter((e) => {
      if (productId !== "" && e._framework?.product_id !== Number(productId)) return false;
      if (frameworkId !== "" && e._framework?.id !== Number(frameworkId)) return false;
      if (sourceFilter === "ai-agent" && !e._isAI) return false;
      if (sourceFilter === "manual" && e._isAI) return false;
      if (statusFilter !== "all" && e._submission?.status !== statusFilter) return false;
      return true;
    }).sort((a, b) => b.id - a.id);
  }, [enriched, productId, frameworkId, sourceFilter, statusFilter]);

  const stats = useMemo(() => {
    const total = enriched.length;
    const ai = enriched.filter((e) => e._isAI).length;
    const manual = total - ai;
    const pending = enriched.filter((e) => e._submission?.status === "pending").length;
    return { total, ai, manual, pending };
  }, [enriched]);

  // Gallery evidence — always derived from live query data so it updates after file deletes
  const galleryEvidence = useMemo(
    () => (galleryEvidenceId != null ? enriched.find((e) => e.id === galleryEvidenceId) ?? null : null),
    [enriched, galleryEvidenceId]
  );
  const galleryFiles: EvidenceFile[] = useMemo(() => {
    if (!galleryEvidence) return [];
    return galleryEvidence.files && galleryEvidence.files.length > 0
      ? galleryEvidence.files
      : [{ id: galleryEvidence.id, file_name: galleryEvidence.file_name, file_url: galleryEvidence.file_url }];
  }, [galleryEvidence]);

  // When an evidence record has no stored EvidenceFile rows, galleryFiles above
  // synthesizes a single entry whose `id` is the *Evidence* id, not a file id.
  // Deleting that would send an Evidence id to /evidence/files/{id}, so the
  // per-file delete control is hidden for the synthesized fallback.
  const galleryHasStoredFiles = !!(galleryEvidence?.files && galleryEvidence.files.length > 0);

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Evidence
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Files captured manually or via the AI agent, linked to compliance controls — with review status and audit notes.
      </Typography>

      <Stack direction="row" spacing={2} sx={{ mb: 3, flexWrap: "wrap", rowGap: 2 }}>
        <StatCard label="Total" value={stats.total} />
        <StatCard label="Pending review" value={stats.pending} accent="warning.main" />
        <StatCard label="AI-generated" value={stats.ai} accent="primary.main" />
        <StatCard label="Manual upload" value={stats.manual} />
      </Stack>

      <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
        <Stack direction={{ xs: "column", md: "row" }} spacing={2} alignItems={{ xs: "stretch", md: "center" }} flexWrap="wrap" rowGap={2}>
          <FormControl size="small" sx={{ minWidth: 200 }}>
            <InputLabel>Product</InputLabel>
            <Select
              label="Product"
              value={productId}
              onChange={(e) => {
                setProductId(e.target.value as number | "");
                setFrameworkId("");
              }}
            >
              <MenuItem value="">All Products</MenuItem>
              {products.map((p) => (
                <MenuItem key={p.id} value={p.id}>{p.name}</MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 200 }} disabled={!visibleFrameworks.length}>
            <InputLabel>Framework</InputLabel>
            <Select
              label="Framework"
              value={frameworkId}
              onChange={(e) => setFrameworkId(e.target.value as number | "")}
            >
              <MenuItem value="">All Frameworks</MenuItem>
              {visibleFrameworks.map((f) => (
                <MenuItem key={f.id} value={f.id}>{f.name}</MenuItem>
              ))}
            </Select>
          </FormControl>
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 0.5, textTransform: "uppercase", letterSpacing: "0.04em", fontWeight: 600 }}>
              Source
            </Typography>
            <ToggleButtonGroup
              value={sourceFilter}
              exclusive
              size="small"
              onChange={(_, v) => v && setSourceFilter(v)}
            >
              <ToggleButton value="all">All</ToggleButton>
              <ToggleButton value="ai-agent">AI Agent</ToggleButton>
              <ToggleButton value="manual">Manual</ToggleButton>
            </ToggleButtonGroup>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 0.5, textTransform: "uppercase", letterSpacing: "0.04em", fontWeight: 600 }}>
              Status
            </Typography>
            <ToggleButtonGroup
              value={statusFilter}
              exclusive
              size="small"
              onChange={(_, v) => v && setStatusFilter(v)}
            >
              <ToggleButton value="all">All</ToggleButton>
              <ToggleButton value="pending">Pending</ToggleButton>
              <ToggleButton value="approved">Approved</ToggleButton>
              <ToggleButton value="rejected">Rejected</ToggleButton>
            </ToggleButtonGroup>
          </Box>
          <Box sx={{ flex: 1 }} />
          <Typography variant="body2" color="text.secondary">
            Showing <strong>{filtered.length}</strong> of {enriched.length}
          </Typography>
        </Stack>
      </Paper>

      {isLoading ? (
        <Box display="flex" justifyContent="center" py={6}>
          <CircularProgress />
        </Box>
      ) : (
        <>
        {/* Desktop table — hidden on mobile */}
        <Box sx={{ display: { xs: "none", md: "block" } }}>
        <TableContainer component={Paper} variant="outlined">
          <Table sx={{ tableLayout: "fixed" }}>
            <TableHead>
              <TableRow>
                <TableCell sx={{ width: "11%", py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }}>Date & Time</TableCell>
                <TableCell sx={{ width: "21%", py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }}>Control</TableCell>
                <TableCell sx={{ width: "11%", py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }}>Screenshots</TableCell>
                <TableCell sx={{ width: "14%", py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }}>Status</TableCell>
                <TableCell sx={{ width: "10%", py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }}>Source</TableCell>
                <TableCell sx={{ width: "24%", py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }}>Task</TableCell>
                <TableCell sx={{ width: "9%",  py: 1.5, px: 2, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.72rem", letterSpacing: "0.04em" }} align="center">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {filtered.map((e) => {
                const displayText = (e.description?.trim() || e.title || "Untitled").replace(/^AI Agent:\s*/, "");
                const isPendingDelete = pendingDeleteId === e.id;
                const files = e.files && e.files.length ? e.files : [{ id: e.id, file_name: e.file_name, file_url: e.file_url }];
                const submission = e._submission;
                const status = statusChipProps(submission?.status ?? "pending");
                const whenIso = submission?.submitted_at ?? e.created_at;
                const clamp2 = {
                  display: "-webkit-box",
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: "vertical" as const,
                  overflow: "hidden",
                };
                return (
                  <TableRow key={e.id} hover sx={{ "& > td": { verticalAlign: "middle", py: 1.25, px: 2 } }}>
                    <TableCell sx={{ whiteSpace: "nowrap" }}>
                      <Stack spacing={0}>
                        <Typography variant="body2" sx={{ fontSize: "0.78rem", fontWeight: 500 }}>
                          {formatDate(whenIso)}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {formatTime(whenIso)}
                        </Typography>
                      </Stack>
                    </TableCell>

                    <TableCell sx={{ overflow: "hidden" }}>
                      {e._control ? (
                        <Stack spacing={0.4}>
                          <Typography variant="caption" sx={{ fontWeight: 700, color: "primary.main" }}>
                            {e._product?.name}{e._product ? " · " : ""}{e._framework?.name ?? "?"}
                          </Typography>
                          <Typography variant="body2" fontWeight={600} sx={{ lineHeight: 1.25 }}>
                            {e._control.control_ref}
                          </Typography>
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{
                              lineHeight: 1.3,
                              display: "-webkit-box",
                              WebkitLineClamp: 2,
                              WebkitBoxOrient: "vertical",
                              overflow: "hidden",
                            }}
                          >
                            {e._control.title}
                          </Typography>
                        </Stack>
                      ) : (
                        <Typography variant="caption" color="text.disabled">—</Typography>
                      )}
                    </TableCell>

                    <TableCell>
                      <Box sx={{ position: "relative", display: "inline-block", cursor: "pointer" }} onClick={() => setGalleryEvidenceId(e.id)}>
                        <Box
                          component="img"
                          src={getFileUrl(files[0].file_url)}
                          alt=""
                          sx={{
                            width: 72,
                            height: 52,
                            objectFit: "cover",
                            borderRadius: 1,
                            border: "1px solid",
                            borderColor: "divider",
                            display: "block",
                            transition: "transform 0.15s ease",
                            "&:hover": { transform: "scale(1.05)", borderColor: "primary.main" },
                          }}
                        />
                        {files.length > 1 && (
                          <Tooltip title={`View all ${files.length} screenshots`}>
                            <IconButton
                              size="small"
                              aria-label="View all screenshots"
                              sx={{
                                position: "absolute",
                                bottom: -6,
                                right: -6,
                                backgroundColor: "background.paper",
                                border: "1px solid",
                                borderColor: "divider",
                                width: 22,
                                height: 22,
                                "&:hover": { backgroundColor: "background.paper" },
                              }}
                              onClick={(ev) => {
                                ev.stopPropagation();
                                setGalleryEvidenceId(e.id);
                              }}
                            >
                              <ArrowUpRightFromSquareIcon size={12} />
                            </IconButton>
                          </Tooltip>
                        )}
                      </Box>
                    </TableCell>

                    <TableCell>
                      <Select
                        size="small"
                        value={submission?.status ?? "pending"}
                        disabled={!submission || updateStatusMutation.isPending}
                        onChange={(ev) =>
                          submission && updateStatusMutation.mutate({ id: submission.id, status: ev.target.value as string })
                        }
                        renderValue={(value) => {
                          const s = statusChipProps(value as string);
                          const Icon = s.Icon;
                          return (
                            <Stack direction="row" spacing={0.5} alignItems="center">
                              <Icon size={14} />
                              <span>{s.label}</span>
                            </Stack>
                          );
                        }}
                        sx={{
                          width: "100%",
                          fontWeight: 600,
                          fontSize: "0.78rem",
                          color: `${status.color}.contrastText`,
                          backgroundColor: `${status.color}.main`,
                          borderRadius: 1,
                          "& .MuiSelect-select": { display: "flex", alignItems: "center", py: 0.5, px: 1 },
                          "& .MuiOutlinedInput-notchedOutline": { border: "none" },
                          "& .MuiSvgIcon-root": { color: `${status.color}.contrastText` },
                        }}
                      >
                        {STATUS_OPTIONS.map((s) => (
                          <MenuItem key={s} value={s}>
                            {s.charAt(0).toUpperCase() + s.slice(1)}
                          </MenuItem>
                        ))}
                      </Select>
                    </TableCell>

                    <TableCell sx={{ overflow: "visible" }}>
                      <Chip
                        icon={e._isAI ? <BoltIcon size={12} /> : <CircleUserIcon size={12} />}
                        label={e._isAI ? "AI Agent" : "Manual"}
                        size="small"
                        color={e._isAI ? "primary" : "default"}
                        variant={e._isAI ? "filled" : "outlined"}
                        sx={{ height: 20, fontSize: "0.65rem", fontWeight: 600, maxWidth: "none", "& .MuiChip-label": { overflow: "visible", whiteSpace: "nowrap" } }}
                      />
                    </TableCell>

                    <TableCell sx={{ overflow: "hidden" }}>
                      <Typography variant="body2" sx={{ lineHeight: 1.35, ...clamp2 }}>
                        {displayText}
                      </Typography>
                    </TableCell>

                    <TableCell align="center">
                      {isPendingDelete ? (
                        <Stack direction="row" spacing={0.5} justifyContent="center">
                          <Button
                            size="small"
                            variant="text"
                            onClick={() => setPendingDeleteId(null)}
                            disabled={deleteMutation.isPending}
                          >
                            Cancel
                          </Button>
                          <Button
                            size="small"
                            color="error"
                            variant="contained"
                            onClick={() => deleteMutation.mutate(e.id)}
                            disabled={deleteMutation.isPending}
                          >
                            Confirm
                          </Button>
                        </Stack>
                      ) : (
                        <Stack direction="row" spacing={0.5} justifyContent="center">
                          <Tooltip title="Rename">
                            <IconButton
                              size="small"
                              aria-label="Rename evidence"
                              onClick={() => {
                                setRenameTarget({ id: e.id, currentText: displayText });
                                setRenameValue(displayText);
                              }}
                            >
                              <DrawingPencilIcon size={16} />
                            </IconButton>
                          </Tooltip>
                          {(isAdmin || e.created_by === user?.email) && (
                            <Tooltip title="Delete evidence">
                              <IconButton
                                size="small"
                                color="error"
                                aria-label="Delete evidence"
                                onClick={() => setPendingDeleteId(e.id)}
                              >
                                <TrashIcon size={16} />
                              </IconButton>
                            </Tooltip>
                          )}
                        </Stack>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7} align="center" sx={{ py: 8 }}>
                    <Stack alignItems="center" spacing={1}>
                      <Box sx={{ color: "text.disabled" }}>
                        <DocumentIcon size={48} />
                      </Box>
                      <Typography color="text.secondary">
                        {enriched.length === 0 ? "No evidence found" : "No evidence matches the current filters"}
                      </Typography>
                      {enriched.length === 0 ? (
                        <Typography variant="caption" color="text.disabled">
                          Upload via Submit or run the AI agent.
                        </Typography>
                      ) : (
                        <Link
                          component="button"
                          type="button"
                          underline="hover"
                          sx={{ fontSize: "0.85rem" }}
                          onClick={() => {
                            setProductId("");
                            setFrameworkId("");
                            setSourceFilter("all");
                            setStatusFilter("all");
                          }}
                        >
                          Clear filters
                        </Link>
                      )}
                    </Stack>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
        </Box>

        {/* Mobile cards — hidden on desktop */}
        <Stack spacing={1.5} sx={{ display: { md: "none" } }}>
          {filtered.length === 0 && (
            <Paper variant="outlined" sx={{ py: 8, textAlign: "center" }}>
              <Stack alignItems="center" spacing={1}>
                <Box sx={{ color: "text.disabled" }}><DocumentIcon size={48} /></Box>
                <Typography color="text.secondary">
                  {enriched.length === 0 ? "No evidence found" : "No evidence matches the current filters"}
                </Typography>
                {enriched.length === 0 ? (
                  <Typography variant="caption" color="text.disabled">Upload via Submit or run the AI agent.</Typography>
                ) : (
                  <Link component="button" type="button" underline="hover" sx={{ fontSize: "0.85rem" }}
                    onClick={() => { setProductId(""); setFrameworkId(""); setSourceFilter("all"); setStatusFilter("all"); }}>
                    Clear filters
                  </Link>
                )}
              </Stack>
            </Paper>
          )}
          {filtered.map((e) => {
            const displayText = (e.description?.trim() || e.title || "Untitled").replace(/^AI Agent:\s*/, "");
            const isPendingDelete = pendingDeleteId === e.id;
            const files = e.files && e.files.length ? e.files : [{ id: e.id, file_name: e.file_name, file_url: e.file_url }];
            const extraCount = files.length - 3;
            return (
              <Paper key={e.id} variant="outlined" sx={{ p: 2 }}>
                <Stack spacing={1}>
                  <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Tooltip title={new Date(e.created_at).toLocaleString()}>
                      <Typography variant="caption" color="text.secondary">{relativeTime(e.created_at)}</Typography>
                    </Tooltip>
                    <Chip
                      icon={e._isAI ? <BoltIcon size={14} /> : <CircleUserIcon size={14} />}
                      label={e._isAI ? "AI Agent" : "Manual"}
                      size="small"
                      color={e._isAI ? "primary" : "default"}
                      variant={e._isAI ? "filled" : "outlined"}
                      sx={{ fontWeight: 600 }}
                    />
                  </Stack>
                  <Typography variant="body2" fontWeight={500} sx={{ lineHeight: 1.35 }}>
                    {displayText}
                  </Typography>
                  <Stack direction="row" spacing={0.75} flexWrap="wrap" rowGap={0.75}>
                    {files.slice(0, 3).map((f, i) => (
                      <Box key={f.id} sx={{ display: "block", lineHeight: 0, position: "relative", cursor: "pointer" }}
                        onClick={() => { if (i === 2 && extraCount > 0) setGalleryEvidenceId(e.id); else window.open(getFileUrl(f.file_url), "_blank", "noreferrer"); }}>
                        <Box component="img" src={getFileUrl(f.file_url)} alt=""
                          sx={{ width: 80, height: 56, objectFit: "cover", borderRadius: 1, border: "1px solid", borderColor: "divider", display: "block" }} />
                        {i === 2 && extraCount > 0 && (
                          <Box sx={{ position: "absolute", inset: 0, borderRadius: 1, backgroundColor: "rgba(0,0,0,0.55)",
                            display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontSize: "0.7rem", fontWeight: 700 }}>
                            +{extraCount}
                          </Box>
                        )}
                      </Box>
                    ))}
                  </Stack>
                  {files.length > 1 && (
                    <Link component="button" type="button" underline="hover"
                      onClick={() => setGalleryEvidenceId(e.id)}
                      sx={{ fontSize: "0.72rem", fontWeight: 600, color: "primary.main", alignSelf: "flex-start", background: "none", border: "none", cursor: "pointer", textAlign: "left" }}>
                      View all {files.length} screenshots →
                    </Link>
                  )}
                  {e._control && (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" rowGap={0.5}>
                      {e._product && (
                        <Chip label={e._product.name} size="small"
                          sx={{ height: 20, fontSize: "0.65rem", fontWeight: 700, backgroundColor: "rgba(255,115,0,0.10)", color: "primary.main", textTransform: "uppercase", letterSpacing: "0.04em" }} />
                      )}
                      <Chip label={`${e._framework?.name ?? "?"} · ${e._control.control_ref}`} size="small" variant="outlined" sx={{ fontWeight: 600 }} />
                    </Stack>
                  )}
                  {isPendingDelete ? (
                    <Stack direction="row" spacing={1}>
                      <Button size="small" variant="text" onClick={() => setPendingDeleteId(null)} disabled={deleteMutation.isPending}>Cancel</Button>
                      <Button size="small" color="error" variant="contained" onClick={() => deleteMutation.mutate(e.id)} disabled={deleteMutation.isPending}>Confirm Delete</Button>
                    </Stack>
                  ) : (
                    <Stack direction="row" spacing={0.5}>
                      <Tooltip title="Rename">
                        <IconButton size="small" aria-label="Rename evidence" onClick={() => { setRenameTarget({ id: e.id, currentText: displayText }); setRenameValue(displayText); }}>
                          <DrawingPencilIcon size={16} />
                        </IconButton>
                      </Tooltip>
                      {(isAdmin || e.created_by === user?.email) && (
                        <Tooltip title="Delete evidence">
                          <IconButton size="small" color="error" aria-label="Delete evidence" onClick={() => setPendingDeleteId(e.id)}>
                            <TrashIcon size={16} />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Stack>
                  )}
                </Stack>
              </Paper>
            );
          })}
        </Stack>
        </>
      )}

      {/* ── Gallery Modal ─────────────────────────────────────────────────── */}
      <Dialog
        open={galleryEvidence != null}
        onClose={() => { setGalleryEvidenceId(null); setPendingDeleteFileId(null); }}
        maxWidth="md"
        fullWidth
        PaperProps={{ sx: { maxHeight: "90vh" } }}
      >
        <DialogTitle sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pb: 1 }}>
          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.3 }}>
              {galleryEvidence
                ? (galleryEvidence.description?.trim() || galleryEvidence.title || "Evidence").replace(/^AI Agent:\s*/, "")
                : ""}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {galleryFiles.length} screenshot{galleryFiles.length !== 1 ? "s" : ""}
            </Typography>
          </Box>
          <IconButton onClick={() => { setGalleryEvidenceId(null); setPendingDeleteFileId(null); }} size="small" aria-label="Close">
            <XMarkIcon size={20} />
          </IconButton>
        </DialogTitle>

        <DialogContent dividers sx={{ p: 2 }}>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2 }}>
            {galleryFiles.map((f, idx) => {
              const isPendingFileDelete = pendingDeleteFileId === f.id;
              return (
                <Box
                  key={f.id}
                  sx={{
                    position: "relative",
                    width: { xs: "calc(50% - 8px)", sm: "calc(33.33% - 11px)" },
                    border: "1px solid",
                    borderColor: isPendingFileDelete ? "error.main" : "divider",
                    borderRadius: 1.5,
                    overflow: "hidden",
                    transition: "border-color 0.15s ease",
                  }}
                >
                  <Link href={getFileUrl(f.file_url)} target="_blank" rel="noreferrer" sx={{ display: "block", lineHeight: 0 }}>
                    <Box
                      component="img"
                      src={getFileUrl(f.file_url)}
                      alt={f.subtask ?? `Screenshot ${idx + 1}`}
                      sx={{
                        width: "100%",
                        aspectRatio: "16/11",
                        objectFit: "cover",
                        display: "block",
                        transition: "opacity 0.15s ease",
                        "&:hover": { opacity: 0.9 },
                      }}
                    />
                  </Link>
                  <Box sx={{ px: 1, py: 0.75, background: "background.paper" }}>
                    <Typography variant="caption" color="text.secondary" sx={{ display: "block", lineHeight: 1.3, fontSize: "0.68rem" }}>
                      {f.subtask ? f.subtask : `Screenshot ${idx + 1}`}
                    </Typography>
                  </Box>
                  <Box sx={{ position: "absolute", top: 4, right: 4 }}>
                    {isPendingFileDelete ? (
                      <Stack direction="row" spacing={0.5}>
                        <Button
                          size="small"
                          variant="contained"
                          color="error"
                          sx={{ minWidth: 0, px: 1, py: 0.25, fontSize: "0.65rem" }}
                          onClick={() => deleteFileMutation.mutate(f.id)}
                          disabled={deleteFileMutation.isPending}
                        >
                          Delete
                        </Button>
                        <Button
                          size="small"
                          variant="contained"
                          sx={{ minWidth: 0, px: 1, py: 0.25, fontSize: "0.65rem", backgroundColor: "rgba(0,0,0,0.5)", "&:hover": { backgroundColor: "rgba(0,0,0,0.7)" } }}
                          onClick={() => setPendingDeleteFileId(null)}
                          disabled={deleteFileMutation.isPending}
                        >
                          Cancel
                        </Button>
                      </Stack>
                    ) : (
                      galleryHasStoredFiles &&
                      (isAdmin || galleryEvidence?.created_by === user?.email) && (
                        <Tooltip title="Delete this screenshot">
                          <IconButton
                            size="small"
                            aria-label="Delete screenshot"
                            onClick={() => setPendingDeleteFileId(f.id)}
                            sx={{ backgroundColor: "rgba(0,0,0,0.5)", color: "#fff", "&:hover": { backgroundColor: "rgba(200,0,0,0.8)" }, width: 28, height: 28 }}
                          >
                            <TrashIcon size={14} />
                          </IconButton>
                        </Tooltip>
                      )
                    )}
                  </Box>
                </Box>
              );
            })}
          </Box>
        </DialogContent>

        <DialogActions sx={{ px: 2.5, py: 1.5 }}>
          <Button onClick={() => { setGalleryEvidenceId(null); setPendingDeleteFileId(null); }}>
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* ── Rename Dialog ─────────────────────────────────────────────────── */}
      <Dialog
        open={renameTarget != null}
        onClose={() => setRenameTarget(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Rename Evidence</DialogTitle>
        <DialogContent sx={{ pt: "12px !important" }}>
          <TextField
            autoFocus
            fullWidth
            label="Name"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && renameValue.trim() && renameTarget) {
                renameMutation.mutate({ id: renameTarget.id, description: renameValue.trim() });
              }
            }}
            size="small"
          />
        </DialogContent>
        <DialogActions sx={{ px: 2.5, py: 1.5 }}>
          <Button onClick={() => setRenameTarget(null)} disabled={renameMutation.isPending}>
            Cancel
          </Button>
          <Button
            variant="contained"
            disabled={!renameValue.trim() || renameMutation.isPending}
            onClick={() => {
              if (renameTarget) {
                renameMutation.mutate({ id: renameTarget.id, description: renameValue.trim() });
              }
            }}
          >
            {renameMutation.isPending ? "Saving…" : "Save"}
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={actionError != null}
        autoHideDuration={6000}
        onClose={() => setActionError(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert onClose={() => setActionError(null)} severity="error" variant="filled" sx={{ width: "100%" }}>
          {actionError}
        </Alert>
      </Snackbar>
    </Box>
  );
}
