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

// AIValidationLog is one AI evidence-validation run, mirrored from the
// Compliance Entity's audit_ai_validation_log. It is advisory only — the
// frontend renders the latest row as a review hint and never gates the
// workflow on it. Result is PASS | FAIL | UNCERTAIN | PENDING | ERROR.
type AIValidationLog struct {
	ID              int64     `json:"id"`
	EvidenceID      int       `json:"evidenceId"`
	ControlID       int       `json:"controlId"`
	Result          string    `json:"result"`
	GapsFound       *string   `json:"gapsFound"` // JSON array of gap objects
	Feedback        *string   `json:"feedback"`  // JSON array of submitter-facing action strings
	Summary         *string   `json:"summary"`
	ConfidenceScore *float64  `json:"confidenceScore"`
	CreatedBy       *string   `json:"createdBy"`
	CreatedOn       time.Time `json:"createdOn"`
}

// AIValidationListResponse is the payload of
// GET /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/ai-validations
// (latest row first).
type AIValidationListResponse struct {
	Validations []*AIValidationLog `json:"validations"`
}
