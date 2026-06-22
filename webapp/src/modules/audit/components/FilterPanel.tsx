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
  Badge,
  Button,
  Chip,
  Divider,
  IconButton,
  InputAdornment,
  Popover,
  TextField,
} from "@mui/material";
import { Box, Typography } from "@wso2/oxygen-ui";
import { Search, SlidersHorizontal, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX, type MouseEvent } from "react";

export interface FilterOption {
  label: string;
  value: string;
}

export interface FilterField {
  /** Unique key matching the key in the values Record. */
  id: string;
  /** Section heading shown in the popover. */
  label: string;
  /** Available options to toggle. */
  options: FilterOption[];
}

export interface FilterPanelProps {
  /** Field definitions — static options (enum-like) or derived from data. */
  fields: FilterField[];
  /** Currently applied filter values, keyed by field id. */
  values: Record<string, string[]>;
  /** Called when the user clicks Apply with the new values. */
  onChange: (values: Record<string, string[]>) => void;
  /** Current search string — applies instantly on keystroke. */
  search: string;
  /** Called on every keystroke. */
  onSearchChange: (search: string) => void;
  /** Placeholder text for the search field. */
  searchPlaceholder?: string;
}

function countActive(values: Record<string, string[]>): number {
  return Object.values(values).filter((arr) => arr.length > 0).length;
}

function emptyValues(fields: FilterField[]): Record<string, string[]> {
  return Object.fromEntries(fields.map((f) => [f.id, []]));
}

/**
 * Filter toolbar: an instant-search input alongside a "Filters" button that
 * opens a popover. Each field shows toggle chips; changes are staged until Apply.
 */
export default function FilterPanel({
  fields,
  values,
  onChange,
  search,
  onSearchChange,
  searchPlaceholder = "Search...",
}: FilterPanelProps): JSX.Element {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [pending, setPending] = useState<Record<string, string[]>>(values);

  const open = Boolean(anchorEl);
  const activeCount = countActive(values);

  const handleOpen = (e: MouseEvent<HTMLElement>) => {
    setPending(values);
    setAnchorEl(e.currentTarget);
  };

  const handleClose = () => setAnchorEl(null);

  const handleApply = () => {
    onChange(pending);
    handleClose();
  };

  const handleClear = () => {
    const cleared = emptyValues(fields);
    setPending(cleared);
    onChange(cleared);
    handleClose();
  };

  const toggle = (fieldId: string, value: string) => {
    setPending((prev) => {
      const current = prev[fieldId] ?? [];
      const updated = current.includes(value)
        ? current.filter((v) => v !== value)
        : [...current, value];
      return { ...prev, [fieldId]: updated };
    });
  };

  return (
    <>
      {/* Search + Filters row */}
      <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
        <TextField
          size="small"
          placeholder={searchPlaceholder}
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          sx={{ flex: 1 }}
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <Search size={16} />
                </InputAdornment>
              ),
              endAdornment: search ? (
                <InputAdornment position="end">
                  <IconButton
                    size="small"
                    edge="end"
                    aria-label="Clear search"
                    onClick={() => onSearchChange("")}
                  >
                    <X size={14} />
                  </IconButton>
                </InputAdornment>
              ) : null,
            },
          }}
        />

        <Badge badgeContent={activeCount} color="primary" invisible={activeCount === 0}>
          <Button
            variant={activeCount > 0 ? "contained" : "outlined"}
            size="small"
            startIcon={<SlidersHorizontal size={15} />}
            onClick={handleOpen}
            sx={{ textTransform: "none", fontWeight: 500, whiteSpace: "nowrap" }}
          >
            Filters
          </Button>
        </Badge>
      </Box>

      {/* Filter popover */}
      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
        slotProps={{
          paper: {
            sx: {
              width: 340,
              maxWidth: "calc(100vw - 32px)",
              maxHeight: 560,
              display: "flex",
              flexDirection: "column",
              borderRadius: 2,
              mt: 0.5,
            },
          },
        }}
      >
        {/* Header */}
        <Box
          sx={{
            px: 2,
            py: 1.5,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            flexShrink: 0,
          }}
        >
          <Typography variant="subtitle2" fontWeight={700}>
            Filters
          </Typography>
          <IconButton size="small" onClick={handleClose} aria-label="Close filters">
            <X size={16} />
          </IconButton>
        </Box>

        <Divider />

        {/* Scrollable body */}
        <Box sx={{ overflowY: "auto", flex: 1, px: 2, py: 1.5 }}>
          {fields.map((field, idx) => {
            const selected = pending[field.id] ?? [];
            return (
              <Box key={field.id} sx={{ mb: idx < fields.length - 1 ? 2.5 : 0 }}>
                <Typography
                  variant="caption"
                  fontWeight={700}
                  color="text.secondary"
                  sx={{
                    textTransform: "uppercase",
                    letterSpacing: 0.6,
                    display: "block",
                    mb: 0.75,
                  }}
                >
                  {field.label}
                </Typography>
                <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>
                  {field.options.map((opt) => {
                    const isSelected = selected.includes(opt.value);
                    return (
                      <Chip
                        key={opt.value}
                        label={opt.label}
                        size="small"
                        variant={isSelected ? "filled" : "outlined"}
                        color={isSelected ? "primary" : "default"}
                        onClick={() => toggle(field.id, opt.value)}
                        sx={{ cursor: "pointer" }}
                      />
                    );
                  })}
                </Box>
              </Box>
            );
          })}
        </Box>

        <Divider />

        {/* Footer */}
        <Box
          sx={{
            px: 2,
            py: 1.25,
            display: "flex",
            justifyContent: "flex-end",
            gap: 1,
            flexShrink: 0,
          }}
        >
          <Button
            size="small"
            variant="text"
            onClick={handleClear}
            sx={{ textTransform: "none" }}
          >
            Clear all
          </Button>
          <Button
            size="small"
            variant="contained"
            onClick={handleApply}
            sx={{ textTransform: "none" }}
          >
            Apply
          </Button>
        </Box>
      </Popover>
    </>
  );
}
