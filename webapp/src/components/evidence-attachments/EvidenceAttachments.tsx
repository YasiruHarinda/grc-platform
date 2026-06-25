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

import { useRef, useState } from "react";
import { Box, IconButton, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { CloudUpload, Trash2 } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";

export interface EvidenceAttachment {
  file: File | null;
  note: string;
}

interface EvidenceAttachmentsProps {
  value: EvidenceAttachment[];
  onChange: (files: EvidenceAttachment[]) => void;
  /** Native file input accept string. Omit to allow all file types. */
  accept?: string;
}

export default function EvidenceAttachments({
  value,
  onChange,
  accept,
}: EvidenceAttachmentsProps): JSX.Element {
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const addFiles = (incoming: FileList | File[]): void => {
    const newFiles = Array.from(incoming);
    if (newFiles.length) {
      onChange([...value, ...newFiles.map((f) => ({ file: f, note: "" }))]);
    }
  };

  const removeFile = (index: number): void => {
    onChange(value.filter((_, i) => i !== index));
  };

  const updateNote = (index: number, note: string): void => {
    onChange(value.map((item, i) => (i === index ? { ...item, note } : item)));
  };

  return (
    <Stack gap={2}>
      {/* Hidden file input */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        hidden
        accept={accept}
        onChange={(e) => {
          if (e.target.files) addFiles(e.target.files);
          e.target.value = "";
        }}
      />

      {/* Drop zone */}
      <Box
        onDragOver={(e) => { e.preventDefault(); setIsDragging(true); }}
        onDragEnter={(e) => { e.preventDefault(); setIsDragging(true); }}
        onDragLeave={() => setIsDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setIsDragging(false);
          if (e.dataTransfer.files) addFiles(e.dataTransfer.files);
        }}
        onClick={() => fileInputRef.current?.click()}
        sx={{
          border: "2px dashed",
          borderColor: isDragging ? "primary.main" : "divider",
          borderRadius: 2,
          p: 4,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: 1,
          cursor: "pointer",
          userSelect: "none",
          bgcolor: isDragging
            ? "rgba(var(--oxygen-palette-primary-mainChannel) / 0.06)"
            : "transparent",
          transition: "border-color 0.15s ease, background-color 0.15s ease",
          "&:hover": {
            borderColor: "primary.main",
            bgcolor: "rgba(var(--oxygen-palette-primary-mainChannel) / 0.04)",
          },
        }}
      >
        <Box sx={{ color: isDragging ? "primary.main" : "text.secondary" }}>
          <CloudUpload size={36} />
        </Box>
        <Typography
          variant="body2"
          fontWeight={500}
          color={isDragging ? "primary.main" : "text.primary"}
        >
          {isDragging ? "Drop files here" : "Drag & drop files here"}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          or click to browse (supports multiple files)
        </Typography>
      </Box>

      {/* File list */}
      {value.length > 0 && (
        <Stack gap={1.5}>
          {value.map((item, index) => {
            const sizeLabel = item.file
              ? item.file.size >= 1_048_576
                ? `${(item.file.size / 1_048_576).toFixed(1)} MB`
                : `${(item.file.size / 1024).toFixed(0)} KB`
              : null;

            return (
              <Stack
                key={`${item.file?.name ?? "file"}-${index}`}
                direction="row"
                gap={1.5}
                alignItems="center"
              >
                {/* File name + size */}
                <Box sx={{ minWidth: 0, flex: "0 0 220px" }}>
                  <Typography
                    variant="body2"
                    fontWeight={500}
                    noWrap
                    title={item.file?.name ?? "No file"}
                  >
                    {item.file?.name ?? "No file selected"}
                  </Typography>
                  {sizeLabel && (
                    <Typography variant="caption" color="text.secondary">
                      {sizeLabel}
                    </Typography>
                  )}
                </Box>
                {/* Per-file note */}
                <TextField
                  size="small"
                  fullWidth
                  placeholder="Optional note…"
                  label="Note"
                  value={item.note}
                  onChange={(e) => updateNote(index, e.target.value)}
                />
                {/* Remove */}
                <IconButton
                  onClick={() => removeFile(index)}
                  size="small"
                  sx={{ flexShrink: 0, color: "error.main" }}
                  aria-label={`Remove attachment ${index + 1}`}
                >
                  <Trash2 size={16} />
                </IconButton>
              </Stack>
            );
          })}
        </Stack>
      )}
    </Stack>
  );
}