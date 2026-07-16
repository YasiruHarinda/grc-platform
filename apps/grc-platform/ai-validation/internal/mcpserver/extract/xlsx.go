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

// Package extract converts spreadsheet bytes to LLM-readable text.
package extract

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// maxRowsPerSheet caps how many rows of each sheet are sent to the LLM.
const maxRowsPerSheet = 200

// XLSXToCSV renders each sheet of an xlsx workbook as "Sheet: <name>" followed
// by CSV rows, capped at maxRowsPerSheet rows per sheet (with a truncation note).
func XLSXToCSV(data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("could not parse spreadsheet: %w", err)
	}
	defer f.Close()

	var out strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return "", fmt.Errorf("could not read sheet %q: %w", sheet, err)
		}
		fmt.Fprintf(&out, "Sheet: %s\n", sheet)
		w := csv.NewWriter(&out)
		truncated := false
		for i, row := range rows {
			if i >= maxRowsPerSheet {
				truncated = true
				break
			}
			_ = w.Write(row)
		}
		w.Flush()
		if truncated {
			fmt.Fprintf(&out, "[truncated: sheet has %d rows, showing first %d]\n", len(rows), maxRowsPerSheet)
		}
		out.WriteString("\n")
	}
	return out.String(), nil
}
