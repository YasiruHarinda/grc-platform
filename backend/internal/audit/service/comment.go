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

	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/repository"
)

// CommentService defines business operations for per-evidence comments.
type CommentService interface {
	List(ctx context.Context, controlID int) ([]*model.AuditComment, error)
	Add(ctx context.Context, controlID int, req model.AddCommentRequest, createdBy string) (*model.AuditComment, error)
}

type commentService struct {
	repo repository.CommentRepository
}

func NewCommentService(repo repository.CommentRepository) CommentService {
	return &commentService{repo: repo}
}

func (s *commentService) List(ctx context.Context, controlID int) ([]*model.AuditComment, error) {
	// TODO: delegate to repo; filter is_internal based on caller's privilege
	return nil, nil
}

func (s *commentService) Add(ctx context.Context, controlID int, req model.AddCommentRequest, createdBy string) (*model.AuditComment, error) {
	// TODO: validate content, set is_internal based on caller's privilege, delegate to repo
	return nil, nil
}
