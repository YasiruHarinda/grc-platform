import { useCallback, useEffect, useRef, useState } from "react";
import { isAxiosError } from "axios";
import Box from "@mui/material/Box";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Alert from "@mui/material/Alert";
import LinearProgress from "@mui/material/LinearProgress";
import { ArrowUpIcon } from "@oxygen-ui/react-icons";
import { controlsApi } from "../api/client";
import { parseControlsCsv, type ParsedControlRow } from "../utils/parseControlsCsv";
import type { Control } from "./ControlFormDialog";

// The human-readable half of an error response's `detail`, whichever of its
// two shapes arrived. A string is used as-is; a pydantic validation list has
// its first entry's `msg` read out, which for an over-length import reads
// "List should have at most 1000 items after validation, not 1001". Anything
// else, including a network error that never reached the server, has no
// message worth showing and falls back to a sentence that at least says what
// happened.
function readDetail(detail: unknown): string {
  if (typeof detail === "string" && detail) return detail;
  if (Array.isArray(detail) && detail.length > 0) {
    const first = detail[0] as { msg?: unknown };
    if (typeof first?.msg === "string" && first.msg) return first.msg;
  }
  return "The import failed. Please try again.";
}

// The stages this dialog moves through, in order. "pick" is where every
// open starts; a bad file goes to "error" and can only go back to "pick";
// a good one goes to "summary", where nothing has been written yet and
// Cancel is still free; confirming moves to "importing" while the single
// bulk request is in flight, and "done" is the last stop, reachable even
// when nothing needed creating (a full re-import still finishes and
// reports what it skipped). A whole-request failure (network error, or the
// server refusing the request outright) lands back on "error" instead of
// "done", since nothing was written and the Admin needs a message they can
// act on rather than a blank dialog.
type ImportPhase =
  | { step: "pick" }
  | { step: "error"; message: string }
  | { step: "summary"; toCreate: ParsedControlRow[]; alreadyThereCount: number; unusableRowCount: number }
  | { step: "importing" }
  | { step: "done"; created: number; skipped: number; failed: { label: string; message: string }[] };

/**
 * Fills a Framework's Controls from the SRE team's CSV export in one pass,
 * instead of an Admin opening ControlFormDialog a hundred times. Parsing
 * and the title rule live in parseControlsCsv — this component only reads
 * the file, shows what parseControlsCsv found before anything is written,
 * and then sends every usable row to the backend's bulk endpoint in one
 * request.
 *
 * Opened either by the browse button below or by a drop on the Controls
 * column — a dropped file arrives as `initialFile` and is fed straight
 * into `handleFile`, the same path the browse button uses, so the two
 * ways in behave identically from here on.
 */
export default function ImportControlsDialog({
  open,
  frameworkId,
  frameworkName,
  productName,
  existingControls,
  initialFile,
  onClose,
  onImported,
}: {
  open: boolean;
  frameworkId: number;
  frameworkName: string;
  productName: string;
  existingControls: Control[];
  /** Set when this dialog was opened by dropping a file on the Controls
   * column rather than by the browse button. Handled once per file, the
   * same way the browse button's chosen file is handled. */
  initialFile?: File | null;
  onClose: () => void;
  onImported: () => void;
}) {
  const [phase, setPhase] = useState<ImportPhase>({ step: "pick" });

  // Every open starts clean. Reusing a stale summary or a stale error from
  // the last file this dialog saw would attach it to whatever framework is
  // selected this time, which is worse than the extra render this costs.
  // Adjusted during render rather than in an effect, so the reset lands in
  // the same pass that opens the dialog instead of one render behind it.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setPhase({ step: "pick" });
  }

  // Wrapped in useCallback so a drop's effect (below) can depend on it
  // without re-running on every render — only when the Controls this page
  // has loaded for the framework actually change.
  const handleFile = useCallback(
    (file: File) => {
      // A CSV in name only still deserves a plain answer, so this is checked
      // before the file is even read — reading and parsing a spreadsheet or a
      // PDF would just surface as a missing-column error, which is honest but
      // not as clear as saying up front what was chosen.
      if (!file.name.toLowerCase().endsWith(".csv")) {
        setPhase({ step: "error", message: "Please choose a CSV file." });
        return;
      }

      const reader = new FileReader();
      reader.onload = () => {
        const text = typeof reader.result === "string" ? reader.result : "";
        const { rows, unusableRowCount, error } = parseControlsCsv(text);

        // A missing column or an empty file already has a clear message from
        // parseControlsCsv — reused as-is rather than rewritten here.
        if (error) {
          setPhase({ step: "error", message: error });
          return;
        }

        // A reference already under this framework is skipped, not
        // duplicated and not overwritten. The check is against the Controls
        // this page already loaded for the selected framework, trimmed and
        // lowercased on both sides so stray spacing or casing in the file
        // doesn't create a duplicate that a human eye would have caught.
        const existingRefs = new Set(
          existingControls.map((c) => c.control_ref.trim().toLowerCase())
        );
        const toCreate = rows.filter((row) => !existingRefs.has(row.reference.trim().toLowerCase()));

        setPhase({
          step: "summary",
          toCreate,
          alreadyThereCount: rows.length - toCreate.length,
          unusableRowCount,
        });
      };
      reader.onerror = () => setPhase({ step: "error", message: "Couldn't read the file." });
      reader.readAsText(file, "utf-8");
    },
    [existingControls]
  );

  // A dropped file is handled once, the moment it arrives, rather than
  // during render — reading a file is a side effect and belongs in an
  // effect, not the render pass that the open/close reset above runs in.
  // The ref stops the same File being handled twice: once for the initial
  // effect run and again if this component re-renders while the dialog is
  // still open with the same dropped file.
  const handledDropRef = useRef<File | null>(null);
  useEffect(() => {
    if (open && initialFile && handledDropRef.current !== initialFile) {
      handledDropRef.current = initialFile;
      handleFile(initialFile);
    }
    if (!open) handledDropRef.current = null;
  }, [open, initialFile, handleFile]);

  async function handleConfirm() {
    if (phase.step !== "summary") return;
    const { toCreate, alreadyThereCount } = phase;
    setPhase({ step: "importing" });

    // One request for the whole file. The server checks every row before
    // writing any of them and commits once, so there is nothing to count
    // as it runs — either the request comes back with the outcome, or
    // nothing was written at all.
    try {
      const result = await controlsApi.bulkCreate({
        framework_id: frameworkId,
        controls: toCreate.map((row) => ({
          control_ref: row.reference,
          title: row.title,
          description: row.description,
        })),
      });

      // Refresh the Controls column with what's already loaded for this
      // framework rather than starting a second query for the same data.
      onImported();

      // Both counts, added, because the two sides skip different rows and
      // neither number is the whole answer on its own. The rows this
      // browser already knew were stored never went in the request at all
      // (see `toCreate` above), so the server cannot count them; the
      // server's own count catches what this page's snapshot missed,
      // which is what another Admin importing at the same moment looks
      // like. Reporting the server's count alone is what made a full
      // re-import say "0 skipped" one screen after the summary said 103.
      //
      // A rejected row is labelled by its reference where it has one, and
      // by its position in the file where it does not: a row rejected for
      // a *blank* reference has nothing else to identify it by.
      setPhase({
        step: "done",
        created: result.created.length,
        skipped: alreadyThereCount + result.skipped,
        failed: result.rejected.map((r) => ({
          label: r.control_ref || `Row ${r.row_number}`,
          message: r.reason,
        })),
      });
    } catch (err) {
      // A whole-request failure, a network error or the server refusing
      // the request outright (missing Framework, over the row limit, a
      // conflict, a non-admin caller). Nothing was written, so this goes
      // back to "error" with whatever plain reason the server gave.
      //
      // `detail` arrives in one of two shapes and both have to be read.
      // The backend's own refusals carry a plain string. FastAPI's
      // request-validation 422 carries a *list* of error objects instead,
      // which is what a file over the row limit now comes back as, since
      // that cap lives on the schema. Handing either the list or one of
      // its objects to the error screen would render an object as a React
      // child and blank the dialog, so the message is pulled out of the
      // first entry rather than the structure being passed along.
      const detail = isAxiosError(err)
        ? (err.response?.data as { detail?: unknown } | undefined)?.detail
        : undefined;
      setPhase({ step: "error", message: readDetail(detail) });
    }
  }

  function handleClose() {
    onClose();
  }

  const importing = phase.step === "importing";
  const pluralize = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

  return (
    <Dialog open={open} onClose={importing ? undefined : handleClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>
        <Stack direction="row" alignItems="center" spacing={1.5}>
          <Box sx={{ color: "primary.main", display: "flex" }}>
            <ArrowUpIcon size={22} />
          </Box>
          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.2 }}>
              Import Controls
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Fill "{frameworkName}" from a CSV file instead of adding Controls one at a time.
            </Typography>
          </Box>
        </Stack>
      </DialogTitle>

      <DialogContent dividers>
        {phase.step === "pick" && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              The file needs a "Control Number" column and a "Control Description from PY Report"
              column. It is read in your browser and nothing is written until you confirm.
            </Typography>
            <Button
              component="label"
              variant="outlined"
              fullWidth
              startIcon={<ArrowUpIcon size={18} />}
              sx={{
                py: 1.75,
                borderStyle: "dashed",
                borderColor: "divider",
                justifyContent: "flex-start",
                px: 2,
                "&:hover": { borderStyle: "dashed", borderColor: "primary.main", backgroundColor: "rgba(255,115,0,0.04)" },
              }}
            >
              Click to choose a CSV file
              <input
                type="file"
                accept=".csv,text/csv"
                hidden
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) handleFile(file);
                }}
              />
            </Button>
          </Stack>
        )}

        {phase.step === "error" && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Alert severity="error">{phase.message}</Alert>
            <Button
              component="label"
              variant="outlined"
              fullWidth
              startIcon={<ArrowUpIcon size={18} />}
              sx={{ py: 1.75, borderStyle: "dashed", borderColor: "divider" }}
            >
              Choose a different file
              <input
                type="file"
                accept=".csv,text/csv"
                hidden
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) handleFile(file);
                }}
              />
            </Button>
          </Stack>
        )}

        {phase.step === "summary" && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Typography variant="body2">
              Importing into the <strong>{frameworkName}</strong> framework, under the{" "}
              <strong>{productName}</strong> product.
            </Typography>
            <Alert severity="info" sx={{ "& .MuiAlert-message": { width: "100%" } }}>
              <Stack spacing={0.25}>
                <Typography variant="body2">
                  • <strong>{phase.toCreate.length}</strong> {pluralize(phase.toCreate.length, "control")} will
                  be created
                </Typography>
                <Typography variant="body2">
                  • <strong>{phase.alreadyThereCount}</strong> already{" "}
                  {phase.alreadyThereCount === 1 ? "exists" : "exist"} under this framework and will be skipped
                </Typography>
                {phase.unusableRowCount > 0 && (
                  <Typography variant="body2">
                    • <strong>{phase.unusableRowCount}</strong> {pluralize(phase.unusableRowCount, "row")} in
                    the file could not be used
                  </Typography>
                )}
              </Stack>
            </Alert>
          </Stack>
        )}

        {phase.step === "importing" && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Typography variant="body2">Creating controls…</Typography>
            <LinearProgress />
          </Stack>
        )}

        {phase.step === "done" && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Alert
              severity={phase.failed.length > 0 ? "warning" : "success"}
              sx={{ "& .MuiAlert-message": { width: "100%" } }}
            >
              <Typography variant="body2" fontWeight={700} sx={{ mb: 0.5 }}>
                {phase.failed.length > 0 ? "Import finished, but not everything went through." : "Import finished."}
              </Typography>
              <Stack spacing={0.25}>
                <Typography variant="body2">
                  • <strong>{phase.created}</strong> created
                </Typography>
                <Typography variant="body2">
                  • <strong>{phase.skipped}</strong> skipped (already under this framework)
                </Typography>
                <Typography variant="body2">
                  • <strong>{phase.failed.length}</strong> failed
                </Typography>
              </Stack>
            </Alert>
            {phase.failed.length > 0 && (
              <Alert severity="error" sx={{ "& .MuiAlert-message": { width: "100%" } }}>
                <Typography variant="body2" fontWeight={700} sx={{ mb: 0.5 }}>
                  Not created:
                </Typography>
                <Stack spacing={0.25}>
                  {phase.failed.map((f, i) => (
                    <Typography key={`${f.label}-${i}`} variant="body2">
                      • <strong>{f.label}</strong>: {f.message}
                    </Typography>
                  ))}
                </Stack>
              </Alert>
            )}
          </Stack>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 1.75 }}>
        {phase.step === "summary" && (
          <>
            <Button onClick={handleClose}>Cancel</Button>
            <Button onClick={handleConfirm} variant="contained">
              {phase.toCreate.length > 0 ? `Create ${pluralize(phase.toCreate.length, "control")}` : "Continue"}
            </Button>
          </>
        )}
        {phase.step === "importing" && <Button disabled>Importing…</Button>}
        {(phase.step === "pick" || phase.step === "error") && <Button onClick={handleClose}>Cancel</Button>}
        {phase.step === "done" && (
          <Button onClick={handleClose} variant="contained">
            Done
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}
