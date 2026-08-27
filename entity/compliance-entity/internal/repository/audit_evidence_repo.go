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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// EvidenceRepository defines persistence operations for audit_evidence and audit_evidence_file.
type EvidenceRepository interface {
	CreateEvidence(ctx context.Context, controlID int, req domain.CreateEvidenceRequest) (*domain.AuditEvidence, error)
	GetEvidenceByID(ctx context.Context, evidenceID int) (*domain.AuditEvidence, error)
	ListEvidenceByControl(ctx context.Context, auditID, controlID int) ([]domain.AuditEvidence, error)
	UpdateEvidence(ctx context.Context, evidenceID int, req domain.UpdateEvidenceRequest) (*domain.AuditEvidence, error)
	AddEvidenceFile(ctx context.Context, evidenceID int, req domain.CreateEvidenceFileRequest) (*domain.AuditEvidenceFile, error)
	ListEvidenceFiles(ctx context.Context, evidenceID int) (*domain.ListEvidenceFilesResponse, error)
	GetEvidenceFileByID(ctx context.Context, fileID int) (*domain.AuditEvidenceFile, error)
	DeleteEvidenceFile(ctx context.Context, fileID int) error
	// DeleteEvidence removes a whole round (audit_evidence row). Its files
	// (audit_evidence_file) and AI validation logs cascade via FK; audit_trail
	// rows referencing it are detached (evidence_id set NULL), not deleted.
	DeleteEvidence(ctx context.Context, evidenceID int) error
}

type evidenceRepo struct{ db *sql.DB }

// NewEvidenceRepository constructs an EvidenceRepository.
func NewEvidenceRepository(db *sql.DB) EvidenceRepository { return &evidenceRepo{db: db} }

func (r *evidenceRepo) CreateEvidence(ctx context.Context, controlID int, req domain.CreateEvidenceRequest) (*domain.AuditEvidence, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_evidence (control_id, reused_from_evidence_id, folder_path, attestation, status, created_by, updated_by)
		 VALUES (?, ?, ?, ?, 'SUBMITTED', ?, ?)`,
		controlID,
		nullableInt(req.ReusedFromEvidenceID),
		req.FolderPath,
		nullableString(req.Attestation),
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("evidence.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetEvidenceByID(ctx, int(id))
}

func (r *evidenceRepo) GetEvidenceByID(ctx context.Context, evidenceID int) (*domain.AuditEvidence, error) {
	var e domain.AuditEvidence
	var reusedFrom sql.NullInt64
	var folderPath, createdBy, attestation sql.NullString
	var controlAuditorID, controlTeamID sql.NullInt64
	// LEFT JOINed through to the owning control's auditor and team — same
	// join as getEvidenceFileByID — so the GRC Backend can authorize
	// evidence-scoped endpoints (e.g. AI validation results) against the
	// assigned auditor or a team-scoped grant without a second round trip.
	err := r.db.QueryRowContext(ctx, `
		SELECT e.id, e.control_id, e.status, e.folder_path, e.reused_from_evidence_id,
		       e.attestation, e.created_by, e.created_at, e.updated_at,
		       c.auditor_id AS control_auditor_id, c.team_id AS control_team_id
		FROM audit_evidence e
		LEFT JOIN audit_control c ON c.id = e.control_id
		WHERE e.id = ?`,
		evidenceID).Scan(&e.ID, &e.ControlID, &e.Status, &folderPath, &reusedFrom, &attestation, &createdBy, &e.CreatedOn, &e.UpdatedOn, &controlAuditorID, &controlTeamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("evidence %d not found", evidenceID)}
	}
	if err != nil {
		return nil, fmt.Errorf("evidence.GetByID(%d): %w", evidenceID, err)
	}
	if folderPath.Valid {
		e.FolderPath = &folderPath.String
	}
	if reusedFrom.Valid {
		v := int(reusedFrom.Int64)
		e.ReusedFromEvidenceID = &v
	}
	if attestation.Valid {
		e.Attestation = &attestation.String
	}
	if createdBy.Valid {
		e.CreatedBy = &createdBy.String
	}
	if controlAuditorID.Valid {
		v := int(controlAuditorID.Int64)
		e.AuditorID = &v
	}
	if controlTeamID.Valid {
		v := int(controlTeamID.Int64)
		e.TeamID = &v
	}
	return &e, nil
}

func (r *evidenceRepo) ListEvidenceByControl(ctx context.Context, auditID, controlID int) ([]domain.AuditEvidence, error) {
	// JOIN audit_control so the control is verified to belong to auditID from the
	// path — a mismatched audit/control pair returns an empty list, not another
	// audit's evidence.
	rows, err := r.db.QueryContext(ctx,
		`SELECT e.id, e.control_id, e.status, e.folder_path,
		        e.reused_from_evidence_id, e.attestation, e.created_by, e.created_at, e.updated_at
		 FROM audit_evidence e
		 JOIN audit_control c ON c.id = e.control_id
		 WHERE e.control_id = ? AND c.audit_id = ?
		 ORDER BY e.created_at DESC`,
		controlID, auditID)
	if err != nil {
		return nil, fmt.Errorf("evidence.ListByControl: %w", err)
	}
	defer rows.Close()

	var evidence []domain.AuditEvidence
	for rows.Next() {
		var e domain.AuditEvidence
		var reusedFrom sql.NullInt64
		var folderPath, createdBy, attestation sql.NullString
		if err := rows.Scan(&e.ID, &e.ControlID, &e.Status, &folderPath, &reusedFrom, &attestation, &createdBy, &e.CreatedOn, &e.UpdatedOn); err != nil {
			return nil, fmt.Errorf("evidence.ListByControl scan: %w", err)
		}
		if folderPath.Valid {
			e.FolderPath = &folderPath.String
		}
		if reusedFrom.Valid {
			v := int(reusedFrom.Int64)
			e.ReusedFromEvidenceID = &v
		}
		if attestation.Valid {
			e.Attestation = &attestation.String
		}
		if createdBy.Valid {
			e.CreatedBy = &createdBy.String
		}
		evidence = append(evidence, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence.ListByControl rows: %w", err)
	}
	return evidence, nil
}

func (r *evidenceRepo) UpdateEvidence(ctx context.Context, evidenceID int, req domain.UpdateEvidenceRequest) (*domain.AuditEvidence, error) {
	sets := []string{"status = ?", "updated_by = ?"}

	var (
		query  string
		args   []any
		result sql.Result
		err    error
	)
	if req.ExpectedStatus != "" {
		args = []any{req.Status, req.UpdatedBy, evidenceID, req.ExpectedStatus}
		query = "UPDATE audit_evidence SET " + strings.Join(sets, ", ") + " WHERE id = ? AND status = ?" // #nosec G202
	} else {
		args = []any{req.Status, req.UpdatedBy, evidenceID}
		query = "UPDATE audit_evidence SET " + strings.Join(sets, ", ") + " WHERE id = ?" // #nosec G202
	}
	if result, err = r.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("evidence.Update(%d): %w", evidenceID, err)
	}
	if req.ExpectedStatus != "" {
		if n, _ := result.RowsAffected(); n == 0 {
			current, err := r.GetEvidenceByID(ctx, evidenceID)
			if err != nil {
				return nil, err // propagates NotFoundError if record was deleted
			}
			if current.Status == req.ExpectedStatus && req.Status == req.ExpectedStatus {
				return current, nil // MySQL no-op: status being set to its current value
			}
			return nil, &apierror.ConflictError{Msg: "evidence was modified concurrently, please retry"}
		}
	}
	return r.GetEvidenceByID(ctx, evidenceID)
}

func (r *evidenceRepo) AddEvidenceFile(ctx context.Context, evidenceID int, req domain.CreateEvidenceFileRequest) (*domain.AuditEvidenceFile, error) {
	// Verify the parent evidence exists so a bad id returns 404, not a raw FK 500.
	var exists int
	if err := r.db.QueryRowContext(ctx,
		"SELECT 1 FROM audit_evidence WHERE id = ?", evidenceID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("evidence %d not found", evidenceID)}
	} else if err != nil {
		return nil, fmt.Errorf("evidence_file.Add parent check: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_evidence_file (evidence_id, file_name, file_path, file_type, file_size, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		evidenceID,
		req.FileName, req.FilePath,
		nullableString(req.FileType),
		req.FileSize,
		req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("evidence_file.Add: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getEvidenceFileByID(ctx, int(id))
}

// GetEvidenceFileByID returns a single evidence file row by its ID.
func (r *evidenceRepo) GetEvidenceFileByID(ctx context.Context, fileID int) (*domain.AuditEvidenceFile, error) {
	return r.getEvidenceFileByID(ctx, fileID)
}

func (r *evidenceRepo) getEvidenceFileByID(ctx context.Context, fileID int) (*domain.AuditEvidenceFile, error) {
	var f domain.AuditEvidenceFile
	var evidenceID, populationID, controlTeamID, controlAuditorID sql.NullInt64
	var fileKind, fileType sql.NullString
	var fileSize sql.NullInt64
	// LEFT JOINed through to the owning control's auditor and team so the GRC
	// Backend can gate downloads to the assigned auditor, or to a team-scoped
	// grant, without a second round trip. All joins miss cleanly for population
	// files (evidence_id NULL) or controls with no auditor/team assigned —
	// control_auditor_id/control_team_id just come back NULL. No `user` join
	// needed: audit_control already carries auditor_id directly.
	err := r.db.QueryRowContext(ctx, `
		SELECT f.id, f.evidence_id, f.population_id, f.file_kind,
		       f.file_name, f.file_path, f.file_type, f.file_size, f.created_at,
		       c.auditor_id AS control_auditor_id, c.team_id AS control_team_id
		FROM audit_evidence_file f
		LEFT JOIN audit_evidence e ON e.id = f.evidence_id
		LEFT JOIN audit_control  c ON c.id = e.control_id
		WHERE f.id = ?`,
		fileID).Scan(&f.ID, &evidenceID, &populationID, &fileKind, &f.FileName, &f.FilePath, &fileType, &fileSize, &f.CreatedOn, &controlAuditorID, &controlTeamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("evidence file %d not found", fileID)}
	}
	if err != nil {
		return nil, fmt.Errorf("evidence_file.GetByID(%d): %w", fileID, err)
	}
	if evidenceID.Valid {
		v := int(evidenceID.Int64)
		f.EvidenceID = &v
	}
	if populationID.Valid {
		v := int(populationID.Int64)
		f.PopulationID = &v
	}
	if fileKind.Valid {
		f.FileKind = &fileKind.String
	}
	if fileType.Valid {
		f.FileType = &fileType.String
	}
	if fileSize.Valid {
		f.FileSize = &fileSize.Int64
	}
	if controlAuditorID.Valid {
		v := int(controlAuditorID.Int64)
		f.AuditorID = &v
	}
	if controlTeamID.Valid {
		v := int(controlTeamID.Int64)
		f.TeamID = &v
	}
	return &f, nil
}

func (r *evidenceRepo) ListEvidenceFiles(ctx context.Context, evidenceID int) (*domain.ListEvidenceFilesResponse, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, evidence_id, population_id, file_kind, file_name, file_path, file_type, file_size, created_at "+
			"FROM audit_evidence_file WHERE evidence_id = ? ORDER BY created_at DESC",
		evidenceID)
	if err != nil {
		return nil, fmt.Errorf("evidence_file.List: %w", err)
	}
	defer rows.Close()

	var files []domain.AuditEvidenceFile
	for rows.Next() {
		var f domain.AuditEvidenceFile
		var evID, popID sql.NullInt64
		var fileKind, fileType sql.NullString
		var fileSize sql.NullInt64
		if err := rows.Scan(&f.ID, &evID, &popID, &fileKind, &f.FileName, &f.FilePath, &fileType, &fileSize, &f.CreatedOn); err != nil {
			return nil, fmt.Errorf("evidence_file.List scan: %w", err)
		}
		if evID.Valid {
			v := int(evID.Int64)
			f.EvidenceID = &v
		}
		if popID.Valid {
			v := int(popID.Int64)
			f.PopulationID = &v
		}
		if fileKind.Valid {
			f.FileKind = &fileKind.String
		}
		if fileType.Valid {
			f.FileType = &fileType.String
		}
		if fileSize.Valid {
			f.FileSize = &fileSize.Int64
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence_file.List rows: %w", err)
	}
	return &domain.ListEvidenceFilesResponse{Files: files}, nil
}

func (r *evidenceRepo) DeleteEvidence(ctx context.Context, evidenceID int) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM audit_evidence WHERE id = ?", evidenceID)
	if err != nil {
		return fmt.Errorf("evidence.Delete(%d): %w", evidenceID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("evidence %d not found", evidenceID)}
	}
	return nil
}

func (r *evidenceRepo) DeleteEvidenceFile(ctx context.Context, fileID int) error {
	// Scope to evidence files only: audit_evidence_file is shared with populations,
	// so require evidence_id IS NOT NULL to prevent this route deleting a population file.
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM audit_evidence_file WHERE id = ? AND evidence_id IS NOT NULL", fileID)
	if err != nil {
		return fmt.Errorf("evidence_file.Delete(%d): %w", fileID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("evidence file %d not found", fileID)}
	}
	return nil
}
