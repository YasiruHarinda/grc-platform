import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Stack from "@mui/material/Stack";
import Box from "@mui/material/Box";
import Alert from "@mui/material/Alert";
import { ClockAsteriskIcon } from "@oxygen-ui/react-icons";

type Props = {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  isPending?: boolean;
  error?: string | null;
};

// Separate from ConfirmDeleteDialog on purpose: a reset does not describe a
// deletion, so its wording (and the dialog it sits in) must not either. It
// follows the same shape — a stated consequence, a button disabled while the
// request is in flight, and the error rendered inside the dialog.
export default function ConfirmResetDialog({ open, onClose, onConfirm, isPending = false, error = null }: Props) {
  return (
    <Dialog open={open} onClose={isPending ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>
        <Stack direction="row" alignItems="center" spacing={1.5}>
          <Box sx={{ color: "primary.main", display: "flex" }}>
            <ClockAsteriskIcon size={22} />
          </Box>
          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.2 }}>
              Reset the cost counters?
            </Typography>
            <Typography variant="caption" color="text.secondary">
              This starts a new counting period.
            </Typography>
          </Box>
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography variant="body2">
            The page will start counting from zero. Every figure above will read as zero until the next Agent Task
            finishes.
          </Typography>
          <Typography variant="body2">Past runs stay saved and can still be reported on if they are ever needed.</Typography>

          {error && <Alert severity="error">{error}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 1.75 }}>
        <Button onClick={onClose} disabled={isPending}>
          Cancel
        </Button>
        <Button onClick={onConfirm} color="primary" variant="contained" disabled={isPending}>
          {isPending ? "Resetting..." : "Reset counters"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
