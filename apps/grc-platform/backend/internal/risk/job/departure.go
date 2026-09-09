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

package job

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directorysync"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// The roles that count as an ongoing assignment. Compliance approvals are
// excluded: who approved is a past act, not something anyone can inherit.
const (
	roleRiskOwner          = "Risk Owner"
	roleRiskAssigner       = "Assigner"
	roleManagementApprover = "Management Approver"
	roleActionOwner        = "Action Owner"
)

// departurePageLimit is the page size used when walking live risks.
const departurePageLimit = 100

// The statuses the sweep asks for. The list query has no "exclude" form, so the
// hub still passes an inclusive filter — but it is derived by excluding the
// terminal statuses rather than hand-listing the live ones, so it stays in step
// with the enum and matches how the audit side reads a live parent
// (model.IsOngoingWorkflowStatus is the counterpart of Audit.IsOngoing).
var liveWorkflowStatuses = model.OngoingWorkflowStatuses()

// The two only the named Management Approver can clear: a risk sitting in one
// whose approver has left is stuck until somebody reassigns it.
var blockingStatuses = map[string]bool{
	model.StatusPendingManagementApproval: true,
	model.StatusPendingManagementClosure:  true,
}

// Rendered as the Risk Hub screens do, so a digest row reads the same as the
// risk it links to.
var workflowStatusLabels = map[string]string{
	model.StatusPendingOwnerApproval:      "Pending Owner Approval",
	model.StatusPendingManagementApproval: "Pending Management Approval",
	model.StatusPendingComplianceReview:   "Pending Compliance Approval",
	model.StatusInRemediation:             "In Remediation",
	model.StatusPendingOwnerCompletion:    "Awaiting Owner Sign-off",
	model.StatusPendingManagementClosure:  "Awaiting Management Sign-off",
	model.StatusPendingComplianceClosure:  "Awaiting Closure",
	model.StatusPendingAmendment:          "Pending Amendment",
	model.StatusPendingRevision:           "Pending Revision",
	model.StatusEscalated:                 "Escalated",
}

func workflowStatusLabel(status string) string {
	if label, ok := workflowStatusLabels[status]; ok {
		return label
	}
	return status
}

// userGetter resolves an owner id to the uuid/status a digest recipient is
// checked against — a narrow local interface, as riskLister is, so this package
// stays clear of the repository types that would cycle back through
// internal/risk/handler.
type userGetter interface {
	GetByID(ctx context.Context, id int) (*user.User, error)
}

// DepartureHub is the risk half of the Directory Status Sync: the ongoing work
// each departed user is still named on, the admins who can reassign it, and the
// digest that tells them. Its three methods are wired into a directorysync.Hub
// in cmd/server.
type DepartureHub struct {
	risks           riskLister
	users           userGetter
	grants          grant.Repository
	directory       *directory.Service
	email           *emailer.Client
	frontendBaseURL string
}

// DepartureDeps is DepartureHub's construction args.
type DepartureDeps struct {
	Risks           riskLister
	Users           userGetter
	Grants          grant.Repository
	Directory       *directory.Service
	Email           *emailer.Client
	FrontendBaseURL string
}

// NewDepartureHub constructs a DepartureHub.
func NewDepartureHub(d DepartureDeps) *DepartureHub {
	return &DepartureHub{
		risks:           d.Risks,
		users:           d.Users,
		grants:          d.Grants,
		directory:       d.Directory,
		email:           d.Email,
		frontendBaseURL: d.FrontendBaseURL,
	}
}

// Assignments returns each user's ongoing risk assignments: one sweep for the
// three roles on the risk row, then one query per person for Action Owner.
func (h *DepartureHub) Assignments(ctx context.Context, userIDs []int) (map[int][]directorysync.Assignment, error) {
	wanted := make(map[int]bool, len(userIDs))
	for _, id := range userIDs {
		wanted[id] = true
	}
	out := make(map[int][]directorysync.Assignment, len(userIDs))
	if len(wanted) == 0 {
		return out, nil
	}

	add := func(userID int, r *model.RiskListItem, role string) {
		if userID <= 0 || !wanted[userID] {
			return
		}
		out[userID] = append(out[userID], directorysync.Assignment{
			Item:       r.RiskCode,
			ItemDetail: r.RiskTitle,
			Parent:     r.SourceRegisterName,
			Role:       role,
			Standing:   workflowStatusLabel(r.WorkflowStatus),
			DetailURL:  h.riskDetailURL(r.ID),
			Blocking:   role == roleManagementApprover && blockingStatuses[r.WorkflowStatus],
		})
	}

	for offset := 0; ; offset += departurePageLimit {
		page, err := h.risks.List(ctx, model.ListRisksFilter{
			Statuses: liveWorkflowStatuses,
			Limit:    departurePageLimit,
			Offset:   offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list live risks: %w", err)
		}
		for _, r := range page.Items {
			add(r.OwnerID, r, roleRiskOwner)
			add(r.AssignerID, r, roleRiskAssigner)
			add(r.ManagementApproverID, r, roleManagementApprover)
		}
		if len(page.Items) < departurePageLimit {
			break
		}
	}

	for _, id := range userIDs {
		ownerID := id
		for offset := 0; ; offset += departurePageLimit {
			page, err := h.risks.List(ctx, model.ListRisksFilter{
				Statuses:      liveWorkflowStatuses,
				ActionOwnerID: &ownerID,
				Limit:         departurePageLimit,
				Offset:        offset,
			})
			if err != nil {
				return nil, fmt.Errorf("list action-plan risks for user %d: %w", id, err)
			}
			for _, r := range page.Items {
				add(ownerID, r, roleActionOwner)
			}
			if len(page.Items) < departurePageLimit {
				break
			}
		}
	}
	return out, nil
}

func (h *DepartureHub) riskDetailURL(riskID int) string {
	return fmt.Sprintf("%s/risk/registers?riskId=%d", h.frontendBaseURL, riskID)
}

// Recipients holds RISK_COMPLIANCE_APPROVE and MANAGE_USERS together, GLOBAL only,
// falling back to platform admins alone when nobody holds both.
func (h *DepartureHub) Recipients(ctx context.Context) ([]int, error) {
	ids, err := grant.CandidateIDsAll(ctx, h.grants, privilege.ComplianceApproveRisk, privilege.ManageUsers)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		return ids, nil
	}
	slog.WarnContext(ctx, "risk departure digest: no GLOBAL holder of both RISK_COMPLIANCE_APPROVE and MANAGE_USERS, falling back to platform admins")
	return grant.CandidateIDs(ctx, h.grants, privilege.ManageUsers)
}

// Notify emails one admin the risk half of a run, grouped by person. An
// unresolvable recipient is an error: the status write depends on it.
func (h *DepartureHub) Notify(ctx context.Context, adminUserID int, departures []directorysync.Departure) error {
	if len(departures) == 0 {
		return nil
	}
	email, err := directorysync.DeliverableEmail(ctx, adminUserID,
		func(ctx context.Context, id int) (*directorysync.Recipient, error) {
			u, err := h.users.GetByID(ctx, id)
			if err != nil || u == nil {
				return nil, err
			}
			return &directorysync.Recipient{UUID: u.UUID, Status: u.Status}, nil
		},
		func(ctx context.Context, uuid, _ string) (string, bool) {
			if h.directory == nil || uuid == "" {
				return "", false
			}
			p, ok := h.directory.Lookup(ctx, uuid)
			return p.Email, ok
		})
	if err != nil {
		return err
	}

	groups := make([]emailer.RiskDepartureGroup, 0, len(departures))
	for _, dep := range departures {
		items := make([]emailer.RiskDepartureItem, 0, len(dep.Assignments))
		for _, a := range dep.Assignments {
			items = append(items, emailer.RiskDepartureItem{
				RiskCode:  a.Item,
				RiskTitle: a.ItemDetail,
				Register:  a.Parent,
				Role:      a.Role,
				Status:    a.Standing,
				DetailURL: a.DetailURL,
				Blocking:  a.Blocking,
			})
		}
		groups = append(groups, emailer.RiskDepartureGroup{Person: dep.Name, Items: items})
	}

	// Spans risks, so this links to the register list rather than any one of them.
	info := emailer.RiskDepartureInfo{
		Groups:    groups,
		DetailURL: h.frontendBaseURL + "/risk/registers",
	}
	if err := h.email.SendRiskDepartureDigest(ctx, email, info); err != nil {
		slog.WarnContext(ctx, "risk departure digest: send failed", "adminId", adminUserID, "err", err)
		return fmt.Errorf("send: %w", err)
	}
	slog.InfoContext(ctx, "risk departure digest sent", "adminId", adminUserID, "people", len(departures))
	return nil
}
