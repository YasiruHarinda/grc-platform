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
	CreateComment(ctx context.Context, controlID int, req domain.CreateAuditCommentRequest) (*domain.AuditComment, error)
	ListCommentsByControl(ctx context.Context, controlID int) ([]domain.AuditComment, error)
	DeleteComment(ctx context.Context, commentID int) error
	// ResolveAuditID looks up the audit a control belongs to. Used to attribute
	// a COMMENTED audit_trail row to the right audit, since audit_comment
	// itself only stores control_id.
	ResolveAuditID(ctx context.Context, controlID int) (auditID int, err error)
}

type commentRepo struct{ db *sql.DB }

// NewCommentRepository constructs a CommentRepository.
func NewCommentRepository(db *sql.DB) CommentRepository { return &commentRepo{db: db} }

func (r *commentRepo) CreateComment(ctx context.Context, controlID int, req domain.CreateAuditCommentRequest) (*domain.AuditComment, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_comment
		 (control_id, author_id, parent_comment_id, content, is_internal, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		controlID,
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

const commentSelectCols = `
	        c.id, c.control_id, c.author_id, c.parent_comment_id, c.content, c.is_internal,
	        c.created_by, u_author.user_type AS author_user_type, c.created_at, c.updated_at`

const commentFromClause = `
	 FROM audit_comment c
	 LEFT JOIN ` + "`user`" + ` u_author ON u_author.id = c.author_id`

func (r *commentRepo) getCommentByID(ctx context.Context, id int) (*domain.AuditComment, error) {
	return scanAuditComment(r.db.QueryRowContext(ctx,
		`SELECT`+commentSelectCols+commentFromClause+` WHERE c.id = ?`, id))
}

func (r *commentRepo) ListCommentsByControl(ctx context.Context, controlID int) ([]domain.AuditComment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT`+commentSelectCols+commentFromClause+` WHERE c.control_id = ? ORDER BY c.created_at ASC`,
		controlID)
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

func (r *commentRepo) ResolveAuditID(ctx context.Context, controlID int) (int, error) {
	var auditID int
	err := r.db.QueryRowContext(ctx,
		`SELECT audit_id FROM audit_control WHERE id = ?`, controlID).Scan(&auditID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, &apierror.NotFoundError{Msg: "control not found"}
	}
	if err != nil {
		return 0, fmt.Errorf("comment.ResolveAuditID(%d): %w", controlID, err)
	}
	return auditID, nil
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
	var createdBy, authorUserType sql.NullString
	err := s.Scan(
		&c.ID, &c.ControlID, &authorID, &parentID,
		&c.Content, &c.IsInternal, &createdBy, &authorUserType, &c.CreatedOn, &c.UpdatedOn,
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
	if authorUserType.Valid {
		c.CreatedByUserType = &authorUserType.String
	}
	return &c, nil
}
