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

import "time"

// Escalation represents a risk escalation record, mapping to `risk_escalation`.
// Created automatically by the compliance-entity's daily overdue-risk job —
// there is no escalated_to/reason, since no human chooses a target or
// justification at creation time; CreatedAt is what "escalated on" shows in
// the UI instead.
type Escalation struct {
	ID                   int     `json:"id"`
	RiskID               int     `json:"risk_id"`
	NewTreatmentStrategy *string `json:"new_treatment_strategy"`
	ActionPlanID         *int    `json:"action_plan_id"`
	// Decision holds the management/lead comment that returns an escalated
	// risk to its assigner. Nil until someone comments.
	Decision *string `json:"decision"`
	// Line managers of the assigner and action owner, resolved from the HR
	// entity once at escalation time. They decide who may comment on a
	// medium/low escalation and who can see the risk — authorizeComment
	// (risk/service/escalation.go) reads these two Go fields directly, but
	// they're json:"-" deliberately: handleListEscalations is gated only on
	// the broad RISK_VIEW_RISKS privilege, so anyone who can view an
	// escalated risk would otherwise receive two other people's email
	// addresses that the webapp never reads.
	AssignerLeadEmail    *string `json:"-"`
	ActionOwnerLeadEmail *string `json:"-"`
	// AssignerLeadUUID/ActionOwnerLeadUUID are the same leads' Asgardeo ids,
	// resolved independently from AssignerLeadEmail/ActionOwnerLeadEmail (see
	// escalationService.managerOf) — a lead may resolve by email but not by
	// uuid (no Asgardeo account), so these can be nil even when the email
	// fields are set. authorizeComment compares the caller by uuid, not
	// email, so these are what actually grant comment rights.
	AssignerLeadUUID     *string   `json:"-"`
	ActionOwnerLeadUUID  *string   `json:"-"`
	Status               string    `json:"status"` // OPEN | RESOLVED
	CreatedAt            time.Time `json:"created_at"`
}

// EscalationCommentRequest is the payload for
// POST /api/v1/risks/{id}/escalations/{escalationId}/comment.
type EscalationCommentRequest struct {
	Comment string `json:"comment"`
}
