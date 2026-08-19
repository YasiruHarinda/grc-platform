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

package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// EscalationService defines business operations for risk escalations.
type EscalationService interface {
	List(ctx context.Context, riskID int) ([]*model.Escalation, error)
	// Escalate resolves the assigner's and action owner's line managers from
	// the HR entity and freezes them on the escalation row.
	Escalate(ctx context.Context, riskID int, createdBy string) (*model.Escalation, error)
	// Comment records the management/lead comment and returns the risk to its
	// assigner (ESCALATED → IN_REMEDIATION). callerEmail is checked against the
	// people entitled to comment on this risk; canOverride lets a compliance
	// admin comment regardless.
	Comment(ctx context.Context, riskID, escalationID int, comment, callerEmail string, canOverride bool) (*model.Escalation, error)
	// ResolveOpen closes every open escalation on a risk, dropping it out of
	// the Overdue tab.
	ResolveOpen(ctx context.Context, riskID int, updatedBy string) error
}

type escalationService struct {
	repo     repository.EscalationRepository
	riskRepo repository.RiskRepository
	planRepo repository.ActionPlanRepository
	users    userentity.Repository
	hr       *hrentity.Client
	// scim resolves a lead's email to their Asgardeo id, so authorizeComment
	// can compare a caller by uuid instead of email. May be nil (unconfigured
	// local dev, matching hr's own nil-tolerant pattern) — managerOf then
	// records the email only, same as before this uuid resolution existed.
	scim *scim.Client
}

// NewEscalationService wires the escalation service. hr may be nil in tests and
// in any deployment without HR credentials — lead resolution then yields no
// leads, which degrades the medium/low comment gate to compliance-admin only
// rather than failing escalation outright. scim may independently be nil —
// see the struct field comment.
func NewEscalationService(
	repo repository.EscalationRepository,
	riskRepo repository.RiskRepository,
	planRepo repository.ActionPlanRepository,
	users userentity.Repository,
	hr *hrentity.Client,
	scimClient *scim.Client,
) EscalationService {
	return &escalationService{repo: repo, riskRepo: riskRepo, planRepo: planRepo, users: users, hr: hr, scim: scimClient}
}

func (s *escalationService) List(ctx context.Context, riskID int) ([]*model.Escalation, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	return s.repo.List(ctx, riskID)
}

func (s *escalationService) Escalate(ctx context.Context, riskID int, createdBy string) (*model.Escalation, error) {
	if riskID <= 0 {
		return nil, badRequest("riskId must be a positive integer")
	}
	if createdBy == "" {
		return nil, badRequest("createdBy is required")
	}
	assignerLead, actionOwnerLead := s.resolveLeads(ctx, riskID)
	return s.repo.Escalate(ctx, riskID, createdBy,
		assignerLead.email, actionOwnerLead.email, assignerLead.uuid, actionOwnerLead.uuid)
}

// escalationLead is one person resolved by resolveLeads/managerOf: their email
// (from HR entity) and, where resolvable, their Asgardeo id (from the identity
// directory). Both are frozen onto the escalation row — see
// domain.RiskEscalation's field comment on the entity side for why holding
// onto both, and why neither is ever re-resolved later.
type escalationLead struct{ email, uuid *string }

// resolveLeads looks up the line managers of the risk's assigner and of its
// action plan owner(s), once, so they can be frozen on the escalation row.
//
// Every failure is soft: a missing lead is ordinary (the org chart has roots,
// and joiners are sometimes unassigned), and an HR outage must not block an
// escalation that is already overdue. A fully-unresolved lead simply means
// nobody is granted that lead's comment rights or visibility.
func (s *escalationService) resolveLeads(ctx context.Context, riskID int) (assignerLead, actionOwnerLead escalationLead) {
	if s.hr == nil {
		return escalationLead{}, escalationLead{}
	}
	detail, err := s.riskRepo.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("escalation: failed to load risk for lead resolution", "riskId", riskID, "err", err)
		return escalationLead{}, escalationLead{}
	}
	assignerLead = s.managerOf(ctx, detail.AssignerID)

	// Only the first action owner's lead is recorded — the column is singular,
	// and a risk with several plans is the exception rather than the rule.
	plans, err := s.planRepo.List(ctx, riskID)
	if err != nil {
		slog.Warn("escalation: failed to list plans for lead resolution", "riskId", riskID, "err", err)
		return assignerLead, escalationLead{}
	}
	for _, p := range plans {
		if p.ActionOwnerID != nil {
			actionOwnerLead = s.managerOf(ctx, *p.ActionOwnerID)
			break
		}
	}
	return assignerLead, actionOwnerLead
}

// managerOf resolves a user's line manager: their email from HR entity, then
// (best-effort, independently) that email's Asgardeo id from the identity
// directory. A manager who has left WSO2's directory but is still on file in
// HR entity resolves email-only — email is recorded regardless of whether the
// uuid lookup succeeds, since it costs nothing and is a human-readable trace
// of who was resolved even when the uuid comparison can't use them.
func (s *escalationService) managerOf(ctx context.Context, userID int) escalationLead {
	if userID <= 0 {
		return escalationLead{}
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil || u == nil || u.Email == "" {
		return escalationLead{}
	}
	emp, err := s.hr.GetEmployeeByEmail(ctx, u.Email)
	if err != nil {
		slog.Warn("escalation: HR lookup failed", "err", err)
		return escalationLead{}
	}
	if emp == nil || strings.TrimSpace(emp.ManagerEmail) == "" {
		return escalationLead{}
	}
	email := strings.TrimSpace(emp.ManagerEmail)
	lead := escalationLead{email: &email}

	if s.scim != nil {
		dirUser, err := s.scim.LookupByEmail(ctx, email)
		if err != nil {
			slog.Warn("escalation: directory lookup for lead failed; comment/visibility rights via this lead are unavailable this time",
				"err", err)
		} else if dirUser != nil {
			lead.uuid = &dirUser.UUID
		}
		// dirUser == nil (no error): the manager has no Asgardeo account. lead.uuid
		// stays nil — same soft degradation as every other unresolvable case here.
	}
	return lead
}

func (s *escalationService) Comment(ctx context.Context, riskID, escalationID int, comment, callerEmail string, canOverride bool) (*model.Escalation, error) {
	if riskID <= 0 || escalationID <= 0 {
		return nil, badRequest("riskId and escalationId must be positive integers")
	}
	if strings.TrimSpace(comment) == "" {
		return nil, badRequest("comment is required")
	}

	detail, err := s.riskRepo.GetByID(ctx, riskID)
	if err != nil {
		return nil, err
	}
	if detail.WorkflowStatus != model.StatusEscalated {
		return nil, &apierror.Error{
			StatusCode: http.StatusConflict,
			Body:       "only an escalated risk can be commented on, currently: " + detail.WorkflowStatus,
		}
	}

	escalations, err := s.repo.List(ctx, riskID)
	if err != nil {
		return nil, err
	}
	var target *model.Escalation
	for _, e := range escalations {
		if e.ID == escalationID {
			target = e
			break
		}
	}
	if target == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "escalation not found for this risk"}
	}
	if target.Status != "OPEN" {
		return nil, &apierror.Error{StatusCode: http.StatusConflict, Body: "this escalation is already resolved"}
	}

	if err := s.authorizeComment(ctx, detail, target, callerEmail, canOverride); err != nil {
		return nil, err
	}

	// repo.Comment records the decision and returns the risk to IN_REMEDIATION
	// in one entity-side transaction — no separate TransitionStatus call here,
	// since that would be a second, independent HTTP round trip with no way
	// to roll either back if the other failed. The escalation stays OPEN
	// deliberately, so the risk remains in the Overdue tab until the assigner
	// submits for completion approval.
	updated, err := s.repo.Comment(ctx, riskID, escalationID, strings.TrimSpace(comment), callerEmail)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// authorizeComment decides who may comment, which depends on the risk's level:
//
//   - HIGH: the risk's own management_approver_id. Senior management owns the
//     judgement call on a high risk that has already slipped its deadline.
//   - MEDIUM / LOW: the assigner's or the action owner's line manager, as
//     frozen on the escalation row. These don't warrant management attention,
//     but somebody above the work still has to look.
//
// A compliance admin bypasses both, mirroring the identity gates on the
// approval transitions — otherwise an escalation whose named commenter has left
// would strand the risk permanently.
func (s *escalationService) authorizeComment(ctx context.Context, detail *model.RiskDetail, esc *model.Escalation, callerUUID string, canOverride bool) error {
	if canOverride {
		return nil
	}
	if isHighRisk(detail) {
		caller, err := s.users.GetByUUID(ctx, callerUUID)
		if err != nil {
			return err
		}
		if caller == nil || caller.ID != detail.ManagementApproverID {
			return &apierror.Error{
				StatusCode: http.StatusForbidden,
				Body:       "only this risk's Management Approver may comment on a high-risk escalation",
			}
		}
		return nil
	}

	// Matched on uuid, not user id: a line manager need not be a platform user
	// — resolveLeads records their Asgardeo id directly from the identity
	// directory, not from any row in `user`. A nil lead (SCIM couldn't resolve
	// the manager, or none is recorded) grants nobody the right via that arm —
	// same soft-degradation resolveLeads already applies when HR entity itself
	// has nothing to offer.
	for _, lead := range []*string{esc.AssignerLeadUUID, esc.ActionOwnerLeadUUID} {
		if lead != nil && *lead == callerUUID {
			return nil
		}
	}
	return &apierror.Error{
		StatusCode: http.StatusForbidden,
		Body:       "only the Risk Assigner's or Action Owner's lead may comment on this escalation",
	}
}

// isHighRisk reads the effective (residual) level, falling back to gross —
// the same "what this risk currently is" convention the registers table shows,
// so the escalation path can't disagree with the level on screen.
func isHighRisk(detail *model.RiskDetail) bool {
	if detail.EffectiveScore != nil {
		return detail.EffectiveScore.RiskLevel == "HIGH"
	}
	return detail.GrossScore != nil && detail.GrossScore.RiskLevel == "HIGH"
}

func (s *escalationService) ResolveOpen(ctx context.Context, riskID int, updatedBy string) error {
	if riskID <= 0 {
		return badRequest("riskId must be a positive integer")
	}
	return s.repo.Resolve(ctx, riskID, updatedBy)
}
