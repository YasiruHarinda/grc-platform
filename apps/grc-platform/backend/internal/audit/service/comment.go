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

package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// CommentService defines business operations for per-control comments (one
// thread per control, spanning population and evidence phases).
type CommentService interface {
	// List returns comments for a control. When includeInternal is false
	// (external auditor), is_internal comments are excluded.
	List(ctx context.Context, auditID, controlID int, includeInternal bool) ([]*model.AuditComment, error)
	Add(ctx context.Context, auditID, controlID int, req model.AddCommentRequest, createdBy string) (*model.AuditComment, error)
}

type commentService struct {
	repo repository.CommentRepository
}

func NewCommentService(repo repository.CommentRepository) CommentService {
	return &commentService{repo: repo}
}

func (s *commentService) List(ctx context.Context, auditID, controlID int, includeInternal bool) ([]*model.AuditComment, error) {
	all, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if includeInternal {
		return all, nil
	}
	// Strip internal comments for external auditors.
	visible := make([]*model.AuditComment, 0, len(all))
	for _, c := range all {
		if !c.IsInternal {
			visible = append(visible, c)
		}
	}
	return visible, nil
}

func (s *commentService) Add(ctx context.Context, auditID, controlID int, req model.AddCommentRequest, createdBy string) (*model.AuditComment, error) {
	if strings.TrimSpace(req.Content) == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "content is required"}
	}
	return s.repo.Create(ctx, auditID, controlID, req.Content, req.IsInternal, req.ParentCommentID, createdBy)
}
