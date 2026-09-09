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

import "testing"

func TestParseIntish(t *testing.T) {
	ok := map[string]int{"2025.0": 2025, "3": 3, " 2 ": 2, "3.0": 3, "-1": -1, "45667.0": 45667}
	for in, want := range ok {
		if got, err := parseIntish(in); err != nil || got != want {
			t.Errorf("parseIntish(%q) = (%d, %v), want (%d, nil)", in, got, err, want)
		}
	}
	// ParseFloat accepts these; each must still be an error here (int(f) on a
	// non-finite or out-of-range value is implementation-defined garbage).
	for _, in := range []string{"", "abc", "2025.5", "3,000", "Q1", "Inf", "-Inf", "NaN", "1e30", "-1e30", "0x1p64"} {
		if got, err := parseIntish(in); err == nil {
			t.Errorf("parseIntish(%q) = (%d, nil): want error", in, got)
		}
	}
}

func TestParseYearAndScore(t *testing.T) {
	if y, err := parseYear("2025.0"); err != nil || y != 2025 {
		t.Errorf("parseYear(2025.0) = (%d, %v)", y, err)
	}
	for _, in := range []string{"1999", "2101", "45000", "x"} {
		if _, err := parseYear(in); err == nil {
			t.Errorf("parseYear(%q): want error", in)
		}
	}
	for in, want := range map[string]int{"1.0": 1, "2": 2, "3.0": 3} {
		if s, err := parseScore(in); err != nil || s != want {
			t.Errorf("parseScore(%q) = (%d, %v)", in, s, err)
		}
	}
	for _, in := range []string{"0", "4", "5.0", "", "two"} {
		if _, err := parseScore(in); err == nil {
			t.Errorf("parseScore(%q): want error", in)
		}
	}
}

func TestParseQuarter(t *testing.T) {
	for in, want := range map[string]string{"Q1": "Q1", "q3": "Q3", " Q4 ": "Q4"} {
		if got, err := parseQuarter(in); err != nil || got != want {
			t.Errorf("parseQuarter(%q) = (%q, %v)", in, got, err)
		}
	}
	for _, in := range []string{"", "Q0", "Q5", "3", "first"} {
		if _, err := parseQuarter(in); err == nil {
			t.Errorf("parseQuarter(%q): want error", in)
		}
	}
}

func TestMapTreatment(t *testing.T) {
	for in, want := range map[string]string{
		"Accept": "ACCEPT", "transfer": "TRANSFER", " Remediate ": "REMEDIATE",
		"Avoid": "AVOID", "Void": "AVOID",
	} {
		if got, err := mapTreatment(in); err != nil || got != want {
			t.Errorf("mapTreatment(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", "Mitigate", "accepted"} {
		if _, err := mapTreatment(in); err == nil {
			t.Errorf("mapTreatment(%q): want error (blank must reject)", in)
		}
	}
}

func TestMapWorkflowStatus(t *testing.T) {
	for in, want := range map[string]string{
		"IN_REMEDIATION": "IN_REMEDIATION", "in remediation": "IN_REMEDIATION",
		"In-Remediation": "IN_REMEDIATION", "Closed": "CLOSED", " CLOSED ": "CLOSED",
	} {
		if got, err := mapWorkflowStatus(in); err != nil || got != want {
			t.Errorf("mapWorkflowStatus(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "PENDING", "DRAFT", "CANCELLED", "REMEDIATION"} {
		if _, err := mapWorkflowStatus(in); err == nil {
			t.Errorf("mapWorkflowStatus(%q): want error", in)
		}
	}
}

func TestMapIdentifiedByType(t *testing.T) {
	cases := []struct {
		in    string
		typ   string
		recog bool
	}{
		{"Employee", "EMPLOYEE", true},
		{"employee (intern)", "EMPLOYEE", true},
		{"External Person", "EXTERNAL_PERSON", true},
		{"External vendor", "EXTERNAL_PERSON", true},
		{"Tool", "TOOL", true},
		{"tool: scanner", "TOOL", true},
		{"", "", false},
		{"Robot", "", false},
	}
	for _, c := range cases {
		typ, recog := mapIdentifiedByType(c.in)
		if typ != c.typ || recog != c.recog {
			t.Errorf("mapIdentifiedByType(%q) = (%q, %v), want (%q, %v)", c.in, typ, recog, c.typ, c.recog)
		}
	}
}

func TestParseDate(t *testing.T) {
	ok := map[string]string{
		"":                 "",
		"2025-06-30":       "2025-06-30",
		" 2025-06-30 ":     "2025-06-30",
		"30th Sep 2025":    "2025-09-30",
		"2 Jan 2026":       "2026-01-02",
		"1st January 2025": "2025-01-01",
		"45667.0":          "2025-01-10", // Excel serial with .0
		"45838":            "2025-06-30",
	}
	for in, want := range ok {
		if got, err := parseDate(in); err != nil || got != want {
			t.Errorf("parseDate(%q) = (%q, %v), want (%q, nil)", in, got, err, want)
		}
	}

	for _, in := range []string{
		"1/2/2024",   // ambiguous d/m/y vs m/d/y
		"2024/01/02", // slash form rejected outright
		"31-13-2020", // not a real date, not a layout
		"2025",       // bare year — out of serial range
		"soon",       // free text
		"12345",      // below serial floor
	} {
		if got, err := parseDate(in); err == nil {
			t.Errorf("parseDate(%q) = (%q, nil), want error", in, got)
		}
	}
}

func TestNormalizeComplianceToken(t *testing.T) {
	for in, want := range map[string]string{
		"iso":       "ISO",
		"ISO 27001": "ISO",
		"ISO27001":  "ISO",
		"hippa":     "HIPAA",
		"SOC 2":     "SOC2",
		"soc2":      "SOC2",
		"pci":       "PCI DSS",
		"PCI-DSS":   "PCI DSS",
		"security":  "SECURITY",
	} {
		if got := normalizeComplianceToken(in); got != want {
			t.Errorf("normalizeComplianceToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrubHTTPURL(t *testing.T) {
	for _, in := range []string{"https://github.com/o/r/issues/1", " http://x.test/a ", "HTTPS://X"} {
		if got, ok := scrubHTTPURL(in); !ok || got == "" {
			t.Errorf("scrubHTTPURL(%q) = (%q, %v), want a kept URL", in, got, ok)
		}
	}
	for _, in := range []string{"", "Text", "github.com/x", "ftp://x", "n/a"} {
		if got, ok := scrubHTTPURL(in); ok || got != "" {
			t.Errorf("scrubHTTPURL(%q) = (%q, %v), want ('', false)", in, got, ok)
		}
	}
}
