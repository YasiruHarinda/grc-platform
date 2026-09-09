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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directorysync"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// The roles that count as an ongoing assignment. Records of past acts — comment
// authors, trail actors, who validated — are deliberately absent.
const (
	roleControlOwner    = "Control Owner"
	roleAuditorPOC      = "Auditor POC"
	rolePopulationOwner = "Population Owner"
)

// userGetter resolves an owner id to the uuid/type/status a digest recipient is
// checked against — a narrow local interface, as auditLister/controlLister are,
// so this package stays clear of the repository types that would cycle back
// through internal/audit/handler.
type userGetter interface {
	GetByID(ctx context.Context, id int) (*model.UserRef, error)
}

// DepartureHub is the audit half of the Directory Status Sync: the ongoing work
// each departed user is still named on, the admins who can reassign it, and the
// digest that tells them. Its three methods are wired into a directorysync.Hub
// in cmd/server.
type DepartureHub struct {
	audits          auditLister
	controls        controlLister
	users           userGetter
	grants          grant.Repository
	directory       *directory.Service
	email           *emailer.Client
	frontendBaseURL string
}

// DepartureDeps is DepartureHub's construction args.
type DepartureDeps struct {
	Audits          auditLister
	Controls        controlLister
	Users           userGetter
	Grants          grant.Repository
	Directory       *directory.Service
	Email           *emailer.Client
	FrontendBaseURL string
}

// NewDepartureHub constructs a DepartureHub.
func NewDepartureHub(d DepartureDeps) *DepartureHub {
	return &DepartureHub{
		audits:          d.Audits,
		controls:        d.Controls,
		users:           d.Users,
		grants:          d.Grants,
		directory:       d.Directory,
		email:           d.Email,
		frontendBaseURL: d.FrontendBaseURL,
	}
}

// Assignments returns each user's ongoing audit assignments. COMPLETED and
// REMOVED audits are excluded; ARCHIVED is still ongoing, as the reminder job has it.
func (h *DepartureHub) Assignments(ctx context.Context, userIDs []int) (map[int][]directorysync.Assignment, error) {
	wanted := make(map[int]bool, len(userIDs))
	for _, id := range userIDs {
		wanted[id] = true
	}
	out := make(map[int][]directorysync.Assignment, len(userIDs))
	if len(wanted) == 0 {
		return out, nil
	}

	audits, err := h.audits.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list audits: %w", err)
	}
	liveAuditIDs := make(map[int]bool, len(audits))
	auditNames := make(map[int]string, len(audits))
	for _, a := range audits {
		if a.IsOngoing() {
			liveAuditIDs[a.ID] = true
		}
		auditNames[a.ID] = a.Name
	}

	controls, err := h.controls.ListAllForReminders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list controls: %w", err)
	}

	add := func(ownerID *int, role, dueDate string, controlNumber, description string, auditID, controlID int) {
		if ownerID == nil || !wanted[*ownerID] {
			return
		}
		out[*ownerID] = append(out[*ownerID], directorysync.Assignment{
			Item:       controlNumber,
			ItemDetail: description,
			Parent:     auditNames[auditID],
			Role:       role,
			Standing:   dueDate,
			DetailURL:  h.controlDetailURL(auditID, controlID),
		})
	}

	for _, c := range controls {
		if c == nil || !liveAuditIDs[c.AuditID] {
			continue
		}
		add(c.OwnerID, roleControlOwner, derefString(c.DueDate), c.ControlNumber, c.Description, c.AuditID, c.ID)
		add(c.AuditorID, roleAuditorPOC, derefString(c.DueDate), c.ControlNumber, c.Description, c.AuditID, c.ID)
		add(c.PopulationOwnerID, rolePopulationOwner, derefString(c.PopulationDueDate), c.ControlNumber, c.Description, c.AuditID, c.ID)
	}
	return out, nil
}

// controlDetailURL deep-links straight to one control's drawer via the
// ?control= query param the audit detail page reads on load.
func (h *DepartureHub) controlDetailURL(auditID, controlID int) string {
	return fmt.Sprintf("%s/audit/audits/%d?control=%d", h.frontendBaseURL, auditID, controlID)
}

// Recipients holds AUDIT_MANAGE_CONTROLS and MANAGE_USERS together, GLOBAL only,
// falling back to platform admins alone when nobody holds both.
func (h *DepartureHub) Recipients(ctx context.Context) ([]int, error) {
	ids, err := grant.CandidateIDsAll(ctx, h.grants, privilege.ManageControls, privilege.ManageUsers)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		return ids, nil
	}
	slog.WarnContext(ctx, "audit departure digest: no GLOBAL holder of both AUDIT_MANAGE_CONTROLS and MANAGE_USERS, falling back to platform admins")
	return grant.CandidateIDs(ctx, h.grants, privilege.ManageUsers)
}

// Notify emails one admin the audit half of a run, grouped by person. An
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
			return &directorysync.Recipient{UUID: u.UUID, UserType: u.UserType, Status: u.Status}, nil
		},
		func(ctx context.Context, uuid, userType string) (string, bool) {
			p, ok := h.directory.LookupTyped(ctx, uuid, userType)
			return p.Email, ok
		})
	if err != nil {
		return err
	}

	groups := make([]emailer.AuditEventGroup, 0, len(departures))
	for _, dep := range departures {
		items := make([]emailer.AuditEventItem, 0, len(dep.Assignments))
		for _, a := range dep.Assignments {
			items = append(items, emailer.AuditEventItem{
				ControlNumber: a.Item,
				Description:   a.ItemDetail,
				DueDate:       a.Standing,
				// Derived from the role: a population owner's row is the
				// population requirement, both control roles the evidence one.
				RequirementType: requirementTypeForRole(a.Role),
				DetailURL:       a.DetailURL,
				Audit:           a.Parent,
				Role:            a.Role,
			})
		}
		groups = append(groups, emailer.AuditEventGroup{Person: dep.Name, Items: items})
	}

	// Spans audits, so each row names its own and the header names none.
	info := emailer.AuditEventInfo{
		DetailURL: h.frontendBaseURL + "/audit/dashboard",
		Groups:    groups,
		ShowAudit: true,
		ShowRole:  true,
	}
	if err := h.email.SendAuditEvent(ctx, emailer.AuditEventDepartureDigest, email, info); err != nil {
		slog.WarnContext(ctx, "audit departure digest: send failed", "adminId", adminUserID, "err", err)
		return fmt.Errorf("send: %w", err)
	}
	slog.InfoContext(ctx, "audit departure digest sent", "adminId", adminUserID, "people", len(departures))
	return nil
}

func requirementTypeForRole(role string) string {
	if role == rolePopulationOwner {
		return "Population Requirement"
	}
	return "Evidence Requirement"
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
