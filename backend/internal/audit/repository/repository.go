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

// Package repository defines the data-access contracts for the Audit Hub module.
// Add methods to each interface as handlers are implemented.
package repository

// AuditRepository is the data-access contract for audit engagements.
// TODO: add CRUD and status-transition methods based on AUDIT_MODULE_DESIGN.md
type AuditRepository interface{}

// FrameworkRepository is the data-access contract for audit frameworks.
// TODO: add CRUD methods
type FrameworkRepository interface{}

// ProductRepository is the data-access contract for audit products.
// TODO: add CRUD methods
type ProductRepository interface{}

// ControlRepository is the data-access contract for audit controls.
// TODO: add CRUD and status-transition methods
type ControlRepository interface{}

// PopulationRepository is the data-access contract for audit population samples.
// TODO: add CRUD methods
type PopulationRepository interface{}

// EvidenceRepository is the data-access contract for audit evidence and file attachments.
// TODO: add CRUD and file-link methods
type EvidenceRepository interface{}

// CommentRepository is the data-access contract for audit comments.
// TODO: add CRUD methods
type CommentRepository interface{}

// ReviewRepository is the data-access contract for auditor review decisions.
// TODO: add CRUD methods
type ReviewRepository interface{}

// AssignmentRepository is the data-access contract for auditor assignments.
// TODO: add CRUD methods
type AssignmentRepository interface{}

// NotificationRepository is the data-access contract for audit notifications.
// TODO: add list and mark-read methods
type NotificationRepository interface{}

// AIValidationLogRepository is the data-access contract for AI validation log records.
// TODO: add insert and list methods
type AIValidationLogRepository interface{}

// TrailRepository is the data-access contract for the immutable audit trail.
// TODO: add insert and list methods
type TrailRepository interface{}
