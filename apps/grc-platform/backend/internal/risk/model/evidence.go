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

// RiskEvidence represents a file attached to a risk, mapping to `risk_evidence`.
//
// EvidenceType is one of:
//   - ACTION_PLAN_ATTACHMENT ("Risk Evidence Attachment") — uploaded from the
//     Add Risk form; risk-level, ActionPlanID is always nil.
//   - FINAL_APPROVAL_ATTACHMENT ("Risk Action Plan Completion Attachment") —
//     uploaded by the action owner before completing a specific plan;
//     ActionPlanID is always set (a risk can have more than one plan).
type RiskEvidence struct {
	ID           int    `json:"id"`
	RiskID       int    `json:"risk_id"`
	ActionPlanID *int   `json:"action_plan_id,omitempty"`
	FileName     string `json:"file_name"`
	FilePath     string `json:"file_path"`
	Note         string `json:"note"`
	EvidenceType string `json:"evidence_type"`
	CreatedBy    string `json:"created_by"`
	// CreatedByEmail is CreatedBy resolved through the identity directory —
	// see model.HistoryEntry.CreatedByEmail for the same fallback contract.
	CreatedByEmail string    `json:"created_by_email,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	// DownloadURL is populated by the service at read time — the browser's
	// authenticated download endpoint, never a direct Azure/entity URL.
	DownloadURL *string `json:"download_url,omitempty"`
}
