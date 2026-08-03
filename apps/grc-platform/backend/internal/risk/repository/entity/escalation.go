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
	"fmt"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type escalationRepository struct{ c *entityclient.Client }

// NewEscalationRepository creates a Compliance Entity-backed repository.EscalationRepository.
//
// Escalation is still primarily automatic (the compliance-entity's daily job)
// and resolution is still entirely the action-plan-completion cascade's job
// (internal/job, risk_action_plan_service.go) — Escalate here only covers the
// manual "Compliance clicks Escalate on an overdue risk" path, an earlier
// trigger for the same automatic outcome, not an alternate one.
func NewEscalationRepository(c *entityclient.Client) repository.EscalationRepository {
	return &escalationRepository{c: c}
}

// entEscalation is the entity's camelCase escalation. escalated_to/reason were
// dropped from risk_escalation — see risk_schema.sql — since escalation is
// system-driven, not a human decision.
type entEscalation struct {
	ID                   int       `json:"id"`
	RiskID               int       `json:"riskId"`
	NewTreatmentStrategy *string   `json:"newTreatmentStrategy"`
	ActionPlanID         *int      `json:"actionPlanId"`
	Decision             *string   `json:"decision"`
	AssignerLeadEmail    *string   `json:"assignerLeadEmail"`
	ActionOwnerLeadEmail *string   `json:"actionOwnerLeadEmail"`
	Status               string    `json:"status"`
	CreatedOn            time.Time `json:"createdOn"`
}

func (e entEscalation) toModel() *model.Escalation {
	return &model.Escalation{
		ID:                   e.ID,
		RiskID:               e.RiskID,
		NewTreatmentStrategy: e.NewTreatmentStrategy,
		ActionPlanID:         e.ActionPlanID,
		Decision:             e.Decision,
		AssignerLeadEmail:    e.AssignerLeadEmail,
		ActionOwnerLeadEmail: e.ActionOwnerLeadEmail,
		Status:               e.Status,
		CreatedAt:            e.CreatedOn,
	}
}

func (r *escalationRepository) List(ctx context.Context, riskID int) ([]*model.Escalation, error) {
	var resp struct {
		Escalations []entEscalation `json:"escalations"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/%d/escalations", riskID), &resp); err != nil {
		return nil, fmt.Errorf("list escalations for risk %d: %w", riskID, err)
	}
	escalations := make([]*model.Escalation, 0, len(resp.Escalations))
	for _, e := range resp.Escalations {
		escalations = append(escalations, e.toModel())
	}
	return escalations, nil
}

func (r *escalationRepository) Escalate(ctx context.Context, riskID int, createdBy string, assignerLead, actionOwnerLead *string) (*model.Escalation, error) {
	body := map[string]any{
		"createdBy":            createdBy,
		"assignerLeadEmail":    assignerLead,
		"actionOwnerLeadEmail": actionOwnerLead,
	}
	var e entEscalation
	if err := r.c.Post(ctx, fmt.Sprintf("/risks/%d/escalate", riskID), body, &e); err != nil {
		return nil, fmt.Errorf("escalate risk %d: %w", riskID, err)
	}
	return e.toModel(), nil
}

// Comment writes the decision text via the entity's escalation PATCH. Status is
// deliberately left alone — the comment returns the risk to the assigner, but
// the escalation stays OPEN so the risk remains in the Overdue tab.
func (r *escalationRepository) Comment(ctx context.Context, riskID, escalationID int, comment, updatedBy string) (*model.Escalation, error) {
	body := map[string]any{"decision": comment, "updatedBy": updatedBy}
	var e entEscalation
	if err := r.c.Patch(ctx, fmt.Sprintf("/risks/%d/escalations/%d", riskID, escalationID), body, &e); err != nil {
		return nil, fmt.Errorf("comment on escalation %d: %w", escalationID, err)
	}
	return e.toModel(), nil
}

// Resolve marks every OPEN escalation on the risk RESOLVED. Plural because a
// risk escalated, returned, and escalated again carries more than one row, and
// leaving an older one open would strand the risk in the Overdue tab.
func (r *escalationRepository) Resolve(ctx context.Context, riskID int, updatedBy string) error {
	escalations, err := r.List(ctx, riskID)
	if err != nil {
		return err
	}
	resolved := "RESOLVED"
	for _, e := range escalations {
		if e.Status != "OPEN" {
			continue
		}
		body := map[string]any{"status": resolved, "updatedBy": updatedBy}
		var out entEscalation
		if err := r.c.Patch(ctx, fmt.Sprintf("/risks/%d/escalations/%d", riskID, e.ID), body, &out); err != nil {
			return fmt.Errorf("resolve escalation %d: %w", e.ID, err)
		}
	}
	return nil
}
