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
	// List returns every audit, unscoped — for internal callers that need the
	// full picture regardless of the caller's row scope (e.g. name-uniqueness
	// checks). The Audits tab must NOT use this; see ListScoped.
	List(ctx context.Context) ([]*model.Audit, error)
	// ListScoped returns only audits with at least one control within scope —
	// audits have no team/owner/auditor of their own (only their controls do),
	// so scope is evaluated per ADR-0002 at the control level and an audit
	// qualifies if any of its controls do. Used by the Audits tab (listAudits).
	ListScoped(ctx context.Context, scope model.Scope, userEmail string) ([]*model.Audit, error)
	GetByID(ctx context.Context, id int) (*model.Audit, error)
	// InScope reports whether id is within scope for userEmail — used by
	// getAudit to reject out-of-scope direct links (a control-guessing IDOR)
	// without fetching every audit just to check membership.
	InScope(ctx context.Context, id int, scope model.Scope, userEmail string) (bool, error)
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
	// List returns frameworks with at least one audit in scope — a framework
	// has no team/owner/auditor of its own (only its audits' controls do), so
	// scope is evaluated per ADR-0002 at the control level, one level deeper
	// than ListScoped does for audits.
	List(ctx context.Context, scope model.Scope, userEmail string) ([]*model.AuditFramework, error)
	// GetByID is intentionally unscoped — used internally to validate a
	// frameworkId reference (e.g. audit creation), which must succeed
	// regardless of the caller's row scope.
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
	// List returns every control in auditID, unscoped — for internal callers
	// that need the full picture regardless of the caller's row scope (e.g.
	// control-number-uniqueness checks). The Audits tab must NOT use this;
	// see ListScoped.
	List(ctx context.Context, auditID int) ([]*model.AuditControl, error)
	// ListScoped returns auditID's controls visible to userEmail at scope —
	// used by the Audits tab (listControls); see ADR-0002.
	ListScoped(ctx context.Context, auditID int, scope model.Scope, userEmail string) ([]*model.AuditControl, error)
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
	// InScope reports whether controlID is within scope for userEmail — used
	// by getControl to reject out-of-scope direct links (an IDOR via guessed
	// control ids) without listing every control in the audit to check.
	InScope(ctx context.Context, auditID, controlID int, scope model.Scope, userEmail string) (bool, error)
	Create(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error)
	BulkCreate(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error)
	Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error
	UpdateStatus(ctx context.Context, auditID, controlID int, status string, comment *string, updatedBy string) error
	// UpdateStatusWithSample atomically sets the control's status and sample_reference
	// in one call — used when the auditor submits the sample, so the two can never
	// be observed out of step with each other.
	UpdateStatusWithSample(ctx context.Context, auditID, controlID int, status string, sampleReference string, updatedBy string) error
	Delete(ctx context.Context, auditID, controlID int) error
	// AssignedAuditID reports whether userEmail is the owner of controlID for
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
	// UpdateStatus advances one evidence round's own status (distinct from the
	// control's status) — e.g. SUBMITTED → COMPLIANCE_REJECTED.
	UpdateStatus(ctx context.Context, evidenceID int, status, updatedBy string) error
}

// PopulationRepository is the data-access contract for OE-control population
// submissions (used by the Evidence Portal population flow and the population
// review/validate/sample web-app routes).
type PopulationRepository interface {
	// AddFile records one uploaded population blob against a population round.
	AddFile(ctx context.Context, populationID int, fileKind, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error
	// UpdateStatus advances the population round's status (e.g. → SUBMITTED).
	UpdateStatus(ctx context.Context, populationID int, status, updatedBy string) error
	// UpdateDetails edits a population round's requirement text, due date,
	// comments, owner, and team — used when a manager edits an OE control's
	// population details from the same form used to create them.
	UpdateDetails(ctx context.Context, populationID int, details model.PopulationDetails, updatedBy string) error
	// ListByControl returns every population round for a control, oldest first.
	ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditPopulation, error)
	// ListFiles returns all files on a population round, newest first.
	ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error)
	// GetFileByID returns a single population/sample file row by its ID (for downloads).
	GetFileByID(ctx context.Context, fileID int) (*model.PopulationFile, error)
	// DeleteFile removes a single population/sample file row by ID.
	DeleteFile(ctx context.Context, fileID int) error
}

// CommentRepository is the data-access contract for audit_comment
// (control-scoped — one thread per control, spanning population and
// evidence phases).
type CommentRepository interface {
	Create(ctx context.Context, auditID, controlID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error)
	ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditComment, error)
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
	// ListByAudit returns the whole audit's trail (audit-level and every control's
	// events together), newest first, narrowed by filter, for the audit-wide
	// activity log.
	ListByAudit(ctx context.Context, auditID int, filter model.TrailFilter, limit, offset int) ([]*model.AuditTrailEntry, int, error)
}

// AIValidationLogRepository reads AI evidence-validation rows from the
// Compliance Entity (advisory hints written by the async validation agent).
type AIValidationLogRepository interface {
	ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AIValidationLog, error)
}

// ReviewRepository is the data-access contract for audit_item_review.
// TODO: add Review/List/GetByID methods as the table schema is finalised.
type ReviewRepository interface{}
