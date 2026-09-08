import { useState } from "react";
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

// The stages this dialog moves through, in order. "pick" is where every
// open starts; a bad file goes to "error" and can only go back to "pick";
// a good one goes to "summary", where nothing has been written yet and
// Cancel is still free; confirming moves to "importing" while the requests
// go out one at a time, and "done" is the last stop, reachable even when
// nothing needed creating (a full re-import still finishes and reports
// what it skipped).
type ImportPhase =
  | { step: "pick" }
  | { step: "error"; message: string }
  | { step: "summary"; toCreate: ParsedControlRow[]; alreadyThereCount: number; unusableRowCount: number }
  | { step: "importing"; total: number; done: number }
  | { step: "done"; created: number; skipped: number; failed: { reference: string; message: string }[] };

/**
 * Fills a Framework's Controls from the SRE team's CSV export in one pass,
 * instead of an Admin opening ControlFormDialog a hundred times. Parsing
 * and the title rule live in parseControlsCsv — this component only reads
 * the file, shows what parseControlsCsv found before anything is written,
 * and then creates the usable rows one request at a time, since the
 * backend has no bulk endpoint.
 *
 * Browse only. Dragging a file onto the column is ticket #123's job.
 */
export default function ImportControlsDialog({
  open,
  frameworkId,
  frameworkName,
  productName,
  existingControls,
  onClose,
  onImported,
}: {
  open: boolean;
  frameworkId: number;
  frameworkName: string;
  productName: string;
  existingControls: Control[];
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

  function handleFile(file: File) {
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
  }

  async function handleConfirm() {
    if (phase.step !== "summary") return;
    const { toCreate, alreadyThereCount } = phase;
    setPhase({ step: "importing", total: toCreate.length, done: 0 });

    let created = 0;
    const failed: { reference: string; message: string }[] = [];

    // One request per Control, in order, because the backend has no bulk
    // endpoint. A failed row is recorded and the loop moves on rather than
    // stopping, so one bad row never costs the other ninety-nine.
    for (const row of toCreate) {
      try {
        await controlsApi.create({
          framework_id: frameworkId,
          control_ref: row.reference,
          title: row.title,
          description: row.description,
        });
        created += 1;
      } catch (err) {
        const detail = isAxiosError(err)
          ? (err.response?.data as { detail?: string } | undefined)?.detail
          : undefined;
        failed.push({ reference: row.reference, message: detail || "The server rejected this row." });
      }
      setPhase((prev) => (prev.step === "importing" ? { ...prev, done: prev.done + 1 } : prev));
    }

    // Refresh the Controls column with what's already loaded for this
    // framework rather than starting a second query for the same data.
    onImported();
    setPhase({ step: "done", created, skipped: alreadyThereCount, failed });
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
            <Typography variant="body2">
              Creating controls… {phase.done} of {phase.total}
            </Typography>
            <LinearProgress
              variant="determinate"
              value={phase.total === 0 ? 100 : (phase.done / phase.total) * 100}
            />
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
                    <Typography key={`${f.reference}-${i}`} variant="body2">
                      • <strong>{f.reference}</strong>: {f.message}
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
