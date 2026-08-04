// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { dialogPaperSx } from "../cardStyles";

interface EscalationCommentDialogProps {
  open: boolean;
  riskCode: string;
  onClose: () => void;
  onConfirm: (comment: string) => Promise<void>;
}

// EscalationCommentDialog answers an escalation. This replaced the Management
// action plan: a comment alone is enough to send the risk back to its assigner,
// who then decides whether further action plans are needed.
//
// Who may submit one is decided server-side by risk level — the risk's
// Management Approver for HIGH, the assigner's or action owner's line manager
// for MEDIUM and LOW — so this dialog deliberately doesn't try to explain the
// rule; it just surfaces the server's error if the caller isn't entitled.
export default function EscalationCommentDialog({
  open,
  riskCode,
  onClose,
  onConfirm,
}: EscalationCommentDialogProps): JSX.Element {
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  // Clear on open so a previous attempt's text or error never bleeds into the
  // next risk the user opens this for.
  useEffect(() => {
    if (open) {
      setComment("");
      setError("");
    }
  }, [open]);

  const handleConfirm = async () => {
    const trimmed = comment.trim();
    if (!trimmed) {
      setError("A comment is required.");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await onConfirm(trimmed);
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Unable to submit the comment.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} maxWidth="sm" fullWidth PaperProps={{ sx: dialogPaperSx }}>
      {/* component="span": DialogTitle already renders an <h2>, and a nested
          heading-level Typography inside it is invalid HTML. */}
      <DialogTitle>
        <Typography component="span" variant="h6" fontWeight={700} sx={{ display: "block" }}>
          Review Escalation
        </Typography>
        <Typography component="span" variant="caption" color="text.secondary" sx={{ display: "block" }}>
          {riskCode}
        </Typography>
      </DialogTitle>

      <DialogContent dividers>
        <Box sx={{ mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Your comment returns this risk to its Risk Assigner. It stays in the Overdue
            Risks tab until they submit it for completion approval.
          </Typography>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <TextField
          label="Comment"
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          multiline
          minRows={4}
          fullWidth
          autoFocus
          disabled={saving}
          placeholder="What should the assigner do to bring this risk back on track?"
        />
      </DialogContent>

      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button variant="contained" onClick={handleConfirm} disabled={saving}>
          {saving ? "Submitting…" : "Submit Comment"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
