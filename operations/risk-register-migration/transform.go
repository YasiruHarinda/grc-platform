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

package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parseIntish parses a whole number that may carry Excel's ".0" numeric
// formatting (e.g. "2025.0", "3.0"). A fractional or non-numeric value errors.
func parseIntish(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not numeric", s)
	}
	// ParseFloat also accepts "Inf"/"NaN" and exponent/hex forms ("1e30"): those
	// can clear the whole-number check below, and int(f) on a value that doesn't
	// fit is implementation-defined garbage. Migration ID stores this result
	// unbounded (sheet.go), so screen non-finite and out-of-range here.
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%q is not a finite number", s)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("%q is not a whole number", s)
	}
	if intLimit := math.Ldexp(1, strconv.IntSize-1); f < -intLimit || f >= intLimit {
		return 0, fmt.Errorf("%q is out of range for an integer", s)
	}
	return int(f), nil
}

// parseYear applies parseIntish and sanity-bounds the result — a value outside
// 2000..2100 in the Year column is a mis-keyed cell, not a year.
func parseYear(s string) (int, error) {
	n, err := parseIntish(s)
	if err != nil {
		return 0, err
	}
	if n < 2000 || n > 2100 {
		return 0, fmt.Errorf("year %d is outside 2000..2100", n)
	}
	return n, nil
}

// parseScore parses a likelihood or impact value: an integer 1..3 (Excel ".0"
// tolerated), else an error.
func parseScore(s string) (int, error) {
	n, err := parseIntish(s)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > 3 {
		return 0, fmt.Errorf("%d is outside 1..3", n)
	}
	return n, nil
}

var quarterRe = regexp.MustCompile(`^Q[1-4]$`)

// parseQuarter normalises to "Q1".."Q4" (case-insensitive input), else errors.
func parseQuarter(s string) (string, error) {
	q := strings.ToUpper(strings.TrimSpace(s))
	if !quarterRe.MatchString(q) {
		return "", fmt.Errorf("%q is not Q1..Q4", s)
	}
	return q, nil
}

// validTreatments maps an upper-cased input to the treatment_strategy enum.
// VOID is the pre-rename spelling still present in older sheets (plan §2 / §7).
var validTreatments = map[string]string{
	"ACCEPT":    "ACCEPT",
	"TRANSFER":  "TRANSFER",
	"REMEDIATE": "REMEDIATE",
	"AVOID":     "AVOID",
	"VOID":      "AVOID",
}

// mapTreatment upper-cases and validates the treatment strategy. Blank is an
// error — every migrated row is post-approval and must carry one (plan §6).
func mapTreatment(s string) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(s))
	if key == "" {
		return "", fmt.Errorf("treatment strategy is blank")
	}
	v, ok := validTreatments[key]
	if !ok {
		return "", fmt.Errorf("%q is not Accept/Transfer/Remediate/Avoid", s)
	}
	return v, nil
}

var wsSepRe = regexp.MustCompile(`[\s\-]+`)

// mapWorkflowStatus accepts exactly IN_REMEDIATION or CLOSED, case-insensitive,
// tolerating spaces/hyphens for underscores ("In Remediation"). Anything else —
// blank included — is an error (plan §5).
func mapWorkflowStatus(s string) (string, error) {
	key := wsSepRe.ReplaceAllString(strings.ToUpper(strings.TrimSpace(s)), "_")
	switch key {
	case "IN_REMEDIATION", "CLOSED":
		return key, nil
	default:
		return "", fmt.Errorf("%q is not IN_REMEDIATION or CLOSED", s)
	}
}

// mapIdentifiedByType maps the free-text "Risk Identified By" column to the
// identified_by_type enum. recognised is false for blank or anything that does
// not clearly name one of the three kinds — the caller then leaves the column
// null and records a WARN (plan §6).
func mapIdentifiedByType(s string) (typ string, recognised bool) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "":
		return "", false
	case strings.HasPrefix(v, "employee"):
		return "EMPLOYEE", true
	case strings.HasPrefix(v, "external"):
		return "EXTERNAL_PERSON", true
	case strings.HasPrefix(v, "tool"):
		return "TOOL", true
	default:
		return "", false
	}
}

// excelEpoch is Excel's day-zero for the 1900 date system (1899-12-30 — the
// 30th, not the 31st, absorbs the deliberate 1900-leap-year bug).
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

const (
	// Below excelSerialMin (1954) a bare number in a date cell is a mis-keyed
	// value (a stray year, a row number), not a serial; modern serials are 5
	// digits (2020 ≈ 43831).
	excelSerialMin = 20000
	excelSerialMax = 80000 // 2119
)

var dateLayouts = []string{
	"2006-01-02",
	"2 Jan 2006",
	"2 January 2006",
	"Jan 2 2006",
	"January 2 2006",
	"Jan 2, 2006",
	"January 2, 2006",
}

var (
	ordinalRe   = regexp.MustCompile(`(?i)(\d{1,2})(st|nd|rd|th)\b`)
	slashDateRe = regexp.MustCompile(`^\d{1,4}/\d{1,2}/\d{1,4}$`)
)

// parseDate normalises a date cell to "YYYY-MM-DD". It accepts ISO
// (2006-01-02), "2 Jan 2006" / "2nd January 2006" (ordinal suffix stripped),
// and a bare Excel serial (with or without a ".0"). A d/m/y or m/d/y slash form
// is rejected as ambiguous. Empty input returns ("", nil) — the caller decides
// whether that is a REJECT (implementation_date) or a WARN.
func parseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if slashDateRe.MatchString(s) {
		return "", fmt.Errorf("%q is an ambiguous slash date — use ISO YYYY-MM-DD", s)
	}

	cleaned := strings.Join(strings.Fields(ordinalRe.ReplaceAllString(s, "$1")), " ")
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, cleaned); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}

	if n, err := parseIntish(s); err == nil {
		if n < excelSerialMin || n > excelSerialMax {
			return "", fmt.Errorf("%q is out of range for an Excel date serial", s)
		}
		return excelEpoch.AddDate(0, 0, n).Format("2006-01-02"), nil
	}

	return "", fmt.Errorf("%q is not a recognised date", s)
}

// complianceAliases fold common spellings onto the seeded reference names
// (RefData.ComplianceIDByName keys are upper-cased). Applied after the token is
// trimmed and upper-cased.
var complianceAliases = map[string]string{
	"HIPPA":     "HIPAA",
	"SOC 2":     "SOC2",
	"PCI":       "PCI DSS",
	"PCI-DSS":   "PCI DSS",
	"PCIDSS":    "PCI DSS",
	"ISO 27001": "ISO",
	"ISO27001":  "ISO",
}

// normalizeComplianceToken trims, upper-cases and de-aliases one
// security-compliance-reference token.
func normalizeComplianceToken(s string) string {
	key := strings.ToUpper(strings.TrimSpace(s))
	if alias, ok := complianceAliases[key]; ok {
		return alias
	}
	return key
}

var httpURLRe = regexp.MustCompile(`(?i)^https?://\S`)

// scrubHTTPURL returns (trimmed, true) when s looks like an http(s) URL, and
// ("", false) otherwise — the sheet uses "Text" and similar as a placeholder.
func scrubHTTPURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if httpURLRe.MatchString(s) {
		return s, true
	}
	return "", false
}
