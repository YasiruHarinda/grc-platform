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

// AuditFrameworkControl is one immutable version of a control definition
// from the audit_framework_control table.
type AuditFrameworkControl struct {
	ID                  int       `json:"id"`
	FrameworkID         int       `json:"frameworkId"`
	ControlNumber       string    `json:"controlNumber"`
	Description         string    `json:"description"`
	EvidenceRequirement *string   `json:"evidenceRequirement"`
	RequirementType     string    `json:"requirementType"`
	ControlType         string    `json:"controlType"`
	Scope               string    `json:"scope"`
	Version             int       `json:"version"`
	IsCurrent           bool      `json:"isCurrent"`
	CreatedOn           time.Time `json:"createdOn"`
}

// FrameworkControlListResponse is returned by
// GET /api/v1/audits/frameworks/controls?frameworkId={id}.
type FrameworkControlListResponse struct {
	Controls []*AuditFrameworkControl `json:"controls"`
	Total    int                      `json:"total"`
}

// CreateFrameworkControlRequest is the payload for
// POST /api/v1/audits/frameworks/controls.
type CreateFrameworkControlRequest struct {
	FrameworkID         int     `json:"frameworkId"`
	ControlNumber       string  `json:"controlNumber"`
	Description         string  `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     string  `json:"requirementType"` // DESIGN | OE
	ControlType         string  `json:"controlType"`     // CONFIG | NON_CONFIG
	Scope               string  `json:"scope"`           // COMMON | PRODUCT_SPECIFIC
}
