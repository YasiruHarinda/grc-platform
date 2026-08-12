import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Stack from "@mui/material/Stack";
import Box from "@mui/material/Box";
import Alert from "@mui/material/Alert";
import { TrashIcon } from "@oxygen-ui/react-icons";

export type CascadeImpact = {
  label: string;
  count: number;
};

type Props = {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  isPending?: boolean;
  impactLoading?: boolean;
  entityType: string;
  entityName: string;
  impact?: CascadeImpact[];
  warnings?: string[];
  error?: string | null;
};

export default function ConfirmDeleteDialog({
  open,
  onClose,
  onConfirm,
  isPending = false,
  impactLoading = false,
  entityType,
  entityName,
  impact = [],
  warnings = [],
  error = null,
}: Props) {
  const totalImpact = impact.reduce((sum, i) => sum + i.count, 0);

  return (
    <Dialog open={open} onClose={isPending ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ pb: 1 }}>
        <Stack direction="row" alignItems="center" spacing={1.5}>
          <Box sx={{ color: "error.main", display: "flex" }}>
            <TrashIcon size={22} />
          </Box>
          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.2 }}>
              Delete {entityType}?
            </Typography>
            <Typography variant="caption" color="text.secondary">
              This action cannot be undone.
            </Typography>
          </Box>
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography variant="body2">
            You're about to delete{" "}
            <Box component="span" sx={{ fontWeight: 700 }}>
              "{entityName}"
            </Box>
            .
          </Typography>

          {warnings.length > 0 && (
            <Alert severity="error" sx={{ "& .MuiAlert-message": { width: "100%" } }}>
              <Stack spacing={0.25}>
                {warnings.map((warning, i) => (
                  <Typography key={i} variant="body2">
                    {warning}
                  </Typography>
                ))}
              </Stack>
            </Alert>
          )}

          {impactLoading ? (
            <Typography variant="caption" color="text.secondary">
              Checking for related records…
            </Typography>
          ) : (
            totalImpact > 0 && (
              <Alert severity="warning" sx={{ "& .MuiAlert-message": { width: "100%" } }}>
                <Typography variant="body2" fontWeight={700} sx={{ mb: 0.5 }}>
                  This will also permanently delete:
                </Typography>
                <Stack spacing={0.25} sx={{ pl: 1 }}>
                  {impact
                    .filter((i) => i.count > 0)
                    .map((i) => (
                      <Typography key={i.label} variant="body2">
                        • <strong>{i.count}</strong> {i.label}
                      </Typography>
                    ))}
                </Stack>
              </Alert>
            )
          )}

          {error && <Alert severity="error">{error}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 1.75 }}>
        <Button onClick={onClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={onConfirm}
          color="error"
          variant="contained"
          disabled={isPending || impactLoading}
        >
          {isPending ? "Deleting..." : `Delete ${entityType}`}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
