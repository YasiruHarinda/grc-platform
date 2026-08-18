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
	"net/url"
	"sync"

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
			r.enrichPopulations(ctx, auditID, all)
			return all, nil
		}
	}
}

func (r *controlRepo) ListScoped(ctx context.Context, auditID int, scope model.Scope, userEmail string, scopeTeamIDs []int) ([]*model.AuditControl, error) {
	var all []*model.AuditControl
	path := fmt.Sprintf("/audits/%d/controls/search", auditID)
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Controls []*model.AuditControl `json:"controls"`
		}
		body := map[string]any{
			"scope":        scope,
			"userEmail":    userEmail,
			"scopeTeamIds": scopeTeamIDs,
			"pagination":   map[string]int{"limit": pageLimit, "offset": offset},
		}
		if err := r.c.Post(ctx, path, body, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Controls...)
		if len(resp.Controls) < pageLimit {
			r.enrichPopulations(ctx, auditID, all)
			return all, nil
		}
	}
}

// InScope reports whether controlID is visible to userEmail at scope, by
// reusing /audits/{auditId}/controls/search (the same scoped query ListScoped
// uses) filtered to just this one control id — avoids a bespoke endpoint for
// what is otherwise the same check.
func (r *controlRepo) InScope(ctx context.Context, auditID, controlID int, scope model.Scope, userEmail string, scopeTeamIDs []int) (bool, error) {
	var resp struct {
		Controls []*model.AuditControl `json:"controls"`
	}
	body := map[string]any{
		"controlIds":   []int{controlID},
		"scope":        scope,
		"userEmail":    userEmail,
		"scopeTeamIds": scopeTeamIDs,
		"pagination":   map[string]int{"limit": 1, "offset": 0},
	}
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/search", auditID), body, &resp); err != nil {
		return false, err
	}
	return len(resp.Controls) > 0, nil
}

func (r *controlRepo) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	var c model.AuditControl
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), &c); err != nil {
		return nil, err
	}
	r.enrichPopulations(ctx, auditID, []*model.AuditControl{&c})
	return &c, nil
}

// entPopulation is the subset of the entity's AuditPopulation JSON needed to
// enrich controls with population-phase fields.
type entPopulation struct {
	ID      int     `json:"id"`
	OwnerID *int    `json:"ownerId"`
	TeamID  *int    `json:"teamId"`
	DueDate *string `json:"dueDate"`
}

// enrichPopulations fills PopulationDueDate/OwnerName/TeamName on OE controls.
// The entity's control queries do not join audit_population (and the entity is
// owned by another team), so the backend stitches the data from the entity's
// per-control populations endpoint plus the user/team lookups. Enrichment is
// best-effort: on any error the population fields simply stay nil.
func (r *controlRepo) enrichPopulations(ctx context.Context, auditID int, controls []*model.AuditControl) {
	var oe []*model.AuditControl
	for _, c := range controls {
		if c.RequirementType == "OE" {
			oe = append(oe, c)
		}
	}
	if len(oe) == 0 {
		return
	}

	// Fetch each OE control's populations and resolve owner/team names with
	// point-lookups, avoiding full-table scans of the user and team directories.
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, c := range oe {
		wg.Add(1)
		go func(c *model.AuditControl) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var pops []entPopulation
			if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d/populations", auditID, c.ID), &pops); err != nil || len(pops) == 0 {
				return
			}
			p := pops[len(pops)-1] // latest round
			c.PopulationDueDate = p.DueDate
			if p.OwnerID != nil {
				var u model.UserRef
				if err := r.c.Get(ctx, fmt.Sprintf("/users/%d", *p.OwnerID), &u); err == nil {
					name := u.DisplayName
					c.PopulationOwnerName = &name
				}
			}
			if p.TeamID != nil {
				var t model.AuditTeam
				if err := r.c.Get(ctx, fmt.Sprintf("/audit/teams/%d", *p.TeamID), &t); err == nil {
					name := t.Name
					c.PopulationTeamName = &name
				}
			}
		}(c)
	}
	wg.Wait()
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
	if req.OwnerID != nil {
		body["ownerId"] = req.OwnerID
	}
	if req.TeamID != nil {
		body["teamId"] = req.TeamID
	}
	if req.AuditorID != nil {
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

func (r *controlRepo) AssignedAuditID(ctx context.Context, userEmail string, controlID int) (int, bool, error) {
	var resp struct {
		AuditID int `json:"auditId"`
	}
	path := fmt.Sprintf("/audit-controls/%d/evidence-assignment?email=%s", controlID, url.QueryEscape(userEmail))
	if err := r.c.Get(ctx, path, &resp); err != nil {
		if notFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return resp.AuditID, true, nil
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
