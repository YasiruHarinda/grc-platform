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

package mcpserver

import "time"

// Minimal projections of the Compliance Entity's JSON payloads — only the
// fields the MCP tools read. Field names match the entity's domain types.

type entControl struct {
	ID                  int     `json:"id"`
	ControlNumber       string  `json:"controlNumber"`
	Description         string  `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     string  `json:"requirementType"`
}

type entEvidence struct {
	ID        int       `json:"id"`
	ControlID int       `json:"controlId"`
	Status    string    `json:"status"`
	CreatedBy *string   `json:"createdBy"`
	CreatedOn time.Time `json:"createdOn"`
}

type entEvidenceFile struct {
	ID         int     `json:"id"`
	EvidenceID *int    `json:"evidenceId"`
	FileName   string  `json:"fileName"`
	FileType   *string `json:"fileType"`
	FileSize   *int64  `json:"fileSize"`
}

type entListEvidenceFiles struct {
	Files []entEvidenceFile `json:"files"`
}

type entComment struct {
	Content    string    `json:"content"`
	IsInternal bool      `json:"isInternal"`
	CreatedBy  *string   `json:"createdBy"`
	CreatedOn  time.Time `json:"createdOn"`
}

type entListComments struct {
	Comments []entComment `json:"comments"`
}

type entTrailEntry struct {
	ControlID  *int      `json:"controlId"`
	EvidenceID *int      `json:"evidenceId"`
	Action     string    `json:"action"`
	Details    *string   `json:"details"`
	CreatedBy  *string   `json:"createdBy"`
	CreatedOn  time.Time `json:"createdOn"`
}

type entListTrail struct {
	Trail []entTrailEntry `json:"trail"`
}

// entCreateAIValidation is the entity's CreateAuditAIValidationLogRequest.
type entCreateAIValidation struct {
	ControlID       int      `json:"controlId"`
	Result          string   `json:"result"`
	GapsFound       *string  `json:"gapsFound"`
	Feedback        *string  `json:"feedback"`
	Summary         *string  `json:"summary"`
	ConfidenceScore *float64 `json:"confidenceScore"`
	CreatedBy       string   `json:"createdBy"`
}

// entCreateTrail is the entity's CreateAuditTrailRequest.
type entCreateTrail struct {
	ControlID  *int    `json:"controlId"`
	EvidenceID *int    `json:"evidenceId"`
	Action     string  `json:"action"`
	Details    *string `json:"details"`
	CreatedBy  *string `json:"createdBy"`
}
