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

// Package model defines the domain types for the Audit Hub module.
package model

import "time"

// AuditControl represents a control under evaluation within an audit. Every
// control owns its full definition text directly — it is never linked to the
// framework catalog by foreign key.
type AuditControl struct {
	ID      int  `json:"id"`
	AuditID int  `json:"auditId"`
	OwnerID *int `json:"ownerId"`
	// OwnerName is filled in by the service layer from OwnerUUID/OwnerUserType
	// via the identity directory, overwriting whatever the entity sent (it
	// doesn't send a name — the `user` table stores none; see shared.sql).
	// OwnerUUID/OwnerUserType stay populated afterward too, so a caller can
	// tell a real "no owner" apart from "not yet enriched".
	OwnerName     *string `json:"ownerName"`
	OwnerUUID     *string `json:"ownerUuid"`
	OwnerUserType *string `json:"ownerUserType"`
	TeamID        *int    `json:"teamId"`
	TeamName      *string `json:"teamName"`
	AuditorID     *int    `json:"auditorId"`
	// AuditorName is filled in the same way OwnerName is.
	AuditorName         *string   `json:"auditorName"`
	AuditorUUID         *string   `json:"auditorUuid"`
	AuditorUserType     *string   `json:"auditorUserType"`
	ControlNumber       string    `json:"controlNumber"`
	Description         string    `json:"description"`
	EvidenceRequirement *string   `json:"evidenceRequirement"`
	RequirementType     string    `json:"requirementType"`
	ControlType         string    `json:"controlType"`
	Scope               string    `json:"scope"`
	DueDate             *string   `json:"dueDate"`
	Status              string    `json:"status"`
	SampleReference     *string   `json:"sampleReference"`
	SampleFileURL       *string   `json:"sampleFileUrl"`
	SampleFileName      *string   `json:"sampleFileName"`
	Comments            *string   `json:"comments"`
	ControlSource       string    `json:"controlSource"` // MANUAL | COPIED | CSV
	IsManuallyAdded     bool      `json:"isManuallyAdded"`
	IsOverdue           bool      `json:"isOverdue"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	// Population-phase fields (OE controls), joined from the initial audit_population record.
	PopulationDescription *string `json:"populationDescription"`
	PopulationComments    *string `json:"populationComments"`
	PopulationDueDate     *string `json:"populationDueDate"`
	// PopulationOwnerName is filled in the same way OwnerName is.
	PopulationOwnerName     *string `json:"populationOwnerName"`
	PopulationOwnerUUID     *string `json:"populationOwnerUuid"`
	PopulationOwnerUserType *string `json:"populationOwnerUserType"`
	PopulationTeamName      *string `json:"populationTeamName"`
	// StatusOverridden/OverriddenBy/OverriddenAt record that this control's
	// status was last set by a backward override rather than the ordinary
	// workflow.
	StatusOverridden bool       `json:"statusOverridden"`
	OverriddenBy     *string    `json:"overriddenBy"`
	OverriddenAt     *time.Time `json:"overriddenAt"`
	// PopulationID/PopulationOwnerID/PopulationStatus are the IDs behind
	// PopulationOwnerName/PopulationTeamName above — used by the reminder job
	// to resolve and dedup against the population's own owner (which may
	// differ from the control's owner) and to know when its round is done.
	PopulationID      *int    `json:"populationId"`
	PopulationOwnerID *int    `json:"populationOwnerId"`
	PopulationStatus  *string `json:"populationStatus"`
}

// ControlListResponse is returned by GET /api/v1/audits/{id}/controls.
type ControlListResponse struct {
	Items []*AuditControl `json:"items"`
	Total int             `json:"total"`
}

// PopulationDetails is included in AddControlRequest for OE-type controls.
// It maps to a row in audit_population.
// OwnerID and TeamID represent the population-phase process owner and team,
// which may differ from the control's owner and team (evidence phase).
// AuditorID is shared with the control and is not stored separately here.
type PopulationDetails struct {
	Description     string  `json:"description"`
	ReferenceNumber *int    `json:"referenceNumber"`
	DueDate         *string `json:"dueDate"`
	Comments        *string `json:"comments"`
	OwnerID         *int    `json:"ownerId"`
	TeamID          *int    `json:"teamId"`
}

// AddControlRequest is the payload for POST /api/v1/audits/{id}/controls.
// Always creates a standalone control with full definition text.
// PushToFramework optionally also writes the control into the audit's
// framework catalog as a side effect (see model/framework_control.go): a
// first version when SourceFrameworkControlID is nil, or a new version of
// that existing catalog control when it's set (edited-existing-control
// push-back).
type AddControlRequest struct {
	ControlSource       string             `json:"controlSource"` // MANUAL | COPIED | CSV; defaults to MANUAL
	ControlNumber       string             `json:"controlNumber"`
	Description         string             `json:"description"`
	EvidenceRequirement *string            `json:"evidenceRequirement"`
	RequirementType     string             `json:"requirementType"` // DESIGN | OE
	ControlType         string             `json:"controlType"`     // CONFIG | NON_CONFIG
	Scope               string             `json:"scope"`           // COMMON | PRODUCT_SPECIFIC
	OwnerID             *int               `json:"ownerId"`
	TeamID              *int               `json:"teamId"`
	AuditorID           *int               `json:"auditorId"`
	DueDate             *string            `json:"dueDate"`
	Population          *PopulationDetails `json:"population"` // OE controls only

	PushToFramework          bool `json:"pushToFramework"`
	SourceFrameworkControlID *int `json:"sourceFrameworkControlId"`
}

// BulkAddControlsRequest is the payload for POST /api/v1/audits/{id}/controls/bulk.
type BulkAddControlsRequest struct {
	Controls []AddControlRequest `json:"controls"`
}

// UpdateControlRequest is the payload for PUT /api/v1/audits/{id}/controls/{controlId}.
// All fields are optional; nil means "do not change".
type UpdateControlRequest struct {
	ControlNumber       *string `json:"controlNumber"`
	Description         *string `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     *string `json:"requirementType"`
	ControlType         *string `json:"controlType"`
	Scope               *string `json:"scope"`
	OwnerID             *int    `json:"ownerId"`
	TeamID              *int    `json:"teamId"`
	AuditorID           *int    `json:"auditorId"`
	DueDate             *string `json:"dueDate"`
	// ClearOwner/ClearTeam/ClearAuditor request unassigning that field back to
	// empty. They exist because OwnerID/TeamID/AuditorID's own nil already
	// means "field omitted, do not change" — JSON can't tell that apart from
	// an explicit null, so there would otherwise be no way to ever unassign
	// one of these once set. Ignored when the matching *ID field is non-nil
	// (assigning and clearing the same field in one request is nonsensical).
	ClearOwner   bool `json:"clearOwner"`
	ClearTeam    bool `json:"clearTeam"`
	ClearAuditor bool `json:"clearAuditor"`
	// Population is set only for OE controls being edited from the same form
	// used to create them; nil means "leave population details unchanged".
	Population *PopulationDetails `json:"population"`
}

// UpdateStatusRequest is the payload for PATCH /api/v1/audits/{id}/controls/{controlId}/status.
type UpdateStatusRequest struct {
	Status  string  `json:"status"`
	Comment *string `json:"comment"`
}

// OverrideStatusRequest is the payload for
// POST /api/v1/audits/{id}/controls/{controlId}/status/override. Unlike
// UpdateStatusRequest, the target status is validated as a backward-only rank
// move by the entity, not the ordinary forward workflow transitions.
type OverrideStatusRequest struct {
	Status string `json:"status"`
}
