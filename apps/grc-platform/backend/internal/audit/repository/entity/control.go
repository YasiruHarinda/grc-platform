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

package entity

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type controlRepo struct{ c *entityclient.Client }

// NewControlRepository returns an entity-backed ControlRepository.
func NewControlRepository(c *entityclient.Client) repository.ControlRepository {
	return &controlRepo{c: c}
}

func (r *controlRepo) List(ctx context.Context, auditID int) ([]*model.AuditControl, error) {
	var all []*model.AuditControl
	path := fmt.Sprintf("/audits/%d/controls/search", auditID)
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Controls []*model.AuditControl `json:"controls"`
		}
		if err := r.c.Post(ctx, path, pageBody(offset), &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Controls...)
		if len(resp.Controls) < pageLimit {
			return all, nil
		}
	}
}

func (r *controlRepo) ListScoped(ctx context.Context, auditID int, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.AuditControl, error) {
	var all []*model.AuditControl
	path := fmt.Sprintf("/audits/%d/controls/search", auditID)
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Controls []*model.AuditControl `json:"controls"`
		}
		body := map[string]any{
			"scope":        scope,
			"userId":       userID,
			"scopeTeamIds": scopeTeamIDs,
			"pagination":   map[string]int{"limit": pageLimit, "offset": offset},
		}
		if err := r.c.Post(ctx, path, body, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Controls...)
		if len(resp.Controls) < pageLimit {
			return all, nil
		}
	}
}

// InScope reports whether controlID is visible to userID at scope, by
// reusing /audits/{auditId}/controls/search (the same scoped query ListScoped
// uses) filtered to just this one control id — avoids a bespoke endpoint for
// what is otherwise the same check.
func (r *controlRepo) InScope(ctx context.Context, auditID, controlID int, scope model.Scope, userID int, scopeTeamIDs []int) (bool, error) {
	var resp struct {
		Controls []*model.AuditControl `json:"controls"`
	}
	body := map[string]any{
		"controlIds":   []int{controlID},
		"scope":        scope,
		"userId":       userID,
		"scopeTeamIds": scopeTeamIDs,
		"pagination":   map[string]int{"limit": 1, "offset": 0},
	}
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/search", auditID), body, &resp); err != nil {
		return false, err
	}
	return len(resp.Controls) > 0, nil
}

// GetByID does not separately enrich population-phase fields (due date, owner,
// team) — the entity's control select already LEFT JOINs the latest
// audit_population round for OE controls (see controlFromClause,
// entity/compliance-entity/internal/repository/audit_control_repo.go), so
// they arrive on the same response as everything else.
func (r *controlRepo) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	var c model.AuditControl
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *controlRepo) Create(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error) {
	var c model.AuditControl
	body := withField(req, map[string]any{"createdBy": createdBy})
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls", auditID), body, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *controlRepo) BulkCreate(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error) {
	controls := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		controls = append(controls, withField(req, map[string]any{"createdBy": createdBy}))
	}
	var resp struct {
		Controls []*model.AuditControl `json:"controls"`
	}
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/bulk", auditID), map[string]any{"controls": controls}, &resp); err != nil {
		return nil, err
	}
	return resp.Controls, nil
}

func (r *controlRepo) Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error {
	body := map[string]any{"updatedBy": updatedBy}
	if req.Description != nil {
		body["description"] = req.Description
	}
	if req.ControlType != nil {
		body["controlType"] = req.ControlType
	}
	if req.Scope != nil {
		body["scope"] = req.Scope
	}
	if req.EvidenceRequirement != nil {
		body["evidenceRequirement"] = req.EvidenceRequirement
	}
	if req.ClearOwner {
		// The entity's own UpdateControlRequest has the identical nil-means-
		// unchanged ambiguity on ownerId, so clearing it must be signaled the
		// same way we signal it here: an explicit clearOwner flag, not just a
		// null value.
		body["ownerId"] = nil
		body["clearOwner"] = true
	} else if req.OwnerID != nil {
		body["ownerId"] = req.OwnerID
	}
	if req.ClearTeam {
		body["teamId"] = nil
		body["clearTeam"] = true
	} else if req.TeamID != nil {
		body["teamId"] = req.TeamID
	}
	if req.ClearAuditor {
		body["auditorId"] = nil
		body["clearAuditor"] = true
	} else if req.AuditorID != nil {
		body["auditorId"] = req.AuditorID
	}
	if req.DueDate != nil {
		body["dueDate"] = req.DueDate
	}
	return r.c.Patch(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), body, nil)
}

func (r *controlRepo) UpdateStatus(ctx context.Context, auditID, controlID int, status string, comment *string, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy, "comments": comment}
	return r.c.Patch(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), body, nil)
}

func (r *controlRepo) UpdateStatusWithSample(ctx context.Context, auditID, controlID int, status string, sampleReference string, updatedBy string) error {
	body := map[string]any{"status": status, "sampleReference": sampleReference, "updatedBy": updatedBy}
	return r.c.Patch(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), body, nil)
}

func (r *controlRepo) OverrideStatus(ctx context.Context, auditID, controlID int, status string, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy}
	return r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/%d/status-override", auditID, controlID), body, nil)
}

func (r *controlRepo) Delete(ctx context.Context, auditID, controlID int) error {
	return r.c.Delete(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID))
}

func (r *controlRepo) AssignedAuditID(ctx context.Context, userID int, controlID int) (int, bool, error) {
	var resp struct {
		AuditID int `json:"auditId"`
	}
	path := fmt.Sprintf("/audit-controls/%d/evidence-assignment?userId=%d", controlID, userID)
	if err := r.c.Get(ctx, path, &resp); err != nil {
		if notFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return resp.AuditID, true, nil
}

// ListAllForReminders returns every control across every audit, unfiltered
// (no status/owner/audit restriction) — for the daily reminder job's sweep.
// The entity has no due-date range filter, so filtering happens in the job
// itself; this reuses the existing global search endpoint with an empty
// filter rather than adding a bespoke one, mirroring List/ListScoped's own
// paging idiom.
func (r *controlRepo) ListAllForReminders(ctx context.Context) ([]*model.AuditControl, error) {
	var all []*model.AuditControl
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Controls []*model.AuditControl `json:"controls"`
		}
		if err := r.c.Post(ctx, "/controls/search", pageBody(offset), &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Controls...)
		if len(resp.Controls) < pageLimit {
			return all, nil
		}
	}
}

func (r *controlRepo) ActivePopulationID(ctx context.Context, controlID int) (int, bool, error) {
	var resp struct {
		PopulationID int `json:"populationId"`
	}
	path := fmt.Sprintf("/audit-controls/%d/active-population", controlID)
	if err := r.c.Get(ctx, path, &resp); err != nil {
		if notFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return resp.PopulationID, true, nil
}

// notFound reports whether err is an entity 404 (mapped to apierror.Error by the
// entity client), so callers can treat "not assigned" / "no population" as a
// domain condition rather than a transport error.
func notFound(err error) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
