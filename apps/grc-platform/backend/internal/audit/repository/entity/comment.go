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

package entity

import (
	"context"
	"fmt"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type commentRepo struct{ c *entityclient.Client }

// NewCommentRepository returns an entity-backed CommentRepository.
func NewCommentRepository(c *entityclient.Client) repository.CommentRepository {
	return &commentRepo{c: c}
}

// entComment mirrors the entity's AuditComment JSON (createdOn / *createdBy)
// which differs from the backend model (createdAt / createdBy string) — see
// entEvidence in evidence.go for the same pattern.
type entComment struct {
	ID                int       `json:"id"`
	ControlID         int       `json:"controlId"`
	ParentCommentID   *int      `json:"parentCommentId"`
	Content           string    `json:"content"`
	IsInternal        bool      `json:"isInternal"`
	CreatedBy         *string   `json:"createdBy"`
	CreatedByUserType *string   `json:"createdByUserType"`
	CreatedOn         time.Time `json:"createdOn"`
}

func (c entComment) toModel() *model.AuditComment {
	m := &model.AuditComment{
		ID:              c.ID,
		ControlID:       c.ControlID,
		ParentCommentID: c.ParentCommentID,
		Content:         c.Content,
		IsInternal:      c.IsInternal,
		CreatedAt:       c.CreatedOn,
	}
	if c.CreatedBy != nil {
		m.CreatedBy = *c.CreatedBy
	}
	if c.CreatedByUserType != nil {
		m.CreatedByUserType = *c.CreatedByUserType
	}
	return m
}

func (r *commentRepo) Create(ctx context.Context, auditID, controlID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error) {
	body := map[string]any{
		"content":         content,
		"isInternal":      isInternal,
		"parentCommentId": parentCommentID,
		"createdBy":       createdBy,
	}
	var ec entComment
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/%d/comments", auditID, controlID), body, &ec); err != nil {
		return nil, err
	}
	return ec.toModel(), nil
}

func (r *commentRepo) Delete(ctx context.Context, commentID int) error {
	return r.c.Delete(ctx, fmt.Sprintf("/comments/%d", commentID))
}

func (r *commentRepo) ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditComment, error) {
	var resp struct {
		Comments []entComment `json:"comments"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d/comments", auditID, controlID), &resp); err != nil {
		return nil, err
	}
	out := make([]*model.AuditComment, 0, len(resp.Comments))
	for _, c := range resp.Comments {
		out = append(out, c.toModel())
	}
	return out, nil
}
