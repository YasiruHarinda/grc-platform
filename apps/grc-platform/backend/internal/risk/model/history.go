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

import (
	"encoding/json"
	"time"
)

// History actions, mirroring risk_change_log.action in risk_schema.sql.
//
// A row is either a field diff (FieldChanged + OldValue/NewValue) or an event (Details).
//
// Note: CREATE is used for both risk creation and action-plan creation; the latter
// sets Details.Plan, so callers must not assume CREATE implies a field diff.
// Same split the Audit Hub's audit_trail uses. Adding one here means adding it
// to the schema ENUM and the entity's validChangeLogActions too, or the write
// is rejected.
const (
	HistoryCreate   = "CREATE"
	HistoryUpdate   = "UPDATE"
	HistoryDelete   = "DELETE"
	HistorySubmit   = "SUBMIT"
	HistoryApprove  = "APPROVE"
	HistoryReject   = "REJECT"
	HistoryEscalate = "ESCALATE"
	HistoryComment  = "COMMENT"
	HistoryAssess   = "ASSESS"
	HistoryComplete = "COMPLETE"
	HistoryClose    = "CLOSE"
	HistoryCancel   = "CANCEL"
)

// HistoryEntry is one event in a risk's history, as surfaced to the web app.
// Details is passed through untouched for the client to render — the shape
// varies by action (see HistoryDetails).
type HistoryEntry struct {
	ID           int64           `json:"id"`
	RiskID       int             `json:"risk_id"`
	Action       string          `json:"action"`
	FieldChanged *string         `json:"field_changed"`
	OldValue     *string         `json:"old_value"`
	NewValue     *string         `json:"new_value"`
	Details      json.RawMessage `json:"details,omitempty"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
}

// HistoryDetails is the payload recorded alongside a workflow event. Every
// field is optional — an ESCALATE carries only OverdueDays, an APPROVE carries
// From/To/Role — so it marshals to the smallest object that says what happened.
type HistoryDetails struct {
	// From/To are workflow statuses, for any action that moves the risk.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Role names the capacity the actor acted in ("Risk Owner", "Compliance"),
	// which the status pair alone doesn't convey — an owner and a management
	// approval can both land on PENDING_COMPLIANCE_REVIEW.
	Role string `json:"role,omitempty"`
	// Comment carries a rejection reason or an escalation review comment.
	Comment string `json:"comment,omitempty"`
	// Stage is the rejection_stage recorded on a REJECT.
	Stage string `json:"stage,omitempty"`
	// Level/PreviousLevel describe a reassessment's residual outcome.
	Level         string `json:"level,omitempty"`
	PreviousLevel string `json:"previousLevel,omitempty"`
	// OverdueDays is how far past its implementation date an ESCALATE was.
	OverdueDays int `json:"overdueDays,omitempty"`
	// Plan names the action plan an ACTION-plan event concerns.
	Plan string `json:"plan,omitempty"`
}

// RecordHistoryRequest is what the handler hands the service to append an entry.
type RecordHistoryRequest struct {
	Action       string
	FieldChanged *string
	OldValue     *string
	NewValue     *string
	Details      *HistoryDetails
}
