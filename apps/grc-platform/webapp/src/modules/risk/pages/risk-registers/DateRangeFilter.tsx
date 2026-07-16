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
  AdapterDateFns,
  Box,
  Button,
  Checkbox,
  DatePickers,
  FormControlLabel,
  IconButton,
  Popover,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Filter } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { parseDateOnly, toDateOnlyString } from "@utils/dateTime";

const { DatePicker, LocalizationProvider } = DatePickers;

// Matches EditRiskDialog's datepickerPaperSx: the calendar popper otherwise
// picks up backdrop blur/transparency from its portal context, making it
// see-through against the page behind it.
const datepickerPaperSx = {
  backdropFilter: "none",
  backgroundColor: "#fff",
  "[data-color-scheme='dark'] &": { backgroundColor: "#1e1e1e" },
};

interface DateRangeFilterProps {
  label: string;
  from: string;
  to: string;
  onChange: (from: string, to: string) => void;
  // Due-column-only: an extra "Overdue only" checkbox alongside the range,
  // mirroring the Audit module's synthetic "Overdue" status-filter option.
  overdueOnly?: boolean;
  onOverdueOnlyChange?: (value: boolean) => void;
}

// Spreadsheet-style date-range column filter: a small icon button opening a
// From/To date-picker popover. Combined with the table's other active
// filters via AND, same as ColumnFilter.
export default function DateRangeFilter({
  label,
  from,
  to,
  onChange,
  overdueOnly,
  onOverdueOnlyChange,
}: DateRangeFilterProps): JSX.Element {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  const isActive = Boolean(from || to || overdueOnly);
  const open = Boolean(anchorEl);

  function handleClose() {
    setAnchorEl(null);
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
        slotProps={{ paper: { sx: { width: 260, borderRadius: 2, mt: 0.5 } } }}
        onClick={(e) => e.stopPropagation()}
      >
        <Box sx={{ p: 1.5 }}>
          <LocalizationProvider dateAdapter={AdapterDateFns}>
            <Stack gap={1.5}>
              <DatePicker
                label="From"
                value={parseDateOnly(from)}
                onChange={(d) => {
                  const newFrom = toDateOnlyString(d) ?? "";
                  const toDate = parseDateOnly(to);
                  // Keep the range valid: if From moves past the current To, clear To
                  // rather than leave an inverted range in place.
                  const newTo = d && toDate && d > toDate ? "" : to;
                  onChange(newFrom, newTo);
                }}
                maxDate={parseDateOnly(to) ?? undefined}
                slotProps={{
                  desktopPaper: { sx: datepickerPaperSx },
                  textField: { size: "small", fullWidth: true },
                }}
              />
              <DatePicker
                label="To"
                value={parseDateOnly(to)}
                onChange={(d) => onChange(from, toDateOnlyString(d) ?? "")}
                minDate={parseDateOnly(from) ?? undefined}
                slotProps={{
                  desktopPaper: { sx: datepickerPaperSx },
                  textField: { size: "small", fullWidth: true },
                }}
              />
            </Stack>
          </LocalizationProvider>

          {onOverdueOnlyChange && (
            <FormControlLabel
              control={
                <Checkbox
                  size="small"
                  checked={overdueOnly ?? false}
                  onChange={(e) => onOverdueOnlyChange(e.target.checked)}
                />
              }
              label={<Typography variant="body2">Overdue only</Typography>}
              sx={{ mt: 1, ml: 0 }}
            />
          )}

          {isActive && (
            <Button
              size="small"
              onClick={() => {
                onChange("", "");
                onOverdueOnlyChange?.(false);
              }}
              sx={{ textTransform: "none", fontSize: "0.72rem", py: 0.25, mt: 0.5, display: "block" }}
            >
              Clear
            </Button>
          )}
        </Box>
      </Popover>
    </>
  );
}
