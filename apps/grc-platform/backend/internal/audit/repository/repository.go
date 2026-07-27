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
package repository

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// AuditRepository is the data-access contract for audit engagements.
type AuditRepository interface {
	List(ctx context.Context) ([]*model.Audit, error)
	GetByID(ctx context.Context, id int) (*model.Audit, error)
	Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error)
	Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error
	Delete(ctx context.Context, id int, deletedBy string) error
}

// FrameworkControlRepository is the data-access contract for the versioned framework control library.
type FrameworkControlRepository interface {
	ListCurrent(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error)
	Create(ctx context.Context, frameworkID int, req model.CreateFrameworkControlRequest, createdBy string) (*model.AuditFrameworkControl, error)
}

// FrameworkRepository is the data-access contract for audit frameworks.
type FrameworkRepository interface {
	List(ctx context.Context) ([]*model.AuditFramework, error)
	GetByID(ctx context.Context, id int) (*model.AuditFramework, error)
	Create(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error)
}

// ProductRepository is the data-access contract for audit products.
type ProductRepository interface {
	List(ctx context.Context) ([]*model.AuditProduct, error)
	GetByID(ctx context.Context, id int) (*model.AuditProduct, error)
	Create(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error)
}

// ControlRepository is the data-access contract for audit controls.
type ControlRepository interface {
	List(ctx context.Context, auditID int) ([]*model.AuditControl, error)
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
	Create(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error)
	BulkCreate(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error)
	Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error
	UpdateStatus(ctx context.Context, auditID, controlID int, status string, comment *string, updatedBy string) error
	// UpdateStatusWithSample atomically sets the control's status and sample_reference
	// in one call — used when the auditor submits the sample, so the two can never
	// be observed out of step with each other.
	UpdateStatusWithSample(ctx context.Context, auditID, controlID int, status string, sampleReference string, updatedBy string) error
	Delete(ctx context.Context, auditID, controlID int) error
	// ListAssignedForEvidence returns all controls assigned to the team of userEmail
	// that are in a status requiring evidence submission.
	ListAssignedForEvidence(ctx context.Context, userEmail string) ([]*model.AssignedControlForEvidence, error)
	// AssignedAuditID reports whether userEmail's team is assigned to controlID for
	// an actionable status, and returns the control's audit id (for server-side
	// folder-path derivation). found=false means not assigned (403).
	AssignedAuditID(ctx context.Context, userEmail string, controlID int) (auditID int, found bool, err error)
	// ActivePopulationID returns the active population round for an OE control.
	// found=false means no active population (e.g. a DESIGN control).
	ActivePopulationID(ctx context.Context, controlID int) (populationID int, found bool, err error)
}

// UserRepository is the data-access contract for the shared user list (owner/auditor dropdowns).
type UserRepository interface {
	List(ctx context.Context) ([]*model.UserRef, error)
}

// TeamRepository is the data-access contract for the audit team list.
type TeamRepository interface {
	List(ctx context.Context) ([]*model.AuditTeam, error)
}

// DashboardRepository aggregates cross-cutting dashboard stats and action items.
type DashboardRepository interface {
	Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error)
	GetWorkQueuePage(ctx context.Context, f model.DashboardFilter, tab model.WorkQueueTab, page, limit int) (*model.WorkQueuePage, error)
}

// EvidenceRepository is the data-access contract for audit evidence submissions.
type EvidenceRepository interface {
	// Create inserts a new evidence row for the given control and returns its ID.
	Create(ctx context.Context, auditID, controlID int, folderPath, createdBy string) (int, error)
	// AddFile inserts a single audit_evidence_file row linked to evidenceID.
	AddFile(ctx context.Context, evidenceID int, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error
	// DeleteEvidence removes an evidence row by ID (used for best-effort rollback on partial failure).
	DeleteEvidence(ctx context.Context, evidenceID int) error
	// ListByControl returns all evidence submissions for a control, newest first, with files pre-loaded.
	ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error)
	// GetFileByID returns a single evidence file row by its ID (for downloads).
	GetFileByID(ctx context.Context, fileID int) (*model.AuditEvidenceFile, error)
	// DeleteFile removes a single evidence file row by ID.
	DeleteFile(ctx context.Context, fileID int) error
}

// PopulationRepository is the data-access contract for OE-control population
// submissions (used by the Evidence Portal population flow and the population
// review/validate/sample web-app routes).
type PopulationRepository interface {
	// AddFile records one uploaded population blob against a population round.
	AddFile(ctx context.Context, populationID int, fileKind, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error
	// UpdateStatus advances the population round's status (e.g. → SUBMITTED).
	UpdateStatus(ctx context.Context, populationID int, status, updatedBy string) error
	// ListByControl returns every population round for a control, oldest first.
	ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditPopulation, error)
	// ListFiles returns all files on a population round, newest first.
	ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error)
	// GetFileByID returns a single population/sample file row by its ID (for downloads).
	GetFileByID(ctx context.Context, fileID int) (*model.PopulationFile, error)
	// DeleteFile removes a single population/sample file row by ID.
	DeleteFile(ctx context.Context, fileID int) error
}

// CommentRepository is the data-access contract for audit_comment (evidence-scoped).
type CommentRepository interface {
	Create(ctx context.Context, evidenceID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error)
	ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AuditComment, error)
}
type AssignmentRepository interface{}
type NotificationRepository interface{}

// TrailRepository appends to and reads the append-only audit_trail (via the entity).
type TrailRepository interface {
	// Create appends one audit_trail entry under auditID. controlID/evidenceID are
	// optional; details is a raw JSON string (may be empty).
	Create(ctx context.Context, auditID int, controlID, evidenceID *int, action, details, createdBy string) error
	// ListByControl returns the trail entries for one control, newest first, along
	// with the total count. limit caps the number of entries returned.
	ListByControl(ctx context.Context, auditID, controlID, limit int) ([]*model.AuditTrailEntry, int, error)
}

// AIValidationLogRepository reads AI evidence-validation rows from the
// Compliance Entity (advisory hints written by the async validation agent).
type AIValidationLogRepository interface {
	ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AIValidationLog, error)
}

// ReviewRepository is the data-access contract for audit_item_review.
// TODO: add Review/List/GetByID methods as the table schema is finalised.
type ReviewRepository interface{}
