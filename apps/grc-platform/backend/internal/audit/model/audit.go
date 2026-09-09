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

// AuditFrameworkRef is the lightweight framework embedded in Audit list/detail responses.
type AuditFrameworkRef struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Version *string `json:"version"`
}

// AuditProductRef is the lightweight product embedded in Audit list/detail responses.
type AuditProductRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ControlCounts holds aggregate control counters shown on each audit card.
type ControlCounts struct {
	Total    int `json:"total"`
	Approved int `json:"approved"`
	Overdue  int `json:"overdue"`
}

// Audit represents a GRC audit engagement.
type Audit struct {
	ID               int               `json:"id"`
	Name             string            `json:"name"`
	Framework        AuditFrameworkRef `json:"framework"`
	Product          AuditProductRef   `json:"product"`
	PeriodStart      string            `json:"periodStart"`
	PeriodEnd        string            `json:"periodEnd"`
	Status           string            `json:"status"`
	ScopeDescription *string           `json:"scopeDescription"`
	ControlCounts    ControlCounts     `json:"controlCounts"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

// IsOngoing reports whether the audit still counts as live work. ARCHIVED is
// ongoing; only COMPLETED and REMOVED are not. The reminder job and the
// departure sweep must agree on this, so both ask here.
func (a *Audit) IsOngoing() bool {
	return a.Status != "COMPLETED" && a.Status != "REMOVED"
}

// AuditListResponse is returned by GET /api/v1/audits.
type AuditListResponse struct {
	Items []*Audit `json:"items"`
	Total int      `json:"total"`
}

// CreateAuditRequest is the payload for POST /api/v1/audits.
type CreateAuditRequest struct {
	Name             string  `json:"name"`
	FrameworkID      int     `json:"frameworkId"`
	ProductID        int     `json:"productId"`
	PeriodStart      string  `json:"periodStart"`
	PeriodEnd        string  `json:"periodEnd"`
	ScopeDescription *string `json:"scopeDescription"`
}

// UpdateAuditRequest is the payload for PUT /api/v1/audits/{id}.
// All fields are optional; nil means "do not change".
type UpdateAuditRequest struct {
	Name             *string `json:"name"`
	PeriodStart      *string `json:"periodStart"`
	PeriodEnd        *string `json:"periodEnd"`
	ScopeDescription *string `json:"scopeDescription"`
	Status           *string `json:"status"`
}
