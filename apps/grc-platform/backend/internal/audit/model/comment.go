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

// AuditComment is a comment on a control (audit_comment) — one thread per
// control, spanning both the population and evidence phases. IsInternal
// hides the comment from external auditors.
type AuditComment struct {
	ID              int       `json:"id"`
	ControlID       int       `json:"controlId"`
	ParentCommentID *int      `json:"parentCommentId"`
	Content         string    `json:"content"`
	IsInternal      bool      `json:"isInternal"`
	CreatedBy       string    `json:"createdBy"`
	CreatedAt       time.Time `json:"createdAt"`
}

// AddCommentRequest is the payload for POST /api/v1/audits/{id}/controls/{controlId}/comments.
type AddCommentRequest struct {
	Content         string `json:"content"`
	IsInternal      bool   `json:"isInternal"`
	ParentCommentID *int   `json:"parentCommentId"`
}

// CommentListResponse is returned by GET /api/v1/audits/{id}/controls/{controlId}/comments.
type CommentListResponse struct {
	Items []*AuditComment `json:"items"`
}
