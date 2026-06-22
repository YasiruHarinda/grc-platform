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

// Package privilege loads role→privilege mappings from the database at startup
// and provides context helpers for per-request privilege resolution.
//
// Privilege names here must exactly match the privilege_name values seeded in
// the privilege table. Roles are never referenced in application code — only
// privilege names appear in handler-level checks.
package privilege

import (
	"context"
	"database/sql"
	"fmt"
)

// Risk Hub privilege names.
const (
	ViewRisks            = "VIEW_RISKS"
	CreateRisk           = "CREATE_RISK"
	UpdateRisk           = "UPDATE_RISK"
	SubmitRisk           = "SUBMIT_RISK"
	ApproveRisk          = "APPROVE_RISK"
	RejectRisk           = "REJECT_RISK"
	CompleteRisk         = "COMPLETE_RISK"
	OwnerApproveRisk     = "OWNER_APPROVE_RISK"
	CloseRisk            = "CLOSE_RISK"
	EscalateRisk         = "ESCALATE_RISK"
	AssessRisk           = "ASSESS_RISK"
	ManageTeams          = "MANAGE_TEAMS"
	ManageRiskScores     = "MANAGE_RISK_SCORES"
	ManageActionPlans    = "MANAGE_ACTION_PLANS"
	ManageComplianceRefs = "MANAGE_COMPLIANCE_REFS"
	ViewAnalytics        = "VIEW_ANALYTICS"
)

// Audit Hub privilege names.
const (
	ViewAudits           = "VIEW_AUDITS"
	CreateAudit          = "CREATE_AUDIT"
	UpdateAudit          = "UPDATE_AUDIT"
	MoveAuditToFieldwork = "MOVE_AUDIT_TO_FIELDWORK"
	SubmitAuditForReview = "SUBMIT_AUDIT_FOR_REVIEW"
	CompleteAudit        = "COMPLETE_AUDIT"
	ManageControls       = "MANAGE_CONTROLS"
	SubmitEvidence       = "SUBMIT_EVIDENCE"
	ReviewEvidence       = "REVIEW_EVIDENCE"
	ManagePopulation     = "MANAGE_POPULATION"
	AddComment           = "ADD_COMMENT"
	ManageAssignments    = "MANAGE_ASSIGNMENTS"
	ViewTrail            = "VIEW_TRAIL"
	ManageFrameworks     = "MANAGE_FRAMEWORKS"
)

type contextKey struct{}

// Store holds the role→privilege mapping loaded from the database at startup.
// It is safe for concurrent reads after construction.
type Store struct {
	// rolePrivileges maps role_name → set of privilege_names.
	rolePrivileges map[string]map[string]bool
}

// New loads the active role→privilege mapping from the database and returns a Store.
// Call once at startup; pass the result to middleware.Config.PrivilegeStore.
func New(ctx context.Context, db *sql.DB) (*Store, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.role_name, p.privilege_name
		FROM role_privilege rp
		JOIN role r ON r.id = rp.role_id
		JOIN privilege p ON p.id = rp.privilege_id
		WHERE rp.is_active = TRUE
		  AND r.status = 'ACTIVE'
		  AND p.status = 'ACTIVE'
	`)
	if err != nil {
		return nil, fmt.Errorf("privilege: load mapping: %w", err)
	}
	defer rows.Close()

	m := make(map[string]map[string]bool)
	for rows.Next() {
		var role, priv string
		if err := rows.Scan(&role, &priv); err != nil {
			return nil, fmt.Errorf("privilege: scan row: %w", err)
		}
		if m[role] == nil {
			m[role] = make(map[string]bool)
		}
		m[role][priv] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("privilege: iterate rows: %w", err)
	}
	return &Store{rolePrivileges: m}, nil
}

// Resolve returns the union of all privileges granted to any of the given roles.
func (s *Store) Resolve(roles []string) map[string]bool {
	result := make(map[string]bool)
	for _, role := range roles {
		for priv := range s.rolePrivileges[role] {
			result[priv] = true
		}
	}
	return result
}

// WithContext stores the resolved privilege set in the context.
// Called by the auth middleware after resolving the user's roles.
func WithContext(ctx context.Context, privs map[string]bool) context.Context {
	return context.WithValue(ctx, contextKey{}, privs)
}

// FromContext retrieves the privilege set from the context.
// Returns nil when no privilege store was configured (local dev — allow-all mode).
func FromContext(ctx context.Context) map[string]bool {
	v, _ := ctx.Value(contextKey{}).(map[string]bool)
	return v
}
