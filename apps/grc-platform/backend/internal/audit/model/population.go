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

// AuditPopulation represents one audit_population row (a population round) for
// an OE control. A control normally has exactly one round for its whole
// lifecycle — both internal-review and auditor rejections reuse the same round
// rather than starting a new one.
type AuditPopulation struct {
	ID              int     `json:"id"`
	ControlID       int     `json:"controlId"`
	Status          string  `json:"status"` // PENDING|SUBMITTED|COMPLIANCE_APPROVED|COMPLIANCE_REJECTED|APPROVED|AUDITOR_REJECTED
	ReferenceNumber *int    `json:"referenceNumber"`
	Description     *string `json:"description"`
	DueDate         *string `json:"dueDate"`
	Comments        *string `json:"comments"`
	// Attestation is a written note standing in for population files (a
	// fileless submit, or a note alongside files). Nil otherwise.
	Attestation *string `json:"attestation"`
	// OwnerID/TeamID are the population-phase process owner and team, which
	// may differ from the control's own owner/team (evidence phase) — see
	// PopulationDetails on model/control.go, which is where they're written.
	OwnerID   *int      `json:"ownerId"`
	TeamID    *int      `json:"teamId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PopulationFile is one file on a population round, tagged POPULATION
// (team-submitted) or SAMPLE (auditor-selected). Shares the audit_evidence_file
// table with evidence files, distinguished by population_id/file_kind.
type PopulationFile struct {
	ID           int       `json:"id"`
	PopulationID int       `json:"populationId"`
	FileKind     string    `json:"fileKind"` // POPULATION | SAMPLE
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	FileType     *string   `json:"fileType"`
	FileSize     *int64    `json:"fileSize"`
	CreatedAt    time.Time `json:"createdAt"`
	// ReadURL is the backend proxy download URL (GET .../population/files/{id}/download).
	// Computed at list time (not persisted); nil if the file has no DB id.
	ReadURL *string `json:"readUrl"`
	// TeamID is this file's owning control's team_id (nil if none). Only
	// populated by PopulationService.GetFileByID's underlying repo call, for
	// the team-scoped download gate — omitted from JSON since it's not
	// population metadata callers need.
	TeamID *int `json:"-"`
}

// PopulationView is the response for GET .../population: the control's current
// round plus its files split by kind, and the auditor's sample note (which lives
// on audit_control, not the population row).
type PopulationView struct {
	Round           *AuditPopulation  `json:"round"`
	PopulationFiles []*PopulationFile `json:"populationFiles"`
	SampleFiles     []*PopulationFile `json:"sampleFiles"`
	SampleReference *string           `json:"sampleReference"`
}

// PopulationSubmitResult is returned by the Evidence Portal population submit
// endpoint once uploaded blobs are recorded and the round advances.
type PopulationSubmitResult struct {
	PopulationID int    `json:"populationId"`
	ControlID    int    `json:"controlId"`
	Status       string `json:"status"`
	FolderPath   string `json:"folderPath"`
	FileCount    int    `json:"fileCount"`
}

// ReviewDecisionRequest is the payload for the population/sample/evidence
// review & validate decision endpoints: {"decision":"APPROVE"|"REJECT","comment":"..."}.
type ReviewDecisionRequest struct {
	Decision string  `json:"decision"`
	Comment  *string `json:"comment"`
}

// SampleSubmitRequest is the payload for POST .../sample/submit.
type SampleSubmitRequest struct {
	FolderPath string `json:"folderPath"`
	Note       string `json:"note"`
}
