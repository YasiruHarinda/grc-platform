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

package service

import (
	"context"
	"encoding/json"
	"log"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type commentService struct {
	repo repository.CommentRepository
	// trail records a COMMENTED entry to the append-only audit trail. Best-effort;
	// may be nil (recording is then skipped) — same contract as the gateway's
	// controlService.recordTrail.
	trail repository.AuditTrailRepository
}

// NewCommentService constructs a CommentService.
func NewCommentService(repo repository.CommentRepository, trail repository.AuditTrailRepository) CommentService {
	return &commentService{repo: repo, trail: trail}
}

func (s *commentService) CreateComment(ctx context.Context, controlID int, req domain.CreateAuditCommentRequest) (domain.AuditComment, error) {
	if controlID <= 0 {
		return domain.AuditComment{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	if req.Content == "" {
		return domain.AuditComment{}, &apierror.ValidationError{Msg: "content is required"}
	}
	if req.CreatedBy == "" {
		return domain.AuditComment{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	c, err := s.repo.CreateComment(ctx, controlID, req)
	if err != nil {
		return domain.AuditComment{}, err
	}
	s.recordCommentTrail(ctx, controlID, req)
	return *c, nil
}

// recordCommentTrail appends a best-effort COMMENTED entry to audit_trail. A
// failure here must never fail the comment write that triggered it — the trail
// is advisory history, not part of the write it describes (mirrors the
// gateway's controlService.recordTrail contract).
func (s *commentService) recordCommentTrail(ctx context.Context, controlID int, req domain.CreateAuditCommentRequest) {
	if s.trail == nil {
		return
	}
	auditID, err := s.repo.ResolveAuditID(ctx, controlID)
	if err != nil {
		log.Printf("comment trail: resolve audit for control %d: %v", controlID, err)
		return
	}
	detailsBytes, err := json.Marshal(map[string]any{
		// key is "comment" (not "content") to match the frontend's existing
		// TrailDetails.comment field, read by ControlHistoryTimeline's readComment.
		"comment":    req.Content,
		"isInternal": req.IsInternal,
	})
	if err != nil {
		log.Printf("comment trail: marshal details for control %d: %v", controlID, err)
		return
	}
	details := string(detailsBytes)
	cID := controlID
	trailReq := domain.CreateAuditTrailRequest{
		ControlID: &cID,
		Action:    "COMMENTED",
		Details:   &details,
		CreatedBy: &req.CreatedBy,
	}
	if _, err := s.trail.CreateAuditTrail(ctx, auditID, trailReq); err != nil {
		log.Printf("comment trail: create for control %d: %v", controlID, err)
	}
}

func (s *commentService) ListCommentsByControl(ctx context.Context, controlID int) (domain.ListAuditCommentsResponse, error) {
	if controlID <= 0 {
		return domain.ListAuditCommentsResponse{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	comments, err := s.repo.ListCommentsByControl(ctx, controlID)
	if err != nil {
		return domain.ListAuditCommentsResponse{}, err
	}
	if comments == nil {
		comments = []domain.AuditComment{}
	}
	return domain.ListAuditCommentsResponse{Comments: comments}, nil
}

func (s *commentService) DeleteComment(ctx context.Context, commentID int) error {
	if commentID <= 0 {
		return &apierror.ValidationError{Msg: "commentId must be a positive integer"}
	}
	return s.repo.DeleteComment(ctx, commentID)
}
