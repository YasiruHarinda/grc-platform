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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// validJSONField returns a ValidationError when v is non-nil, non-empty, and not
// valid JSON. The target columns (audit_trail.details, risk_change_log.old_value /
// new_value) are MySQL JSON types, so invalid JSON would otherwise fail at insert
// with a 500 instead of a clean 400.
func validJSONField(name string, v *string) error {
	if v == nil || *v == "" {
		return nil
	}
	if !json.Valid([]byte(*v)) {
		return &apierror.ValidationError{Msg: name + " must be valid JSON"}
	}
	return nil
}

type auditTrailService struct{ repo repository.AuditTrailRepository }

// NewAuditTrailService constructs a AuditTrailService.
func NewAuditTrailService(repo repository.AuditTrailRepository) AuditTrailService {
	return &auditTrailService{repo: repo}
}

var validTrailActions = map[string]bool{
	"CREATED": true, "UPLOADED": true, "RESUBMITTED": true,
	"APPROVED": true, "REJECTED": true, "COMMENTED": true,
	"ESCALATED": true, "AI_VALIDATED": true, "EXPORTED": true,
	"UPDATED": true, "DELETED": true,
}

func (s *auditTrailService) CreateAuditTrail(ctx context.Context, auditID int, req domain.CreateAuditTrailRequest) (domain.AuditTrail, error) {
	if auditID <= 0 {
		return domain.AuditTrail{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	req.Action = strings.ToUpper(req.Action)
	if !validTrailActions[req.Action] {
		return domain.AuditTrail{}, &apierror.ValidationError{Msg: "invalid action: " + req.Action}
	}
	if err := validJSONField("details", req.Details); err != nil {
		return domain.AuditTrail{}, err
	}
	e, err := s.repo.CreateAuditTrail(ctx, auditID, req)
	if err != nil {
		return domain.AuditTrail{}, err
	}
	return *e, nil
}

func (s *auditTrailService) ListAuditTrail(ctx context.Context, auditID int, filter domain.TrailFilter, limit, offset int) (domain.ListAuditTrailResponse, error) {
	if auditID <= 0 {
		return domain.ListAuditTrailResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	for _, cid := range filter.ControlIDs {
		if cid <= 0 {
			return domain.ListAuditTrailResponse{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
		}
	}
	p := domain.Pagination{Limit: limit, Offset: offset}
	normalizePagination(&p)
	entries, total, err := s.repo.ListAuditTrail(ctx, auditID, filter, p.Limit, p.Offset)
	if err != nil {
		return domain.ListAuditTrailResponse{}, err
	}
	if entries == nil {
		entries = []domain.AuditTrail{}
	}
	return domain.ListAuditTrailResponse{Trail: entries, Total: total, Limit: p.Limit, Offset: p.Offset}, nil
}
