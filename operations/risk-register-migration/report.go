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
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Severity of a Finding (plan §9).
type Severity string

const (
	SevReject Severity = "REJECT" // the row is not migrated
	SevWarn   Severity = "WARN"   // the row is migrated, with a caveat
)

// Finding is one row of the errors.csv block.
type Finding struct {
	MigrationID int
	CSVRow      int
	RiskTitle   string
	Severity    Severity
	Failure     string // short code, e.g. "Risk Owner", "Implementation Date"
	Detail      string
	// Value is the raw offending token when there is one — an unresolvable
	// email, an unknown reference. Feeds the report's "unresolved people" list.
	Value string
}

// personColumns are the Failure codes whose Value is an unresolvable email.
var personColumns = map[string]bool{
	"Risk Owner": true, "Risk Assigned To": true,
	"Management Approver": true, "Action Owner": true,
}

// Report accumulates findings and per-row outcomes for the run, then renders
// the errors.csv + report.txt blocks to the log (plan §9).
type Report struct {
	findings []Finding
	migrated int
	skipped  int // already complete on a resume

	migratedByBucket       map[string]int
	grantsWritten          int
	suppressingEscalations []int // Migration IDs that got a D8 suppressing escalation
}

func NewReport() *Report { return &Report{migratedByBucket: map[string]int{}} }

func (r *Report) Add(fs ...Finding) { r.findings = append(r.findings, fs...) }

// Migrated records one row written to completion, tallied by target bucket
// (IN_REMEDIATION | CLOSED).
func (r *Report) Migrated(bucket string) {
	r.migrated++
	r.migratedByBucket[bucket]++
}

func (r *Report) Skipped() { r.skipped++ }

func (r *Report) GrantWritten() { r.grantsWritten++ }

// SuppressingEscalation records that migrationID's risk got the D8 escalation
// that keeps the nightly job (and its emails) off an already-overdue import.
func (r *Report) SuppressingEscalation(migrationID int) {
	r.suppressingEscalations = append(r.suppressingEscalations, migrationID)
}

func (r *Report) HasFindings() bool { return len(r.findings) > 0 }

// RejectedMigrationIDs is the set of Migration IDs with at least one REJECT.
func (r *Report) RejectedMigrationIDs() map[int]struct{} {
	out := map[int]struct{}{}
	for _, f := range r.findings {
		if f.Severity == SevReject {
			out[f.MigrationID] = struct{}{}
		}
	}
	return out
}

// Emit writes the two blocks the operator reads out of the Choreo logs.
func (r *Report) Emit(w io.Writer) {
	fmt.Fprintf(w, "\n===== risk-register-import report — %s =====\n", time.Now().UTC().Format(time.RFC3339))

	rejects, warns := 0, 0
	for _, f := range r.findings {
		switch f.Severity {
		case SevReject:
			rejects++
		case SevWarn:
			warns++
		}
	}
	fmt.Fprintf(w, "migrated=%d  skipped(resume)=%d  rejected=%d  warnings=%d\n\n", r.migrated, r.skipped, rejects, warns)

	fmt.Fprintln(w, "----- errors.csv -----")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"migration_id", "csv_row", "risk_title", "severity", "failure", "detail"})
	sorted := append([]Finding(nil), r.findings...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].MigrationID != sorted[j].MigrationID {
			return sorted[i].MigrationID < sorted[j].MigrationID
		}
		return sorted[i].CSVRow < sorted[j].CSVRow
	})
	for _, f := range sorted {
		_ = cw.Write([]string{
			strconv.Itoa(f.MigrationID), strconv.Itoa(f.CSVRow), f.RiskTitle, string(f.Severity), f.Failure, f.Detail,
		})
	}
	cw.Flush()

	r.emitNarrative(w)
	fmt.Fprintln(w, "----- end report -----")
}

// emitNarrative is the report.txt block (plan §9): a failure-code breakdown,
// the distinct unresolved people, the rows that got a suppressing escalation,
// the grant count, and — on a real run — the per-bucket migrated counts.
func (r *Report) emitNarrative(w io.Writer) {
	fmt.Fprintln(w, "\n----- report.txt -----")

	// Failure-code counts, most frequent first.
	counts := map[string]int{}
	for _, f := range r.findings {
		counts[string(f.Severity)+"  "+f.Failure]++
	}
	if len(counts) > 0 {
		type kv struct {
			k string
			n int
		}
		rows := make([]kv, 0, len(counts))
		for k, n := range counts {
			rows = append(rows, kv{k, n})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].n != rows[j].n {
				return rows[i].n > rows[j].n
			}
			return rows[i].k < rows[j].k
		})
		fmt.Fprintln(w, "findings by code:")
		for _, row := range rows {
			fmt.Fprintf(w, "  %4d  %s\n", row.n, row.k)
		}
	}

	// Distinct unresolved people.
	emails := map[string]struct{}{}
	for _, f := range r.findings {
		if personColumns[f.Failure] && f.Value != "" {
			emails[strings.ToLower(f.Value)] = struct{}{}
		}
	}
	if len(emails) > 0 {
		list := make([]string, 0, len(emails))
		for e := range emails {
			list = append(list, e)
		}
		sort.Strings(list)
		fmt.Fprintf(w, "unresolved people (%d):\n", len(list))
		for _, e := range list {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}

	if len(r.suppressingEscalations) > 0 {
		ids := append([]int(nil), r.suppressingEscalations...)
		sort.Ints(ids)
		fmt.Fprintf(w, "suppressing escalations seeded for Migration IDs: %v (%d)\n", ids, len(ids))
	}

	if r.grantsWritten > 0 {
		fmt.Fprintf(w, "grants written: %d\n", r.grantsWritten)
	}

	if r.migrated > 0 {
		buckets := make([]string, 0, len(r.migratedByBucket))
		for b := range r.migratedByBucket {
			buckets = append(buckets, b)
		}
		sort.Strings(buckets)
		fmt.Fprint(w, "migrated by bucket:")
		for _, b := range buckets {
			fmt.Fprintf(w, " %s=%d", b, r.migratedByBucket[b])
		}
		fmt.Fprintln(w)
	}
}
