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

// AuditComment represents a threaded comment on a control or evidence.
// TODO: add fields based on `audit_comment` in audit_schema.sql
type AuditComment struct{}

// AddCommentRequest is the payload for POST .../comments.
// TODO: define fields (content, parentId for threading, isInternal)
type AddCommentRequest struct{}
