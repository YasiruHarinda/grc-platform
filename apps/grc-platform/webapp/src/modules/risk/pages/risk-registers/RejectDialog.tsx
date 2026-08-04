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

import { useState } from "react";
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

interface RejectDialogProps {
  open: boolean;
  title: string;
  description?: string;
  onClose: () => void;
  onConfirm: (comment: string) => Promise<void>;
}

export default function RejectDialog({
  open,
  title,
  description,
  onClose,
  onConfirm,
}: RejectDialogProps): JSX.Element {
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [apiError, setApiError] = useState("");

  const handleClose = () => {
    if (submitting) return;
    setComment("");
    setError("");
    setApiError("");
    onClose();
  };

  const handleSubmit = async () => {
    if (!comment.trim()) {
      setError("Rejection comment is required.");
      return;
    }
    setSubmitting(true);
    setApiError("");
    try {
      await onConfirm(comment.trim());
      setComment("");
      setError("");
      onClose();
    } catch (e: unknown) {
      setApiError(e instanceof Error ? e.message : "An error occurred.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{ sx: dialogPaperSx }}
    >
      <DialogTitle>
        <Typography component="span" variant="h6" fontWeight={700} display="block">
          {title}
        </Typography>
        {description && (
          <Typography component="span" variant="body2" color="text.secondary" sx={{ mt: 0.5 }} display="block">
            {description}
          </Typography>
        )}
      </DialogTitle>
      <DialogContent>
        {apiError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {apiError}
          </Alert>
        )}
        <Box sx={{ pt: 1 }}>
          <TextField
            label="Rejection Comment"
            multiline
            rows={4}
            fullWidth
            value={comment}
            onChange={(e) => {
              setComment(e.target.value);
              if (error) setError("");
            }}
            error={!!error}
            helperText={error}
            placeholder="Explain why this risk is being rejected..."
            disabled={submitting}
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={handleClose} disabled={submitting} color="inherit">
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={submitting}
          color="error"
          variant="contained"
        >
          {submitting ? "Rejecting..." : "Reject"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
