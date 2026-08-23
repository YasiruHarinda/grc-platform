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

// PopulationRepository defines persistence operations for audit_population and its files.
type PopulationRepository interface {
	CreatePopulation(ctx context.Context, auditID, controlID int, req domain.CreatePopulationRequest) (*domain.AuditPopulation, error)
	GetPopulationByID(ctx context.Context, populationID int) (*domain.AuditPopulation, error)
	ListPopulations(ctx context.Context, auditID, controlID int) ([]domain.AuditPopulation, error)
	UpdatePopulation(ctx context.Context, populationID int, req domain.UpdatePopulationRequest) (*domain.AuditPopulation, error)
	AddPopulationFile(ctx context.Context, populationID int, req domain.CreatePopulationFileRequest) (*domain.AuditEvidenceFile, error)
	ListPopulationFiles(ctx context.Context, populationID int) ([]domain.AuditEvidenceFile, error)
	GetPopulationFileByID(ctx context.Context, fileID int) (*domain.AuditEvidenceFile, error)
	DeletePopulationFile(ctx context.Context, fileID int) error
}

type populationRepo struct{ db *sql.DB }

// NewPopulationRepository constructs a PopulationRepository.
func NewPopulationRepository(db *sql.DB) PopulationRepository { return &populationRepo{db: db} }

func (r *populationRepo) CreatePopulation(ctx context.Context, auditID, controlID int, req domain.CreatePopulationRequest) (*domain.AuditPopulation, error) {
	var exists int
	if err := r.db.QueryRowContext(ctx,
		"SELECT 1 FROM audit_control WHERE id = ? AND audit_id = ?", controlID, auditID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
	} else if err != nil {
		return nil, fmt.Errorf("population.Create parent check: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_population
		 (control_id, owner_id, team_id, reference_number, description, due_date, status, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
		controlID,
		nullableInt(req.OwnerID), nullableInt(req.TeamID),
		req.ReferenceNumber, nullableString(req.Description),
		req.DueDate,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("population.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetPopulationByID(ctx, int(id))
}

func (r *populationRepo) GetPopulationByID(ctx context.Context, populationID int) (*domain.AuditPopulation, error) {
	pop, err := scanPopulationRow(r.db.QueryRowContext(ctx,
		`SELECT id, control_id, owner_id, team_id, reference_number, description,
		        status, DATE_FORMAT(due_date,'%Y-%m-%d'), comments, attestation, created_at, updated_at
		 FROM audit_population WHERE id = ?`, populationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("population %d not found", populationID)}
	}
	if err != nil {
		return nil, fmt.Errorf("population.GetByID(%d): %w", populationID, err)
	}
	return pop, nil
}

func (r *populationRepo) ListPopulations(ctx context.Context, auditID, controlID int) ([]domain.AuditPopulation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.control_id, p.owner_id, p.team_id, p.reference_number, p.description,
		        p.status, DATE_FORMAT(p.due_date,'%Y-%m-%d'), p.comments, p.attestation, p.created_at, p.updated_at
		 FROM audit_population p
		 JOIN audit_control c ON c.id = p.control_id
		 WHERE p.control_id = ? AND c.audit_id = ? ORDER BY p.id`,
		controlID, auditID)
	if err != nil {
		return nil, fmt.Errorf("population.List: %w", err)
	}
	defer rows.Close()

	var pops []domain.AuditPopulation
	for rows.Next() {
		pop, err := scanPopulationRow(rows)
		if err != nil {
			return nil, fmt.Errorf("population.List scan: %w", err)
		}
		pops = append(pops, *pop)
	}
	return pops, rows.Err()
}

func (r *populationRepo) UpdatePopulation(ctx context.Context, populationID int, req domain.UpdatePopulationRequest) (*domain.AuditPopulation, error) {
	sets := []string{}
	args := []any{}

	if req.OwnerID != nil {
		sets = append(sets, "owner_id = ?")
		args = append(args, *req.OwnerID)
	}
	if req.TeamID != nil {
		sets = append(sets, "team_id = ?")
		args = append(args, *req.TeamID)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	if req.Comments != nil {
		sets = append(sets, "comments = ?")
		args = append(args, *req.Comments)
	}
	if req.ReferenceNumber != nil {
		sets = append(sets, "reference_number = ?")
		args = append(args, *req.ReferenceNumber)
	}
	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.DueDate != nil {
		sets = append(sets, "due_date = ?")
		args = append(args, *req.DueDate)
	}
	if req.Attestation != nil {
		sets = append(sets, "attestation = ?")
		args = append(args, *req.Attestation)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)

	var (
		query  string
		result sql.Result
		err    error
	)
	if req.ExpectedStatus != "" {
		args = append(args, populationID, req.ExpectedStatus)
		query = "UPDATE audit_population SET " + strings.Join(sets, ", ") + " WHERE id = ? AND status = ?" // #nosec G202
	} else {
		args = append(args, populationID)
		query = "UPDATE audit_population SET " + strings.Join(sets, ", ") + " WHERE id = ?" // #nosec G202
	}
	if result, err = r.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("population.Update(%d): %w", populationID, err)
	}
	if req.ExpectedStatus != "" {
		if n, _ := result.RowsAffected(); n == 0 {
			current, err := r.GetPopulationByID(ctx, populationID)
			if err != nil {
				return nil, err // propagates NotFoundError if record was deleted
			}
			if current.Status == req.ExpectedStatus && (req.Status == nil || *req.Status == req.ExpectedStatus) {
				return current, nil // MySQL no-op: status not being changed, or being set to its current value
			}
			return nil, &apierror.ConflictError{Msg: "population was modified concurrently, please retry"}
		}
	}
	return r.GetPopulationByID(ctx, populationID)
}

func (r *populationRepo) AddPopulationFile(ctx context.Context, populationID int, req domain.CreatePopulationFileRequest) (*domain.AuditEvidenceFile, error) {
	// Verify the parent population exists so a bad id returns 404, not a raw FK 500.
	var exists int
	if err := r.db.QueryRowContext(ctx,
		"SELECT 1 FROM audit_population WHERE id = ?", populationID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("population %d not found", populationID)}
	} else if err != nil {
		return nil, fmt.Errorf("population_file.Add parent check: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_evidence_file (population_id, file_kind, file_name, file_path, file_type, file_size, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		populationID,
		req.FileKind, req.FileName, req.FilePath,
		nullableString(req.FileType),
		req.FileSize,
		req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("population_file.Add: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getPopulationFileByID(ctx, int(id))
}

// GetPopulationFileByID returns a single population/sample file row by ID (used
// for downloads — the caller reads the actual blob bytes via FilePath).
func (r *populationRepo) GetPopulationFileByID(ctx context.Context, fileID int) (*domain.AuditEvidenceFile, error) {
	return r.getPopulationFileByID(ctx, fileID)
}

func (r *populationRepo) getPopulationFileByID(ctx context.Context, fileID int) (*domain.AuditEvidenceFile, error) {
	var f domain.AuditEvidenceFile
	var evidenceID, populationID, controlTeamID sql.NullInt64
	var fileKind, fileType sql.NullString
	var fileSize sql.NullInt64
	// LEFT JOINed through to the owning control's team so the GRC Backend can
	// gate downloads to a team-scoped grant instead of an unscoped privilege
	// union, mirroring audit_evidence_repo.go's getEvidenceFileByID. Misses
	// cleanly for a control with no team — control_team_id just comes back NULL.
	err := r.db.QueryRowContext(ctx, `
		SELECT f.id, f.evidence_id, f.population_id, f.file_kind,
		       f.file_name, f.file_path, f.file_type, f.file_size, f.created_at,
		       c.team_id AS control_team_id
		FROM audit_evidence_file f
		LEFT JOIN audit_population p ON p.id = f.population_id
		LEFT JOIN audit_control    c ON c.id = p.control_id
		WHERE f.id = ?`,
		fileID).Scan(&f.ID, &evidenceID, &populationID, &fileKind, &f.FileName, &f.FilePath, &fileType, &fileSize, &f.CreatedOn, &controlTeamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("population file %d not found", fileID)}
	}
	if err != nil {
		return nil, fmt.Errorf("population_file.GetByID(%d): %w", fileID, err)
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
	if controlTeamID.Valid {
		v := int(controlTeamID.Int64)
		f.TeamID = &v
	}
	return &f, nil
}

func (r *populationRepo) ListPopulationFiles(ctx context.Context, populationID int) ([]domain.AuditEvidenceFile, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, evidence_id, population_id, file_kind, file_name, file_path, file_type, file_size, created_at "+
			"FROM audit_evidence_file WHERE population_id = ? ORDER BY created_at DESC",
		populationID)
	if err != nil {
		return nil, fmt.Errorf("population_file.List: %w", err)
	}
	defer rows.Close()

	var files []domain.AuditEvidenceFile
	for rows.Next() {
		var f domain.AuditEvidenceFile
		var evID, popID sql.NullInt64
		var fileKind, fileType sql.NullString
		var fileSize sql.NullInt64
		if err := rows.Scan(&f.ID, &evID, &popID, &fileKind, &f.FileName, &f.FilePath, &fileType, &fileSize, &f.CreatedOn); err != nil {
			return nil, fmt.Errorf("population_file.List scan: %w", err)
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
	return files, rows.Err()
}

func (r *populationRepo) DeletePopulationFile(ctx context.Context, fileID int) error {
	// Scope to population files only: audit_evidence_file is shared with evidence,
	// so require population_id IS NOT NULL to prevent this route deleting an evidence file.
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM audit_evidence_file WHERE id = ? AND population_id IS NOT NULL", fileID)
	if err != nil {
		return fmt.Errorf("population_file.Delete(%d): %w", fileID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("population file %d not found", fileID)}
	}
	return nil
}

func scanPopulationRow(s scanner) (*domain.AuditPopulation, error) {
	var p domain.AuditPopulation
	var ownerID, teamID, refNum sql.NullInt64
	var desc, dueDate, comments, attestation sql.NullString
	err := s.Scan(
		&p.ID, &p.ControlID,
		&ownerID, &teamID, &refNum, &desc,
		&p.Status, &dueDate, &comments, &attestation,
		&p.CreatedOn, &p.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	if ownerID.Valid {
		v := int(ownerID.Int64)
		p.OwnerID = &v
	}
	if teamID.Valid {
		v := int(teamID.Int64)
		p.TeamID = &v
	}
	if refNum.Valid {
		v := int(refNum.Int64)
		p.ReferenceNumber = &v
	}
	if desc.Valid {
		p.Description = &desc.String
	}
	if dueDate.Valid {
		p.DueDate = &dueDate.String
	}
	if comments.Valid {
		p.Comments = &comments.String
	}
	if attestation.Valid {
		p.Attestation = &attestation.String
	}
	return &p, nil
}
