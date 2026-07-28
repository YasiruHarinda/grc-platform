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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// CommentRepository defines persistence for audit_comment.
type CommentRepository interface {
	CreateComment(ctx context.Context, evidenceID int, req domain.CreateAuditCommentRequest) (*domain.AuditComment, error)
	ListCommentsByEvidence(ctx context.Context, evidenceID int) ([]domain.AuditComment, error)
	DeleteComment(ctx context.Context, commentID int) error
	// ResolveControlAndAudit looks up the control and audit a piece of evidence
	// belongs to, via audit_evidence.control_id -> audit_control.audit_id. Used
	// to attribute a COMMENTED audit_trail row to the right audit/control, since
	// audit_comment itself only stores evidence_id.
	ResolveControlAndAudit(ctx context.Context, evidenceID int) (controlID, auditID int, err error)
}

type commentRepo struct{ db *sql.DB }

// NewCommentRepository constructs a CommentRepository.
func NewCommentRepository(db *sql.DB) CommentRepository { return &commentRepo{db: db} }

func (r *commentRepo) CreateComment(ctx context.Context, evidenceID int, req domain.CreateAuditCommentRequest) (*domain.AuditComment, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_comment
		 (evidence_id, author_id, parent_comment_id, content, is_internal, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		evidenceID,
		nullableInt(req.AuthorID),
		nullableInt(req.ParentCommentID),
		req.Content,
		req.IsInternal,
		nullableString(&req.CreatedBy),
		nullableString(&req.CreatedBy),
	)
	if err != nil {
		return nil, fmt.Errorf("comment.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getCommentByID(ctx, int(id))
}

func (r *commentRepo) getCommentByID(ctx context.Context, id int) (*domain.AuditComment, error) {
	return scanAuditComment(r.db.QueryRowContext(ctx,
		`SELECT id, evidence_id, author_id, parent_comment_id, content, is_internal,
		        created_by, created_at, updated_at
		 FROM audit_comment WHERE id = ?`, id))
}

func (r *commentRepo) ListCommentsByEvidence(ctx context.Context, evidenceID int) ([]domain.AuditComment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, evidence_id, author_id, parent_comment_id, content, is_internal,
		        created_by, created_at, updated_at
		 FROM audit_comment WHERE evidence_id = ? ORDER BY created_at ASC`,
		evidenceID)
	if err != nil {
		return nil, fmt.Errorf("comment.List: %w", err)
	}
	defer rows.Close()

	var comments []domain.AuditComment
	for rows.Next() {
		c, err := scanAuditComment(rows)
		if err != nil {
			return nil, fmt.Errorf("comment.List scan: %w", err)
		}
		comments = append(comments, *c)
	}
	return comments, rows.Err()
}

func (r *commentRepo) ResolveControlAndAudit(ctx context.Context, evidenceID int) (int, int, error) {
	var controlID, auditID int
	err := r.db.QueryRowContext(ctx,
		`SELECT ac.id, ac.audit_id
		 FROM audit_evidence ae
		 JOIN audit_control ac ON ac.id = ae.control_id
		 WHERE ae.id = ?`, evidenceID).Scan(&controlID, &auditID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, &apierror.NotFoundError{Msg: "evidence not found"}
	}
	if err != nil {
		return 0, 0, fmt.Errorf("comment.ResolveControlAndAudit(%d): %w", evidenceID, err)
	}
	return controlID, auditID, nil
}

func (r *commentRepo) DeleteComment(ctx context.Context, commentID int) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM audit_comment WHERE id = ?", commentID)
	if err != nil {
		return fmt.Errorf("comment.Delete(%d): %w", commentID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("comment %d not found", commentID)}
	}
	return nil
}

func scanAuditComment(s scanner) (*domain.AuditComment, error) {
	var c domain.AuditComment
	var authorID, parentID sql.NullInt64
	var createdBy sql.NullString
	err := s.Scan(
		&c.ID, &c.EvidenceID, &authorID, &parentID,
		&c.Content, &c.IsInternal, &createdBy, &c.CreatedOn, &c.UpdatedOn,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: "comment not found"}
	}
	if err != nil {
		return nil, err
	}
	if authorID.Valid {
		v := int(authorID.Int64)
		c.AuthorID = &v
	}
	if parentID.Valid {
		v := int(parentID.Int64)
		c.ParentCommentID = &v
	}
	if createdBy.Valid {
		c.CreatedBy = &createdBy.String
	}
	return &c, nil
}
