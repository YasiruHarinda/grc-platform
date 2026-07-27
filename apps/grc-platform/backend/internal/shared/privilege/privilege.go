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

// Package privilege loads role→privilege mappings from the Compliance Entity
// and keeps them current with a periodic refresh (every 15 min), matching the
// JWKS cache refresh cadence. Revoked roles or privileges take effect within
// one window without requiring a redeploy.
//
// Privilege names here must exactly match the privilege_name values seeded in
// the privilege table. Roles are never referenced in application code — only
// privilege names appear in handler-level checks.
package privilege

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// Risk Hub privilege names. All prefixed RISK_ so they group together
// (visually and alphabetically) apart from the Audit Hub block below.
const (
	// ViewRisks gates the Registers list (GET /risks) and single risk detail
	// (GET /risks/{id}).
	ViewRisks = "RISK_VIEW_RISKS"

	// CreateRisk gates registering a new risk (POST /risks) — creation and
	// initial submission into the approval workflow happen in one step.
	CreateRisk = "RISK_CREATE"

	// UpdateRisk gates editing an existing risk (PUT /risks/{id}).
	UpdateRisk = "RISK_UPDATE"

	// SubmitRisk gates resubmitting a rejected risk (POST /risks/{id}/resubmit)
	// — not the initial submission, which is bundled into CreateRisk.
	SubmitRisk = "RISK_SUBMIT"

	// CancelRisk gates cancelling (soft-deleting) a risk.
	CancelRisk = "RISK_CANCEL"

	// OwnerApproveRisk gates the Risk Owner's approval at the owner-approval
	// workflow stage (initial submission or post-remediation completion).
	OwnerApproveRisk = "RISK_OWNER_APPROVE"

	// ManagementApproveRisk gates Management's approval at the
	// PENDING_MANAGEMENT_APPROVAL stage (Accept+HIGH escalation path).
	ManagementApproveRisk = "RISK_MANAGEMENT_APPROVE"

	// ComplianceApproveRisk gates Compliance's approval at the
	// PENDING_COMPLIANCE_REVIEW stage.
	ComplianceApproveRisk = "RISK_COMPLIANCE_APPROVE"

	// OwnerRejectRisk gates the Risk Owner's rejection, sending the risk to
	// PENDING_REVISION.
	OwnerRejectRisk = "RISK_OWNER_REJECT"

	// ManagementRejectRisk gates Management's rejection at the management
	// approval stage.
	ManagementRejectRisk = "RISK_MANAGEMENT_REJECT"

	// ComplianceRejectRisk gates Compliance's rejection at the compliance
	// review stage.
	ComplianceRejectRisk = "RISK_COMPLIANCE_REJECT"

	// CompleteRisk gates submitting action-plan completion for Risk Owner
	// sign-off (IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL).
	CompleteRisk = "RISK_COMPLETE"

	// CloseRisk gates Compliance's final closure of a risk after completion
	// is approved.
	CloseRisk = "RISK_CLOSE"

	// EscalateRisk gates escalating a risk to Management, manually or via the
	// daily overdue-risk job.
	EscalateRisk = "RISK_ESCALATE"

	// AssessRisk gates recording a residual reassessment on an IN_REMEDIATION
	// risk (new score, progress notes, next reassessment date).
	AssessRisk = "RISK_ASSESS"

	// ManageTeams is reserved for a future risk-team admin endpoint
	// (create/edit/deactivate risk teams) — not yet wired to a handler; today
	// it's only used as a "sees everything" classification signal in
	// risk_registers.go.
	ManageTeams = "RISK_MANAGE_TEAMS"

	// ManageRiskScores is reserved for a future risk-score-matrix admin
	// endpoint — not yet wired to a handler; same classification-only role as
	// ManageTeams above.
	ManageRiskScores = "RISK_MANAGE_SCORES"

	// ManageActionPlans is reserved for editing a STANDARD action plan/its
	// steps outside of risk creation — not yet wired to a handler (STANDARD
	// plans are created inline as part of CreateRisk); today it's only used
	// as a classification signal, same as ManageTeams above.
	ManageActionPlans = "RISK_MANAGE_ACTION_PLANS"

	// ManageComplianceRefs is reserved for a future compliance-reference admin
	// endpoint (the framework tags a risk can be linked against) — not yet
	// wired to a handler; same classification-only role as ManageTeams above.
	ManageComplianceRefs = "RISK_MANAGE_COMPLIANCE_REFS"

	// ViewAnalytics gates the Analytics page.
	ViewAnalytics = "RISK_VIEW_ANALYTICS"

	// ViewRiskDashboard gates the Dashboard nav item/route specifically —
	// distinct from ViewRisks (which gates the Registers list) so an Action
	// Owner can hold list access without also getting the org-wide dashboard.
	ViewRiskDashboard = "RISK_VIEW_DASHBOARD"

	// CreateManagementActionPlan gates creating a plan_type=MANAGEMENT action
	// plan on an ESCALATED risk — distinct from ManageActionPlans (which
	// Risk Assigners and Admin already hold for STANDARD plans) so Management
	// can create MANAGEMENT plans without also being able to touch STANDARD ones.
	CreateManagementActionPlan = "RISK_CREATE_MANAGEMENT_ACTION_PLAN"

	// CompleteActionSteps gates viewing/completing the steps of a plan the
	// caller is action_owner_id on — applies uniformly to STANDARD and
	// MANAGEMENT plans; ownership is checked separately at the handler/service
	// layer, this privilege alone does not grant access to every plan.
	CompleteActionSteps = "RISK_COMPLETE_ACTION_STEPS"
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
	ManageUsers          = "MANAGE_USERS"
	ExportReport         = "EXPORT_REPORT"
)

type contextKey struct{}

// Store holds the role→privilege mapping and refreshes it periodically from the
// database. Safe for concurrent reads at all times.
type Store struct {
	mu             sync.RWMutex
	rolePrivileges map[string]map[string]bool
	client         *entityclient.Client
}

// NewForTest constructs a Store with a pre-populated mapping without a database.
// For unit tests only — never call in production code.
func NewForTest(rolePrivileges map[string]map[string]bool) *Store {
	return &Store{rolePrivileges: rolePrivileges}
}

// initialLoadAttempts and initialLoadBackoff bound the retry on the first load.
// The entity and the backend are usually started together, so a first attempt
// can lose a race with the entity's own startup by a second or two; without a
// retry that race is a hard failure needing an orchestrator restart.
// initialLoadTimeout caps the whole retry loop, so it must outlast
// attempts × backoff.
const (
	initialLoadAttempts = 5
	initialLoadBackoff  = 2 * time.Second
	initialLoadTimeout  = 30 * time.Second
)

// New loads the active role→privilege mapping from the Compliance Entity,
// starts a background goroutine that reloads it every 15 minutes, and returns
// the Store. The goroutine stops when ctx is cancelled (typically at server
// shutdown), so callers must pass an application-lifetime context — the
// initial load is bounded separately by initialLoadTimeout, derived from ctx
// here rather than imposed by the caller. Passing a short-lived context would
// otherwise kill the periodic refresh once its deadline elapsed.
//
// A failure to load is fatal, and deliberately so: with no mapping every
// authorisation check is unanswerable, and a server that starts in that state
// would deny legitimate users with 403s that look like a permissions bug rather
// than an outage. Failing to boot is the louder, more diagnosable failure. The
// retry below absorbs a startup race without weakening that guarantee.
func New(ctx context.Context, client *entityclient.Client) (*Store, error) {
	s := &Store{client: client}

	loadCtx, cancelLoad := context.WithTimeout(ctx, initialLoadTimeout)
	defer cancelLoad()

	var err error
	for attempt := 1; attempt <= initialLoadAttempts; attempt++ {
		if err = s.reload(loadCtx); err == nil {
			break
		}
		if attempt == initialLoadAttempts {
			return nil, fmt.Errorf("privilege: initial load failed after %d attempts: %w", attempt, err)
		}
		slog.Warn("privilege: initial load failed, retrying",
			"attempt", attempt, "of", initialLoadAttempts, "err", err)
		select {
		case <-loadCtx.Done():
			return nil, loadCtx.Err()
		case <-time.After(initialLoadBackoff):
		}
	}
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := s.reload(ctx); err != nil {
					slog.Error("privilege: reload failed", "err", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return s, nil
}

// reload fetches the current role→privilege mapping from the Compliance Entity
// and atomically replaces the in-memory map under the write lock. On failure
// the previous map is left in place, so a transient entity outage degrades to
// stale-but-working authorisation rather than denying everything.
func (s *Store) reload(ctx context.Context) error {
	var resp struct {
		RolePrivileges map[string][]string `json:"rolePrivileges"`
	}
	if err := s.client.Get(ctx, "/role-privileges", &resp); err != nil {
		return fmt.Errorf("privilege: load mapping: %w", err)
	}

	m := make(map[string]map[string]bool, len(resp.RolePrivileges))
	for role, privs := range resp.RolePrivileges {
		set := make(map[string]bool, len(privs))
		for _, p := range privs {
			set[p] = true
		}
		m[role] = set
	}

	s.mu.Lock()
	s.rolePrivileges = m
	s.mu.Unlock()
	slog.Info("privilege: map reloaded", "roles", len(m))
	return nil
}

// Resolve returns the union of all privileges granted to any of the given roles.
func (s *Store) Resolve(roles []string) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
