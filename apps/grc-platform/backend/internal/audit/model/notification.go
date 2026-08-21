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

// Package model defines the domain types for the Audit Hub module.
package model

// NotificationLogEntry is one row written to audit_notification after a
// successful email send — the send-log for every audit-module notification,
// and the daily reminder job's de-dup mechanism (see audit_schema.sql's
// audit_notification comment). Exactly one of ControlID/PopulationID is set.
type NotificationLogEntry struct {
	RecipientID  int
	AuditID      *int
	ControlID    *int
	PopulationID *int
	// Type matches one of the audit_notification.type ENUM values (mirrors
	// the log-type strings derived from emailer.AuditEvent — see notify.go).
	Type string
	// DueDateSnapshot freezes the due_date a reminder was computed against,
	// so editing a control/population's due_date later correctly restarts
	// its reminder cycle. Only set for REMINDER_* types.
	DueDateSnapshot *string
	Message         *string
}
