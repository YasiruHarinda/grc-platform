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
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/blobpath"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

const maxRiskEvidenceBytes = 25 << 20 // 25 MiB — matches the entity's proxy-upload backstop

// RiskRootFolder is the literal top-level Azure Blob folder for every risk's
// evidence, kept separate from the Audit Hub's own top-level folder
// (service.AuditRootFolder in the audit package).
const RiskRootFolder = "risk"

// The two evidence_type values, spelled out where the folder/validation logic
// needs the literal string (mirrors risk_evidence.evidence_type in risk_schema.sql).
const (
	EvidenceTypeActionPlanAttachment    = "ACTION_PLAN_ATTACHMENT"    // "Risk Evidence Attachment" — risk-level, from the Add Risk form
	EvidenceTypeFinalApprovalAttachment = "FINAL_APPROVAL_ATTACHMENT" // "Risk Action Plan Completion Attachment" — always tied to one plan
)

func riskEvidenceAttachmentFolderPath(riskCode string) string {
	return RiskRootFolder + "/" + riskCode + "/risk-evidence-attachment/"
}

func riskActionPlanCompletionFolderPath(riskCode string) string {
	return RiskRootFolder + "/" + riskCode + "/risk-action-plan-completion-attachment/"
}

// EvidenceService defines business operations for risk evidence files.
type EvidenceService interface {
	List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error)
	// Upload validates evidenceType/actionPlanID, sanitizes fileName, streams
	// the bytes to Azure (proxied through the Compliance Entity) under the
	// risk's evidence-type-specific folder, and records the row. actionPlanID
	// is required for FINAL_APPROVAL_ATTACHMENT and must belong to riskID;
	// it is ignored (forced nil) for ACTION_PLAN_ATTACHMENT.
	Upload(ctx context.Context, riskID int, evidenceType string, actionPlanID *int, fileName, contentType string, content io.Reader, note, createdBy string) (*model.RiskEvidence, error)
	// Delete removes one evidence file. The caller must be the file's creator
	// or hold the admin override. The blob in Azure is not deleted — only the
	// DB record is removed (same rule the Audit Hub's evidence delete uses).
	Delete(ctx context.Context, riskID, evidenceID int, actor string, isAdmin bool) error
	// DownloadFile returns one evidence file's bytes (proxied via the
	// Compliance Entity) plus its name and content type. riskID must match
	// the file's actual risk — the same cross-risk guard Delete uses —
	// otherwise a caller who can view some risk could download another
	// risk's evidence by fileId alone.
	DownloadFile(ctx context.Context, riskID, evidenceID int) (data []byte, fileName, contentType string, err error)
}

type evidenceService struct {
	repo           repository.RiskEvidenceRepository
	riskRepo       repository.RiskRepository
	actionPlanRepo repository.ActionPlanRepository
	storage        *file.Service
}

// NewEvidenceService wires the evidence repository plus the risk and action
// plan repositories — needed to resolve the risk's risk_code (the readable
// blob path segment) and to verify a completion attachment's actionPlanID
// actually belongs to the risk it's being attached to.
func NewEvidenceService(
	repo repository.RiskEvidenceRepository,
	riskRepo repository.RiskRepository,
	actionPlanRepo repository.ActionPlanRepository,
	storage *file.Service,
) EvidenceService {
	return &evidenceService{repo: repo, riskRepo: riskRepo, actionPlanRepo: actionPlanRepo, storage: storage}
}

func (s *evidenceService) List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error) {
	evidence, err := s.repo.List(ctx, riskID)
	if err != nil {
		return nil, err
	}
	// Attach a backend download URL to each file. The browser fetches this
	// authenticated endpoint, which proxies the bytes from the Compliance
	// Entity (the browser never contacts Azure directly).
	for _, e := range evidence {
		if e.ID == 0 {
			continue
		}
		downloadURL := fmt.Sprintf("/api/v1/risks/%d/evidence/%d/download", riskID, e.ID)
		e.DownloadURL = &downloadURL
	}
	return evidence, nil
}

func (s *evidenceService) Upload(ctx context.Context, riskID int, evidenceType string, actionPlanID *int, fileName, contentType string, content io.Reader, note, createdBy string) (*model.RiskEvidence, error) {
	evidenceType = strings.ToUpper(evidenceType)
	if evidenceType != EvidenceTypeActionPlanAttachment && evidenceType != EvidenceTypeFinalApprovalAttachment {
		return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: "evidenceType must be ACTION_PLAN_ATTACHMENT or FINAL_APPROVAL_ATTACHMENT"}
	}

	if evidenceType == EvidenceTypeFinalApprovalAttachment {
		if actionPlanID == nil || *actionPlanID <= 0 {
			return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: "actionPlanId is required for FINAL_APPROVAL_ATTACHMENT"}
		}
		plan, err := s.actionPlanRepo.GetByID(ctx, *actionPlanID)
		if err != nil {
			return nil, err
		}
		if plan.RiskID != riskID {
			return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: "actionPlanId does not belong to this risk"}
		}
	} else {
		// Risk-level evidence is never plan-scoped, regardless of what the
		// caller sent.
		actionPlanID = nil
	}

	risk, err := s.riskRepo.GetByID(ctx, riskID)
	if err != nil {
		return nil, err
	}
	riskCode := blobpath.SanitizeSegment(risk.RiskCode)

	data, err := io.ReadAll(io.LimitReader(content, maxRiskEvidenceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: "file is empty"}
	}
	if int64(len(data)) > maxRiskEvidenceBytes {
		return nil, &apierror.Error{StatusCode: http.StatusRequestEntityTooLarge, Body: "file exceeds 25 MB limit"}
	}

	stem, ext := blobpath.SanitizeFileName(fileName)
	folder := riskEvidenceAttachmentFolderPath(riskCode)
	if evidenceType == EvidenceTypeFinalApprovalAttachment {
		folder = riskActionPlanCompletionFolderPath(riskCode)
	}
	blobName := folder + blobpath.BuildBlobName(stem, ext)

	if err := s.storage.UploadBlob(ctx, blobName, contentType, data); err != nil {
		return nil, err
	}

	ev, err := s.repo.Create(ctx, riskID, actionPlanID, fileName, blobName, note, evidenceType, createdBy)
	if err != nil {
		// Best-effort blob cleanup so a failed DB write doesn't leave an
		// orphaned file in Azure.
		_ = s.storage.Delete(ctx, blobName)
		return nil, err
	}
	return ev, nil
}

func (s *evidenceService) Delete(ctx context.Context, riskID, evidenceID int, actor string, isAdmin bool) error {
	ev, err := s.repo.GetByID(ctx, evidenceID)
	if err != nil {
		return err
	}
	if ev.RiskID != riskID {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "evidence file not found"}
	}
	// A COMPLETED plan's completion evidence is locked, not just from its
	// creator but from the admin override too — this is the guarantee the
	// evidence gate on "Complete Action Plan" exists for in the first place,
	// so it can't be an ownership-overridable rule the way delete otherwise
	// is. The UI already hides delete in this state; this is the server-side
	// enforcement of that same rule.
	if ev.EvidenceType == EvidenceTypeFinalApprovalAttachment && ev.ActionPlanID != nil {
		plan, err := s.actionPlanRepo.GetByID(ctx, *ev.ActionPlanID)
		if err != nil {
			return err
		}
		if plan.Status == "COMPLETED" {
			return &apierror.Error{StatusCode: http.StatusConflict, Body: "cannot delete completion evidence for an already-completed action plan"}
		}
	}
	// Risk-level evidence locks the same way once the risk owner has given
	// their (first) approval — same admin-non-overridable rule as completion
	// evidence above, and the same flag the webapp's RiskEvidenceSection
	// checks client-side (detail.owner_first_approved_at) to hide delete.
	if ev.EvidenceType == EvidenceTypeActionPlanAttachment {
		risk, err := s.riskRepo.GetByID(ctx, riskID)
		if err != nil {
			return err
		}
		if risk.OwnerFirstApprovedAt != nil {
			return &apierror.Error{StatusCode: http.StatusConflict, Body: "cannot delete risk evidence after the risk owner has approved this risk"}
		}
	}
	if !isAdmin && ev.CreatedBy != actor {
		return &apierror.Error{StatusCode: http.StatusForbidden, Body: "forbidden"}
	}
	return s.repo.Delete(ctx, riskID, evidenceID)
}

func (s *evidenceService) DownloadFile(ctx context.Context, riskID, evidenceID int) (data []byte, fileName, contentType string, err error) {
	ev, err := s.repo.GetByID(ctx, evidenceID)
	if err != nil {
		return nil, "", "", err
	}
	if ev.RiskID != riskID {
		return nil, "", "", &apierror.Error{StatusCode: http.StatusNotFound, Body: "evidence file not found"}
	}
	data, contentType, err = s.storage.ReadBlob(ctx, ev.FilePath)
	if err != nil {
		return nil, "", "", err
	}
	return data, ev.FileName, contentType, nil
}
