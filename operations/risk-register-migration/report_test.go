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
	"strings"
	"testing"
)

func TestReportEmit_Narrative(t *testing.T) {
	r := NewReport()
	r.Add(
		Finding{MigrationID: 1, CSVRow: 2, Severity: SevReject, Failure: "Risk Owner", Value: "user1@wso2.com", Detail: `"user1@wso2.com" is not in the SCIM directory`},
		Finding{MigrationID: 3, CSVRow: 4, Severity: SevReject, Failure: "Risk Owner", Value: "User1@wso2.com"},
		Finding{MigrationID: 5, CSVRow: 6, Severity: SevReject, Failure: "Action Owner", Value: "user2@wso2.com"},
		Finding{MigrationID: 7, CSVRow: 8, Severity: SevWarn, Failure: "Git Issue URL"},
		Finding{MigrationID: 9, CSVRow: 10, Severity: SevWarn, Failure: "Git Issue URL"},
	)
	r.Migrated("IN_REMEDIATION")
	r.Migrated("IN_REMEDIATION")
	r.Migrated("CLOSED")
	r.Skipped()
	r.GrantWritten()
	r.GrantWritten()
	r.GrantWritten()
	r.SuppressingEscalation(2)
	r.SuppressingEscalation(1)

	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()

	must := []string{
		"migrated=3  skipped(resume)=1  rejected=3  warnings=2",
		"----- errors.csv -----",
		"----- report.txt -----",
		"findings by code:",
		"2  REJECT  Risk Owner",
		"2  WARN  Git Issue URL",
		"unresolved people (2):", // user1@ (case-folded) + user2@
		"user1@wso2.com",
		"user2@wso2.com",
		"suppressing escalations seeded for Migration IDs: [1 2] (2)",
		"grants written: 3",
		"migrated by bucket: CLOSED=1 IN_REMEDIATION=2",
	}
	for _, m := range must {
		if !strings.Contains(out, m) {
			t.Errorf("report missing %q\n---\n%s", m, out)
		}
	}
	if strings.Contains(out, "User1@wso2.com") {
		t.Errorf("unresolved-people list should be case-folded and de-duped:\n%s", out)
	}
}

func TestReportEmit_CleanRunHasNoNarrativeClutter(t *testing.T) {
	r := NewReport()
	r.Migrated("IN_REMEDIATION")

	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()

	if strings.Contains(out, "unresolved people") || strings.Contains(out, "findings by code") {
		t.Errorf("a clean run should not print empty finding sections:\n%s", out)
	}
	if !strings.Contains(out, "migrated by bucket: IN_REMEDIATION=1") {
		t.Errorf("bucket line missing:\n%s", out)
	}
}
