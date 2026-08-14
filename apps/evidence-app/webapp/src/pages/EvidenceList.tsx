import { useMemo, useState, useEffect, useDeferredValue } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { getFileUrl } from "../api/client";
import { stableFileUrl, forgetFileUrl, fileUrlKey } from "../utils/stableFileUrl";
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
import LinearProgress from "@mui/material/LinearProgress";
import Skeleton from "@mui/material/Skeleton";
import Pagination from "@mui/material/Pagination";
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
import useMediaQuery from "@mui/material/useMediaQuery";
import { useTheme } from "@mui/material/styles";
import type { SxProps, Theme } from "@mui/material/styles";
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
  DownloadIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
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

// How many rows are drawn at once. Every row is roughly 35 MUI components,
// and React builds all of them synchronously before the browser is allowed
// to paint anything -- which is why drawing the whole list froze the page,
// down to the sidebar button not even showing its pressed state.
const ROWS_PER_PAGE = 25;

// A single shared empty array for useDeferredValue's initial value. It has to
// keep the same identity across renders: a fresh [] each time would never
// compare equal, so the "still drawing" check below would never turn off.
const NO_ROWS: never[] = [];

const STATUS_OPTIONS = ["pending", "approved", "rejected"] as const;

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "avif"]);

// Identity for the per-file bookkeeping below, keyed on the blob path rather
// than on `id`. Both list views and `galleryFiles` synthesize a fallback entry
// for evidence with no stored EvidenceFile rows, and that entry's `id` is the
// *Evidence* id, so two different tables' ids end up in one set: once
// EvidenceFile 7's link expires, a legacy Evidence with id 7 reads as already
// retried and settles into a document card instead of refetching. A blob path
// cannot collide that way. This is the same reasoning `stableFileUrl` uses for
// its own cache. The id fallback only applies to a URL that will not parse, no
// worse than keying on the id throughout as this used to.
function fileKey(file: { id: number; file_url: string }): string {
  return fileUrlKey(getFileUrl(file.file_url)) ?? `id:${file.id}`;
}

function isImageFile(fileName: string): boolean {
  const ext = fileName.split(".").pop()?.toLowerCase();
  return !!ext && IMAGE_EXTENSIONS.has(ext);
}

// Enter and Space fire a click on a real <button>. The tiles below cannot be
// buttons: each already contains its own button or download link, and nesting
// interactive elements inside a button is invalid HTML. So they keep their
// markup and are given the keyboard behaviour a button would have brought.
function activateOnKey(activate: () => void) {
  return (event: ReactKeyboardEvent) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      activate();
    }
  };
}

// Fallback tile for PDFs, other non-image files, and images whose retried
// load still failed. Fills the same footprint as the <img> it replaces so
// the grid layout does not shift.
function FileFallbackCard({
  fileName,
  fileUrl,
  variant,
}: {
  fileName: string;
  fileUrl: string;
  variant: "grid" | "preview" | "tile";
}) {
  // The list thumbnails are 72x52 and 80x56. The file name and the download
  // button used below would spill straight out of one, so a tile gets the
  // icon alone -- the tile itself already opens the file when clicked, and
  // the name is there on hover.
  if (variant === "tile") {
    return (
      <Box
        title={fileName}
        sx={{
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          backgroundColor: "action.hover",
          color: "text.secondary",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
        }}
      >
        <DocumentIcon size={20} />
      </Box>
    );
  }

  const isPreview = variant === "preview";
  return (
    <Box
      sx={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: isPreview ? 1.5 : 0.75,
        backgroundColor: "action.hover",
        color: "text.secondary",
        textAlign: "center",
        p: isPreview ? 4 : 1.5,
      }}
    >
      <DocumentIcon size={isPreview ? 48 : 28} />
      <Typography variant={isPreview ? "body2" : "caption"} sx={{ wordBreak: "break-word", lineHeight: 1.3, px: 1 }}>
        {fileName}
      </Typography>
      <Tooltip title="Download">
        <IconButton
          component="a"
          href={fileUrl}
          target="_blank"
          rel="noreferrer"
          size="small"
          aria-label={`Download ${fileName}`}
          onClick={(e) => e.stopPropagation()}
        >
          <DownloadIcon size={16} />
        </IconButton>
      </Tooltip>
    </Box>
  );
}

// Shared by the real desktop table and its loading skeleton, so the two
// can never drift out of sync on column widths -- the skeleton's whole job
// is to promise a shape that the real table then delivers on.
function EvidenceTableHead() {
  const headerSx = {
    py: 1.5,
    px: 2,
    fontWeight: 600,
    color: "text.secondary",
    textTransform: "uppercase" as const,
    fontSize: "0.72rem",
    letterSpacing: "0.04em",
  };
  return (
    <TableHead>
      <TableRow>
        <TableCell sx={{ width: "11%", ...headerSx }}>Date & Time</TableCell>
        <TableCell sx={{ width: "21%", ...headerSx }}>Control</TableCell>
        <TableCell sx={{ width: "11%", ...headerSx }}>Screenshots</TableCell>
        <TableCell sx={{ width: "14%", ...headerSx }}>Status</TableCell>
        <TableCell sx={{ width: "10%", ...headerSx }}>Source</TableCell>
        <TableCell sx={{ width: "24%", ...headerSx }}>Task</TableCell>
        <TableCell sx={{ width: "9%", ...headerSx }} align="center">Actions</TableCell>
      </TableRow>
    </TableHead>
  );
}

// A thumbnail that fades in over a neutral placeholder once it has actually
// downloaded, instead of popping in abruptly or leaving a blank tile that
// shifts the grid once it resolves. The "loaded" flag needs a component of
// its own -- useState can't be called from inside the .map() that renders
// each row's thumbnails, only from a real component.
//
// `loading="lazy"` and `decoding="async"` live here too, since every
// call site wants both: lazy so a thumbnail below the fold doesn't compete
// for one of the browser's ~6 connections-per-host with the ones on screen,
// async so decoding it doesn't block the row it just appeared in.
function Thumbnail({
  src,
  alt,
  onError,
  onLoad,
  sx,
}: {
  src: string;
  alt: string;
  onError?: () => void;
  onLoad?: () => void;
  sx?: SxProps<Theme>;
}) {
  const [loaded, setLoaded] = useState(false);
  return (
    <Box
      sx={{
        width: "100%",
        height: "100%",
        overflow: "hidden",
        borderRadius: 1,
        backgroundColor: "action.hover",
      }}
    >
      <Box
        component="img"
        src={src}
        alt={alt}
        loading="lazy"
        decoding="async"
        onLoad={() => {
          setLoaded(true);
          onLoad?.();
        }}
        onError={onError}
        sx={[
          {
            width: "100%",
            height: "100%",
            objectFit: "cover",
            display: "block",
            opacity: loaded ? 1 : 0,
            transition: "opacity 0.2s ease, transform 0.15s ease",
          },
          ...(Array.isArray(sx) ? sx : [sx]),
        ]}
      />
    </Box>
  );
}

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

// Its own component, holding its own `value` state, so a keystroke re-renders
// one text field instead of the whole table -- before this split, `renameValue`
// lived in `EvidenceList` itself and every row, `<Select>` and `<Tooltip>` on
// the page re-rendered on every character typed.
function RenameEvidenceDialog({
  target,
  isSaving,
  onClose,
  onSave,
}: {
  target: { id: number; currentText: string } | null;
  isSaving: boolean;
  onClose: () => void;
  onSave: (id: number, description: string) => void;
}) {
  const [value, setValue] = useState(target?.currentText ?? "");

  // Load the target row's current text into the field whenever a different
  // (or no) row becomes the rename target. Adjusted during render rather
  // than in an effect, matching the pattern the gallery already uses for
  // its own "reset when the identity changes" state further down this file
  // -- it takes effect before the first paint of the new target instead of
  // flashing the previous row's text for one frame.
  const [prevTargetId, setPrevTargetId] = useState(target?.id ?? null);
  if ((target?.id ?? null) !== prevTargetId) {
    setPrevTargetId(target?.id ?? null);
    setValue(target?.currentText ?? "");
  }

  const trimmed = value.trim();
  const save = () => {
    if (target && trimmed) onSave(target.id, trimmed);
  };

  return (
    <Dialog open={target != null} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Rename Evidence</DialogTitle>
      <DialogContent sx={{ pt: "12px !important" }}>
        <TextField
          autoFocus
          fullWidth
          label="Name"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && trimmed) save();
          }}
          size="small"
        />
      </DialogContent>
      <DialogActions sx={{ px: 2.5, py: 1.5 }}>
        <Button onClick={onClose} disabled={isSaving}>
          Cancel
        </Button>
        <Button variant="contained" disabled={!trimmed || isSaving} onClick={save}>
          {isSaving ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

export default function EvidenceList() {
  const queryClient = useQueryClient();
  const { user, isAdmin } = useCurrentUser();
  const theme = useTheme();
  // `noSsr: true` matters here: without it this hook returns `false` on the
  // very first render (there's no way to know the viewport before paint),
  // so a phone would briefly mount the desktop table and download every
  // image in it -- exactly what mounting only one layout is meant to avoid.
  const isMobile = useMediaQuery(theme.breakpoints.down("md"), { noSsr: true });
  const [productId, setProductId] = useState<number | "">("");
  const [frameworkId, setFrameworkId] = useState<number | "">("");
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  // Evidence ids with a zip download in flight -- per-row so one slow
  // build doesn't disable every row's button, but a row can't be clicked
  // twice while its own zip is still being built.
  const [downloadingIds, setDownloadingIds] = useState<Set<number>>(new Set());

  // Gallery modal state
  const [galleryEvidenceId, setGalleryEvidenceId] = useState<number | null>(null);
  const [pendingDeleteFileId, setPendingDeleteFileId] = useState<number | null>(null);
  // Full-size preview state — null means the dialog shows the grid
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  // File ids that have already had one "expired link" retry, and file ids
  // whose image failed to load even after that retry (shown as a card).
  const [retriedFileKeys, setRetriedFileKeys] = useState<Set<string>>(new Set());
  const [failedFileKeys, setFailedFileKeys] = useState<Set<string>>(new Set());
  // Rename dialog state -- just which row, and whether a save is in flight.
  // The text itself lives in RenameEvidenceDialog now, so a keystroke there
  // no longer re-renders this component (and everything under it).
  const [renameTarget, setRenameTarget] = useState<{ id: number; currentText: string } | null>(null);

  // staleTime: 60_000 on each of these five, and only here -- not on the
  // QueryClient in main.tsx, which would also change Dashboard, Agent, Cost
  // and all three pickers. Per-observer staleTime means visiting Evidence,
  // leaving, and coming back within a minute costs nothing: no refetch, no
  // re-signed URLs, no re-downloaded screenshots. refetchOnWindowFocus is
  // left at its default (true) deliberately -- see EvidenceList's stableFileUrl
  // usage below for why a focus refetch is now cheap rather than something
  // to avoid.
  const { data: products = [] } = useQuery<Product[]>({
    queryKey: ["products"],
    queryFn: productsApi.list,
    staleTime: 60_000,
  });
  const { data: allFrameworks = [] } = useQuery<Framework[]>({
    queryKey: ["frameworks"],
    queryFn: () => frameworksApi.list(),
    staleTime: 60_000,
  });
  const { data: allControls = [] } = useQuery<Control[]>({
    queryKey: ["controls"],
    queryFn: () => controlsApi.list(),
    staleTime: 60_000,
  });
  const { data: evidence = [], isLoading, isFetching } = useQuery<Evidence[]>({
    queryKey: ["evidence"],
    queryFn: evidenceApi.list,
    staleTime: 60_000,
  });
  const { data: submissions = [] } = useQuery<Submission[]>({
    queryKey: ["submissions"],
    queryFn: submissionsApi.list,
    staleTime: 60_000,
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
      evidenceApi.rename(id, description) as Promise<Evidence>,
    onSuccess: (updated, variables) => {
      // Write the PATCH's own response straight into the cache instead of
      // invalidating. The PATCH (backend/app/api/routes/evidence.py) already
      // returns the full updated row -- refetching to get it back would
      // re-sign, and so re-download, every screenshot on the page just to
      // change one word of text. Only `description` is copied: the response
      // also carries freshly re-signed `file_url`/`files`, and adopting
      // those would rotate this row's links and undo exactly the caching
      // change stableFileUrl makes. Dashboard.tsx reads this same cache
      // entry, so it picks up the new name too.
      queryClient.setQueryData<Evidence[]>(["evidence"], (prev) =>
        prev?.map((row) => (row.id === variables.id ? { ...row, description: updated.description } : row))
      );
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

  // Not a useMutation: react-query's mutation state is shared across every
  // call site, so a second row's click would show as "pending" on the
  // first row too. downloadingIds (above) is the per-row equivalent.
  async function handleDownload(id: number) {
    if (downloadingIds.has(id)) return;
    setDownloadingIds((prev) => new Set(prev).add(id));
    try {
      const response = await evidenceApi.download(id);
      const disposition = response.headers["content-disposition"] as string | undefined;
      const filenameMatch = disposition?.match(/filename="?([^";]+)"?/);
      const filename = filenameMatch?.[1] || `evidence-${id}.zip`;

      const url = URL.createObjectURL(response.data as Blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch {
      setActionError("Failed to download evidence.");
    } finally {
      setDownloadingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }

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

  // ── One page at a time, drawn in two steps ────────────────────────────
  const [page, setPage] = useState(0);

  // Changing a filter returns the reader to the first page, rather than
  // leaving them on a page number the new result set may not reach.
  // Adjusted during render -- the same pattern the gallery uses further
  // down -- so it takes effect before the first paint rather than causing
  // an extra one.
  const filterSignature = `${productId}|${frameworkId}|${sourceFilter}|${statusFilter}`;
  const [prevFilterSignature, setPrevFilterSignature] = useState(filterSignature);
  if (filterSignature !== prevFilterSignature) {
    setPrevFilterSignature(filterSignature);
    setPage(0);
  }

  const pageCount = Math.max(1, Math.ceil(filtered.length / ROWS_PER_PAGE));
  // Clamped rather than corrected in an effect: deleting the last row on the
  // last page shrinks the list out from under this page number, and clamping
  // lands on the new last page instead of an empty one.
  const safePage = Math.min(page, pageCount - 1);
  const pageStart = safePage * ROWS_PER_PAGE;
  const pageRows = useMemo(
    () => filtered.slice(pageStart, pageStart + ROWS_PER_PAGE),
    [filtered, pageStart]
  );

  // Draw the page in two steps. The second argument matters more than it
  // looks: without it useDeferredValue does not defer on a mount at all,
  // and a mount is exactly this case, since navigating here builds the page
  // from scratch every time. With it, the first render uses NO_ROWS, so the
  // heading, stat cards and filters paint straight away above skeleton
  // rows, and React fills the real rows in afterwards at a priority it is
  // free to interrupt. The total work is the same; what changes is that the
  // browser is no longer blocked from painting while it happens.
  const deferredRows = useDeferredValue(pageRows, NO_ROWS);
  const isDrawingRows = deferredRows !== pageRows;

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

  // Reset to the grid whenever a different (or no) evidence is opened.
  // Adjusted during render (React's documented pattern for resetting state
  // when a value changes) rather than in an effect, so it takes effect
  // before the first paint of the new gallery instead of causing an extra one.
  const [prevGalleryEvidenceId, setPrevGalleryEvidenceId] = useState(galleryEvidenceId);
  if (galleryEvidenceId !== prevGalleryEvidenceId) {
    setPrevGalleryEvidenceId(galleryEvidenceId);
    setPreviewIndex(null);
  }

  // The displayed preview index is clamped against the live file count
  // rather than corrected via an effect. This means a delete needs no
  // special-casing at all: removing a file that isn't last leaves the raw
  // index unchanged, which lands on the file that shifted into its place —
  // i.e. "advance to next" — and removing the last file clamps the display
  // back to the new last file. Emptying the array clamps to null, which is
  // exactly what shows the grid again.
  const clampedPreviewIndex =
    previewIndex != null && galleryFiles.length > 0 ? Math.min(previewIndex, galleryFiles.length - 1) : null;
  const previewFile = clampedPreviewIndex != null ? galleryFiles[clampedPreviewIndex] ?? null : null;

  // Left/right arrow keys move through the preview. Escape is handled by
  // the Dialog's onClose (below) so it can distinguish "close preview" from
  // "close dialog".
  useEffect(() => {
    if (clampedPreviewIndex == null) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft") {
        if (clampedPreviewIndex > 0) setPreviewIndex(clampedPreviewIndex - 1);
      } else if (event.key === "ArrowRight") {
        if (clampedPreviewIndex < galleryFiles.length - 1) setPreviewIndex(clampedPreviewIndex + 1);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [clampedPreviewIndex, galleryFiles.length]);

  // An <img> failed to load — most likely its signed URL expired mid-review.
  // Refetch the evidence list once to get fresh URLs; retry at most once per
  // file so a genuinely missing blob settles into a card instead of looping.
  const handleImageLoadError = (key: string, fileUrl: string) => {
    if (failedFileKeys.has(key)) return;
    if (retriedFileKeys.has(key)) {
      setFailedFileKeys((prev) => new Set(prev).add(key));
      return;
    }
    setRetriedFileKeys((prev) => new Set(prev).add(key));
    // Drop this blob's cached URL *before* the refetch below lands, so the
    // stable-url cache accepts the freshly re-signed link the refetch
    // brings back instead of handing the same (broken) URL straight out
    // again on the next render.
    forgetFileUrl(getFileUrl(fileUrl));
    queryClient.invalidateQueries({ queryKey: ["evidence"] });
  };

  // An <img> loaded — drop its retry record, so a *later* expiry in the same
  // session gets its own refetch instead of being read as a second failure
  // and settling into a fallback card a refetch would have fixed.
  //
  // Returns the previous Set untouched when there is nothing to drop: every
  // image on the page calls this on load, and handing back a new Set each
  // time would re-render the whole list for nothing.
  const handleImageLoadSuccess = (key: string) => {
    setRetriedFileKeys((prev) => {
      if (!prev.has(key)) return prev;
      const next = new Set(prev);
      next.delete(key);
      return next;
    });
  };

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Evidence
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Files captured manually or via the AI agent, linked to compliance controls, with review status and audit notes.
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
            Showing{" "}
            <strong>
              {filtered.length === 0
                ? 0
                : `${pageStart + 1}-${pageStart + pageRows.length}`}
            </strong>{" "}
            of {filtered.length}
          </Typography>
        </Stack>
      </Paper>

      {/* Background refetch indicator. Cached data keeps the rows on screen
          during a refetch (that's the point of staleTime below) -- this
          just makes that refresh visible instead of silent. Skipped while
          isLoading, since the skeleton already says "loading" then. */}
      {isFetching && !isLoading && <LinearProgress sx={{ mb: 2, height: 2, borderRadius: 1 }} />}

      {isLoading || isDrawingRows ? (
        // Skeleton rows instead of a centred spinner: the page's shape
        // appears immediately, in the layout that's actually going to be
        // used, rather than a blank page saying only "wait".
        //
        // Shown for two different reasons now. `isLoading` is the first
        // fetch, with no data yet. `isDrawingRows` is the gap opened up by
        // useDeferredValue above, where the data is already in hand and
        // React is still building the rows. Both want the same placeholder,
        // and rendering the real list with the deferred (empty) rows
        // instead would flash "No evidence found" for a frame.
        isMobile ? (
          <Stack spacing={1.5}>
            {Array.from({ length: 4 }).map((_, i) => (
              <Paper key={i} variant="outlined" sx={{ p: 2 }}>
                <Stack spacing={1}>
                  <Skeleton variant="text" width="30%" />
                  <Skeleton variant="text" width="70%" />
                  <Stack direction="row" spacing={0.75}>
                    <Skeleton variant="rounded" width={80} height={56} />
                    <Skeleton variant="rounded" width={80} height={56} />
                    <Skeleton variant="rounded" width={80} height={56} />
                  </Stack>
                </Stack>
              </Paper>
            ))}
          </Stack>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            <Table sx={{ tableLayout: "fixed" }}>
              <EvidenceTableHead />
              <TableBody>
                {Array.from({ length: 6 }).map((_, i) => (
                  <TableRow key={i} sx={{ "& > td": { py: 1.25, px: 2 } }}>
                    <TableCell><Skeleton variant="text" width="80%" /><Skeleton variant="text" width="50%" /></TableCell>
                    <TableCell><Skeleton variant="text" width="60%" /><Skeleton variant="text" width="90%" /></TableCell>
                    <TableCell><Skeleton variant="rounded" width={72} height={52} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width="85%" height={26} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width={70} height={20} /></TableCell>
                    <TableCell><Skeleton variant="text" width="90%" /></TableCell>
                    <TableCell align="center"><Skeleton variant="circular" width={20} height={20} sx={{ mx: "auto" }} /></TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )
      ) : isMobile ? (
        // Mobile cards. `useMediaQuery` (above, with `noSsr: true`) picks
        // between this and the desktop table below, so only one of the two
        // is ever mounted -- CSS `display: none` still downloads the
        // images inside it, which is what made a desktop visit pull four
        // images per row instead of one.
        <Stack spacing={1.5}>
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
          {deferredRows.map((e) => {
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
                    {files.slice(0, 3).map((f, i) => {
                      const isOverflowTile = i === 2 && extraCount > 0;
                      const openTile = () => {
                        if (isOverflowTile) setGalleryEvidenceId(e.id);
                        else window.open(getFileUrl(f.file_url), "_blank", "noreferrer");
                      };
                      return (
                      <Box key={f.id} role="button" tabIndex={0}
                        aria-label={isOverflowTile ? `View all ${files.length} screenshots` : `Open ${f.file_name}`}
                        sx={{ position: "relative", width: 80, height: 56, cursor: "pointer" }}
                        onClick={openTile}
                        onKeyDown={activateOnKey(openTile)}>
                        {isImageFile(f.file_name) && !failedFileKeys.has(fileKey(f)) ? (
                          <Thumbnail
                            src={stableFileUrl(getFileUrl(f.file_url))}
                            alt=""
                            onError={() => handleImageLoadError(fileKey(f), f.file_url)}
                            onLoad={() => handleImageLoadSuccess(fileKey(f))}
                            sx={{ border: "1px solid", borderColor: "divider" }}
                          />
                        ) : (
                          <FileFallbackCard fileName={f.file_name} fileUrl={getFileUrl(f.file_url)} variant="tile" />
                        )}
                        {isOverflowTile && (
                          <Box sx={{ position: "absolute", inset: 0, borderRadius: 1, backgroundColor: "rgba(0,0,0,0.55)",
                            display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontSize: "0.7rem", fontWeight: 700 }}>
                            +{extraCount}
                          </Box>
                        )}
                      </Box>
                      );
                    })}
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
                      <Tooltip title="Download all as zip">
                        <IconButton
                          size="small"
                          aria-label="Download evidence"
                          onClick={() => handleDownload(e.id)}
                          disabled={downloadingIds.has(e.id)}
                        >
                          {downloadingIds.has(e.id) ? (
                            <CircularProgress size={16} />
                          ) : (
                            <DownloadIcon size={16} />
                          )}
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Rename">
                        <IconButton size="small" aria-label="Rename evidence" onClick={() => setRenameTarget({ id: e.id, currentText: displayText })}>
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
      ) : (
        <TableContainer component={Paper} variant="outlined">
          <Table sx={{ tableLayout: "fixed" }}>
            <EvidenceTableHead />
            <TableBody>
              {deferredRows.map((e) => {
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
                        <Typography variant="caption" color="text.disabled">No control</Typography>
                      )}
                    </TableCell>

                    <TableCell>
                      <Box
                        role="button"
                        tabIndex={0}
                        aria-label={`View screenshots for ${displayText}`}
                        sx={{ position: "relative", width: 72, height: 52, cursor: "pointer" }}
                        onClick={() => setGalleryEvidenceId(e.id)}
                        onKeyDown={activateOnKey(() => setGalleryEvidenceId(e.id))}
                      >
                        {isImageFile(files[0].file_name) && !failedFileKeys.has(fileKey(files[0])) ? (
                          <Thumbnail
                            src={stableFileUrl(getFileUrl(files[0].file_url))}
                            alt=""
                            onError={() => handleImageLoadError(fileKey(files[0]), files[0].file_url)}
                            onLoad={() => handleImageLoadSuccess(fileKey(files[0]))}
                            sx={{
                              border: "1px solid",
                              borderColor: "divider",
                              "&:hover": { transform: "scale(1.05)", borderColor: "primary.main" },
                            }}
                          />
                        ) : (
                          <FileFallbackCard
                            fileName={files[0].file_name}
                            fileUrl={getFileUrl(files[0].file_url)}
                            variant="tile"
                          />
                        )}
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
                          <Tooltip title="Download all as zip">
                            <IconButton
                              size="small"
                              aria-label="Download evidence"
                              onClick={() => handleDownload(e.id)}
                              disabled={downloadingIds.has(e.id)}
                            >
                              {downloadingIds.has(e.id) ? (
                                <CircularProgress size={16} />
                              ) : (
                                <DownloadIcon size={16} />
                              )}
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Rename">
                            <IconButton
                              size="small"
                              aria-label="Rename evidence"
                              onClick={() => setRenameTarget({ id: e.id, currentText: displayText })}
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
      )}

      {/* The pager sits outside the branch above on purpose: it stays usable
          while the rows themselves are still being drawn, and it is driven
          by the full filtered list rather than the page currently on
          screen. Hidden entirely when everything already fits on one page. */}
      {filtered.length > ROWS_PER_PAGE && (
        <Stack direction="row" justifyContent="center" sx={{ mt: 2.5 }}>
          <Pagination
            count={pageCount}
            page={safePage + 1}
            onChange={(_, value) => setPage(value - 1)}
            color="primary"
            shape="rounded"
          />
        </Stack>
      )}

      {/* ── Gallery Modal ─────────────────────────────────────────────────── */}
      <Dialog
        open={galleryEvidence != null}
        onClose={(_event, reason) => {
          if (reason === "escapeKeyDown" && clampedPreviewIndex != null) {
            // First Escape leaves the preview and returns to the grid.
            setPreviewIndex(null);
            return;
          }
          setGalleryEvidenceId(null);
          setPendingDeleteFileId(null);
        }}
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
          {previewFile ? (
            (() => {
              const f = previewFile;
              const idx = clampedPreviewIndex as number;
              const isPendingFileDelete = pendingDeleteFileId === f.id;
              const isFirst = idx === 0;
              const isLast = idx === galleryFiles.length - 1;
              const showImage = isImageFile(f.file_name) && !failedFileKeys.has(fileKey(f));
              return (
                <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5 }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
                    <Typography variant="caption" color="text.secondary">
                      {idx + 1} of {galleryFiles.length}
                    </Typography>
                    <Stack direction="row" spacing={0.5}>
                      {isPendingFileDelete ? (
                        <Stack direction="row" spacing={0.5}>
                          <Button
                            size="small"
                            variant="text"
                            onClick={() => setPendingDeleteFileId(null)}
                            disabled={deleteFileMutation.isPending}
                          >
                            Cancel
                          </Button>
                          <Button
                            size="small"
                            color="error"
                            variant="contained"
                            onClick={() => deleteFileMutation.mutate(f.id)}
                            disabled={deleteFileMutation.isPending}
                          >
                            Delete
                          </Button>
                        </Stack>
                      ) : (
                        <>
                          <Tooltip title="Download">
                            <IconButton
                              component="a"
                              href={getFileUrl(f.file_url)}
                              target="_blank"
                              rel="noreferrer"
                              size="small"
                              aria-label="Download file"
                            >
                              <DownloadIcon size={16} />
                            </IconButton>
                          </Tooltip>
                          {galleryHasStoredFiles && (isAdmin || galleryEvidence?.created_by === user?.email) && (
                            <Tooltip title="Delete this screenshot">
                              <IconButton
                                size="small"
                                color="error"
                                aria-label="Delete screenshot"
                                onClick={() => setPendingDeleteFileId(f.id)}
                              >
                                <TrashIcon size={16} />
                              </IconButton>
                            </Tooltip>
                          )}
                        </>
                      )}
                    </Stack>
                  </Box>

                  <Box sx={{ position: "relative", width: "100%", display: "flex", alignItems: "center", justifyContent: "center" }}>
                    <IconButton
                      onClick={() => setPreviewIndex(idx - 1)}
                      disabled={isFirst}
                      aria-label="Previous screenshot"
                      sx={{ position: "absolute", left: 0, zIndex: 1 }}
                    >
                      <ChevronLeftIcon size={20} />
                    </IconButton>

                    <Box sx={{ width: "100%", height: { xs: 320, sm: 440 }, display: "flex", alignItems: "center", justifyContent: "center", mx: 6 }}>
                      {showImage ? (
                        // No loading="lazy" here, deliberately: this is the
                        // full-size image the user just asked to see, not a
                        // thumbnail waiting to scroll into view -- it should
                        // start loading immediately.
                        <Box
                          component="img"
                          src={stableFileUrl(getFileUrl(f.file_url))}
                          alt={f.subtask ?? `Screenshot ${idx + 1}`}
                          onError={() => handleImageLoadError(fileKey(f), f.file_url)}
                          onLoad={() => handleImageLoadSuccess(fileKey(f))}
                          sx={{ maxWidth: "100%", maxHeight: "100%", objectFit: "contain" }}
                        />
                      ) : (
                        <FileFallbackCard fileName={f.file_name} fileUrl={getFileUrl(f.file_url)} variant="preview" />
                      )}
                    </Box>

                    <IconButton
                      onClick={() => setPreviewIndex(idx + 1)}
                      disabled={isLast}
                      aria-label="Next screenshot"
                      sx={{ position: "absolute", right: 0, zIndex: 1 }}
                    >
                      <ChevronRightIcon size={20} />
                    </IconButton>
                  </Box>

                  <Typography variant="body2" color="text.secondary" sx={{ textAlign: "center" }}>
                    {f.subtask ? f.subtask : `Screenshot ${idx + 1}`}
                  </Typography>
                </Box>
              );
            })()
          ) : (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2 }}>
            {galleryFiles.map((f, idx) => {
              const isPendingFileDelete = pendingDeleteFileId === f.id;
              const showImage = isImageFile(f.file_name) && !failedFileKeys.has(fileKey(f));
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
                  <Box
                    role="button"
                    tabIndex={0}
                    aria-label={`Open ${f.subtask ?? `screenshot ${idx + 1}`}`}
                    onClick={() => setPreviewIndex(idx)}
                    onKeyDown={activateOnKey(() => setPreviewIndex(idx))}
                    sx={{ cursor: "pointer", width: "100%", aspectRatio: "16/11", overflow: "hidden" }}
                  >
                    {showImage ? (
                      <Thumbnail
                        src={stableFileUrl(getFileUrl(f.file_url))}
                        alt={f.subtask ?? `Screenshot ${idx + 1}`}
                        onError={() => handleImageLoadError(fileKey(f), f.file_url)}
                        onLoad={() => handleImageLoadSuccess(fileKey(f))}
                        sx={{ "&:hover": { opacity: 0.9 } }}
                      />
                    ) : (
                      <FileFallbackCard fileName={f.file_name} fileUrl={getFileUrl(f.file_url)} variant="grid" />
                    )}
                  </Box>
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
                      <Stack direction="row" spacing={0.5}>
                        <Tooltip title="Download">
                          <IconButton
                            component="a"
                            href={getFileUrl(f.file_url)}
                            target="_blank"
                            rel="noreferrer"
                            size="small"
                            aria-label="Download screenshot"
                            onClick={(ev) => ev.stopPropagation()}
                            sx={{ backgroundColor: "rgba(0,0,0,0.5)", color: "#fff", "&:hover": { backgroundColor: "rgba(0,0,0,0.7)" }, width: 28, height: 28 }}
                          >
                            <DownloadIcon size={14} />
                          </IconButton>
                        </Tooltip>
                        {galleryHasStoredFiles &&
                          (isAdmin || galleryEvidence?.created_by === user?.email) && (
                            <Tooltip title="Delete this screenshot">
                              <IconButton
                                size="small"
                                aria-label="Delete screenshot"
                                onClick={(ev) => { ev.stopPropagation(); setPendingDeleteFileId(f.id); }}
                                sx={{ backgroundColor: "rgba(0,0,0,0.5)", color: "#fff", "&:hover": { backgroundColor: "rgba(200,0,0,0.8)" }, width: 28, height: 28 }}
                              >
                                <TrashIcon size={14} />
                              </IconButton>
                            </Tooltip>
                          )}
                      </Stack>
                    )}
                  </Box>
                </Box>
              );
            })}
          </Box>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 2.5, py: 1.5 }}>
          <Button onClick={() => { setGalleryEvidenceId(null); setPendingDeleteFileId(null); }}>
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* ── Rename Dialog ─────────────────────────────────────────────────── */}
      <RenameEvidenceDialog
        target={renameTarget}
        isSaving={renameMutation.isPending}
        onClose={() => setRenameTarget(null)}
        onSave={(id, description) => renameMutation.mutate({ id, description })}
      />

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
