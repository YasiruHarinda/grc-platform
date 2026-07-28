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

// AuditTrailEntry is one immutable event in a control's history, as surfaced to
// the web app. Action is one of the audit_trail enum values (CREATED, UPLOADED,
// RESUBMITTED, APPROVED, REJECTED, COMMENTED, ESCALATED, AI_VALIDATED, EXPORTED,
// UPDATED, DELETED). Details carries the raw JSON recorded with the event (e.g.
// {"from","to","via"}) for the client to render; it is passed through untouched.
// ControlID is nil for audit-level events (audit CREATED/UPDATED/DELETED).
type AuditTrailEntry struct {
	ID         int64           `json:"id"`
	Action     string          `json:"action"`
	ControlID  *int            `json:"controlId"`
	EvidenceID *int            `json:"evidenceId"`
	CreatedBy  string          `json:"createdBy"`
	CreatedAt  time.Time       `json:"createdAt"`
	Details    json.RawMessage `json:"details,omitempty"`
}

// TrailListResponse is the envelope for GET .../trail.
type TrailListResponse struct {
	Items []*AuditTrailEntry `json:"items"`
	Total int                `json:"total"`
}

// TrailFilter narrows an audit-wide trail listing. ControlIDs empty means "don't
// filter on this" — multiple values are OR'd, matching the Activity Log page's
// Control column filter (checkbox popover).
type TrailFilter struct {
	ControlIDs []int
	From       *time.Time
	To         *time.Time
}
