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

package model

// ReminderItem is one control or population line queued into an owner's
// daily due-date reminder digest — both what the email should display and
// what the send-log needs to record it (for de-dup on future runs). Lives in
// this package (not internal/audit/job or internal/audit/handler) so both
// can share it without either importing the other: internal/audit/job builds
// these while sweeping controls, and internal/audit/handler.Deps.
// SendReminderDigestSync (the function job.ReminderJob is wired to at
// startup, exactly as internal/risk/job.EscalationJob is wired to
// NotifyEscalationSync) turns them into one combined email plus one
// audit_notification row per item.
type ReminderItem struct {
	AuditID      int
	ControlID    *int
	PopulationID *int
	// Type is the audit_notification.type value for this item's tier:
	// REMINDER_DUE_10 | REMINDER_DUE_5 | REMINDER_OVERDUE.
	Type string
	// ControlNumber/Description/DueDate/Tier/RequirementType mirror
	// emailer.AuditEventItem's display fields.
	ControlNumber   string
	Description     string
	DueDate         string
	Tier            string
	RequirementType string // "Evidence Requirement" | "Population Requirement"
	// DedupSnapshot is the date written to audit_notification.due_date_snapshot
	// for this item's log row — distinct from DueDate (which is always the
	// item's real due date, for display). For the DUE_10/DUE_5 tiers it's the
	// same value as DueDate, so each tier logs (and therefore fires) exactly
	// once per due date. For OVERDUE it is instead the date the reminder was
	// sent, so — unlike the other two tiers — an overdue item logs a new row,
	// and therefore re-sends, every day it stays overdue.
	DedupSnapshot string
	// NotificationID is the audit_notification row ID this item's claim
	// reserved — set by internal/audit/job.ReminderJob's queue closure once
	// job.claimer.Claim succeeds, and used only by that same job to release
	// the claim (delete the row) if the owner's digest send then fails, so
	// the item is retried on a future run instead of staying claimed forever
	// with nothing sent. See docs/new/Reminder-Notification-Atomic-Claim-Design.md.
	NotificationID int64
}
