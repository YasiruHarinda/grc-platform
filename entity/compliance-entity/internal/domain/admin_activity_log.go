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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package domain

import "time"

// =============================================================================
// Admin Activity Log (admin_activity_log) — append-only
// =============================================================================

// AdminActivityLog is one immutable entry in the Admin Console's activity log.
type AdminActivityLog struct {
	ID int64 `json:"id"`
	// ActorID is the acting user's uuid, not user.id — resolved to a display
	// name/email live via the identity directory at read time, never
	// persisted (see the admin_activity_log table comment).
	ActorID    string  `json:"actorId"`
	Action     string  `json:"action"`     // CREATED | UPDATED | DELETED | STATUS_CHANGED | GRANTED | REVOKED
	EntityType string  `json:"entityType"` // USER | GRANT | RISK_TEAM | RISK_CATEGORY | COMPLIANCE_REFERENCE | RISK_SCORE | AUDIT_TEAM
	EntityID   int     `json:"entityId"`
	Details    *string `json:"details"` // raw JSON string
	// ActorUserType is the actor's user.user_type (INTERNAL | EXTERNAL),
	// joined from actor_id — same pattern as AuditTrail.CreatedByUserType.
	ActorUserType string    `json:"actorUserType"`
	CreatedOn     time.Time `json:"createdOn"`
}

// CreateAdminActivityLogRequest is the payload for POST /admin-activity-log.
type CreateAdminActivityLogRequest struct {
	ActorID    string  `json:"actorId"`
	Action     string  `json:"action"`
	EntityType string  `json:"entityType"`
	EntityID   int     `json:"entityId"`
	Details    *string `json:"details"`
}

// AdminActivityLogFilter narrows a GET /admin-activity-log listing. Every
// field is optional; zero-value skips that filter entirely, returning the
// whole log (subject to pagination).
type AdminActivityLogFilter struct {
	ActorID    string
	Action     string
	EntityType string
	From       *time.Time
	To         *time.Time
}

// ListAdminActivityLogResponse is returned by GET /admin-activity-log.
type ListAdminActivityLogResponse struct {
	Entries []AdminActivityLog `json:"entries"`
	Total   int                `json:"total"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
}
