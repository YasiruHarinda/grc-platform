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

import {
  Box,
  Button,
  Checkbox,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Popover,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Filter, Search, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";

export interface ColumnFilterOption {
  label: string;
  value: string;
}

interface ColumnFilterProps {
  label: string;
  options: ColumnFilterOption[];
  selected: string[];
  onChange: (values: string[]) => void;
  searchable?: boolean;
}

// Spreadsheet-style per-column filter: a small icon button that opens a
// checkbox multi-select popover. Selecting values is OR-within-column;
// combined with the table's other active filters via AND. Mirrors the
// Audit module's ControlsTable ColumnFilter (not shared cross-module, to
// keep that already-working feature untouched).
export default function ColumnFilter({
  label,
  options,
  selected,
  onChange,
  searchable = false,
}: ColumnFilterProps): JSX.Element {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");

  const isActive = selected.length > 0;
  const open = Boolean(anchorEl);

  const visible = query.trim()
    ? options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()))
    : options;

  function toggle(value: string) {
    onChange(selected.includes(value) ? selected.filter((v) => v !== value) : [...selected, value]);
  }

  function handleClose() {
    setAnchorEl(null);
    setQuery("");
  }

  return (
    <>
      <IconButton
        size="small"
        aria-label={`Filter by ${label}`}
        onClick={(e) => {
          e.stopPropagation();
          setAnchorEl(e.currentTarget);
        }}
        sx={{
          ml: 0.25,
          p: 0.25,
          borderRadius: 0.75,
          color: isActive ? "primary.main" : "text.primary",
          bgcolor: isActive ? "rgba(25,118,210,0.08)" : "transparent",
          "&:hover": {
            color: isActive ? "primary.main" : "text.primary",
            bgcolor: isActive ? "rgba(25,118,210,0.12)" : "action.hover",
          },
        }}
      >
        {/* oxygen-ui-icons-react ships a global ".lucide { stroke-width: 1.5px }"
            rule that overrides the strokeWidth prop (a CSS class rule beats an
            SVG presentation attribute) — an inline style wins over it instead. */}
        <Filter size={14} style={{ strokeWidth: 1 }} />
      </IconButton>

      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
        slotProps={{ paper: { sx: { width: 230, borderRadius: 2, mt: 0.5 } } }}
        onClick={(e) => e.stopPropagation()}
      >
        <Box sx={{ p: 1.25 }}>
          {searchable && (
            <TextField
              size="small"
              fullWidth
              placeholder="Search..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              autoFocus
              sx={{ mb: 0.75 }}
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <Search size={14} />
                    </InputAdornment>
                  ),
                  endAdornment: query ? (
                    <InputAdornment position="end">
                      <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => setQuery("")}>
                        <X size={12} />
                      </IconButton>
                    </InputAdornment>
                  ) : null,
                },
              }}
            />
          )}

          {isActive && (
            <Button
              size="small"
              onClick={() => onChange([])}
              sx={{ textTransform: "none", fontSize: "0.72rem", py: 0.25, mb: 0.5, display: "block" }}
            >
              Clear ({selected.length} selected)
            </Button>
          )}

          <Box sx={{ maxHeight: 260, overflowY: "auto" }}>
            {visible.length === 0 ? (
              <Typography variant="caption" color="text.secondary" sx={{ px: 1, py: 1, display: "block" }}>
                No matches
              </Typography>
            ) : (
              visible.map((opt) => (
                <FormControlLabel
                  key={opt.value}
                  control={
                    <Checkbox
                      size="small"
                      checked={selected.includes(opt.value)}
                      onChange={() => toggle(opt.value)}
                      disableRipple
                      sx={{ p: 0.5 }}
                    />
                  }
                  label={
                    <Typography variant="body2" sx={{ fontSize: "0.82rem", lineHeight: 1.4 }}>
                      {opt.label}
                    </Typography>
                  }
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    px: 0.5,
                    py: 0.1,
                    borderRadius: 1,
                    mx: 0,
                    width: "100%",
                    "&:hover": { bgcolor: "action.hover" },
                  }}
                />
              ))
            )}
          </Box>
        </Box>
      </Popover>
    </>
  );
}
