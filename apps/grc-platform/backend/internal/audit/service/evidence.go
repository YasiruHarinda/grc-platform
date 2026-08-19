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
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// EvidenceService defines business operations for audit evidence submissions.
type EvidenceService interface {
	// GetUploadLink returns the control's evidence folder path — a deterministic,
	// human-readable prefix (not a per-session folder).
	GetUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error)

	// PopulationUploadLink returns the control's population folder path — flat,
	// shared by every submission for the control's whole lifecycle (a control
	// normally has exactly one population round; resubmissions reuse it).
	PopulationUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error)

	// SampleUploadLink returns the auditor's sample-upload folder — a "sample/"
	// subfolder of the same control's population folder, kept physically
	// separate so listing one never picks up the other.
	SampleUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error)

	// ValidateEvidenceFolderPath enforces that folderPath is exactly this
	// control's evidence folder, derived server-side from the audit name and
	// control number. Returns a 400 apierror on mismatch.
	ValidateEvidenceFolderPath(ctx context.Context, auditID, controlID int, folderPath string) error

	// ValidatePopulationFolderPath enforces that folderPath is exactly this
	// control's population folder. Returns a 400 apierror on mismatch.
	ValidatePopulationFolderPath(ctx context.Context, auditID, controlID int, folderPath string) error

	// ValidateSampleFolderPath enforces that folderPath is exactly this
	// control's population/sample folder. Returns a 400 apierror on mismatch.
	ValidateSampleFolderPath(ctx context.Context, auditID, controlID int, folderPath string) error

	// UploadFile stores one file into folderPath by proxying the bytes through
	// the backend to the Compliance Entity, which writes it to Azure. The
	// uploaded name is sanitized and given a short UUID suffix so two files
	// sharing a human name never collide; the resulting stored blob name is
	// returned so the caller can accumulate it for Submit.
	UploadFile(ctx context.Context, folderPath, fileName, contentType string, data []byte) (blobName string, err error)

	// Submit records exactly the given files as a new evidence submission —
	// there is no folder re-listing in the flat evidence layout, so every blob
	// name must fall under this control's server-derived evidence folder or the
	// call is rejected. The caller (handler) is responsible for advancing the
	// control status afterwards.
	//
	// A submission with zero files is rejected unless isAdmin is true and
	// attestation is non-empty (fileless completion, gated on ManageControls —
	// see the handler). Everyone else submitting zero files gets the ordinary
	// "no files provided" rejection regardless of what they put in attestation.
	Submit(ctx context.Context, auditID, controlID int, files []model.EvidenceFileRef, attestation string, isAdmin bool, submittedBy string) (*model.AuditEvidence, error)

	// AddFiles appends files to the control's current evidence round instead of
	// starting a new one — used by "Add Files" while a submission is still
	// awaiting internal review (round status SUBMITTED). Without this, every
	// "Add Files" click would create a sibling round via Submit, and a later
	// reviewer decision (which only closes out the single latest round) would
	// leave the earlier one stranded in SUBMITTED forever — silently resurfacing
	// its files alongside every future resubmission. Returns a 404 if there is no
	// round yet, or a 409 if the current round has already been decided (the
	// caller should fall back to Submit for a fresh round in that case).
	AddFiles(ctx context.Context, auditID, controlID int, files []model.EvidenceFileRef, actor string) (*model.AuditEvidence, error)

	// List returns all evidence submissions for a control, newest first.
	List(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error)

	// LatestRound returns a control's most recently submitted evidence round —
	// used to record a reviewer's decision against the round they actually looked
	// at. Returns a 404 apierror if the control has no evidence round yet.
	LatestRound(ctx context.Context, auditID, controlID int) (*model.AuditEvidence, error)

	// UpdateRoundStatus advances one evidence round's own status (distinct from
	// the control's status) — e.g. SUBMITTED → COMPLIANCE_REJECTED. This lets the
	// live evidence view (SubmittedEvidenceList) tell a rejected round apart from
	// the fresh resubmission that follows it, instead of showing both together.
	UpdateRoundStatus(ctx context.Context, evidenceID int, status, updatedBy string) error

	// DownloadFile returns one evidence file's bytes (proxied via the Compliance
	// Entity) plus its name and content type, by file ID.
	DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error)

	// FileAuditorEmail returns the email of the auditor assigned to fileID's
	// owning control (nil if none) and that control's team id (nil if none),
	// for the assigned-auditor and team-scoped download gates — the download
	// route only carries a file id, not a control id, so this is how
	// downloadEvidenceFile resolves assignment/team without one.
	FileAuditorEmail(ctx context.Context, fileID int) (auditorEmail *string, teamID *int, err error)

	// DeleteFile removes a single evidence file from the submission. The caller
	// must be the file's creator or hold ManageControls (isAdmin=true). The blob
	// in Azure is not deleted — only the DB record is removed.
	DeleteFile(ctx context.Context, fileID int, actor string, isAdmin bool) error

	// DeleteRound removes a whole fileless (attestation-only) evidence round —
	// the "Completed without files" case, which has no per-file delete to fall
	// back on (rounds holding files go through DeleteFile instead, one file at
	// a time). The caller must be the round's creator or hold ManageControls
	// (isAdmin=true).
	DeleteRound(ctx context.Context, auditID, controlID, evidenceID int, actor string, isAdmin bool) error
}

type evidenceService struct {
	repo        repository.EvidenceRepository
	auditRepo   repository.AuditRepository
	controlRepo repository.ControlRepository
	storage     *file.Service
}

// NewEvidenceService wires the evidence repository plus the audit and control
// repositories — the latter two are needed to resolve the audit name and
// control number that the readable blob layout is built from (see resolveNames).
func NewEvidenceService(
	repo repository.EvidenceRepository,
	auditRepo repository.AuditRepository,
	controlRepo repository.ControlRepository,
	storage *file.Service,
) EvidenceService {
	return &evidenceService{repo: repo, auditRepo: auditRepo, controlRepo: controlRepo, storage: storage}
}

var errFolderPathMismatch = &apierror.Error{
	StatusCode: http.StatusBadRequest,
	Body:       "folderPath does not match this control",
}

// resolveNames loads and path-sanitizes the audit name and control number for
// auditID/controlID. This is the single server-side place evidence/population/
// sample paths are derived from — auditID and controlID always come from the
// route or a server-side lookup, never trusted verbatim from the client, so a
// caller cannot aim bytes at another audit/control's folder by forging a name.
func (s *evidenceService) resolveNames(ctx context.Context, auditID, controlID int) (auditName, controlNumber string, err error) {
	a, err := s.auditRepo.GetByID(ctx, auditID)
	if err != nil {
		return "", "", err
	}
	if a == nil {
		return "", "", &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	c, err := s.controlRepo.GetByID(ctx, auditID, controlID)
	if err != nil {
		return "", "", err
	}
	if c == nil {
		return "", "", &apierror.Error{StatusCode: http.StatusNotFound, Body: "control not found"}
	}
	return sanitizeSegment(a.Name), sanitizeSegment(c.ControlNumber), nil
}

// AuditRootFolder is the literal top-level Azure Blob folder for every audit's
// evidence, keeping the Audit Hub's storage tree separate from the Risk
// module's (which owns its own top-level folder). Exported so callers deriving
// a folder path outside this package (e.g. the handler's display-only
// baseFolderPathFor) stay in sync with evidenceFolderPath/populationFolderPath.
const AuditRootFolder = "audit"

func evidenceFolderPath(auditName, controlNumber string) string {
	return AuditRootFolder + "/" + auditName + "/" + controlNumber + "/evidence/"
}

func populationFolderPath(auditName, controlNumber string) string {
	return AuditRootFolder + "/" + auditName + "/" + controlNumber + "/population/"
}

func sampleFolderPath(auditName, controlNumber string) string {
	return AuditRootFolder + "/" + auditName + "/" + controlNumber + "/population/sample/"
}

func (s *evidenceService) GetUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error) {
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	return &model.UploadLinkResponse{
		FolderPath: evidenceFolderPath(auditName, controlNumber),
		ExpiresAt:  time.Now().UTC().Add(4 * time.Hour),
	}, nil
}

func (s *evidenceService) PopulationUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error) {
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	return &model.UploadLinkResponse{
		FolderPath: populationFolderPath(auditName, controlNumber),
		ExpiresAt:  time.Now().UTC().Add(4 * time.Hour),
	}, nil
}

func (s *evidenceService) SampleUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error) {
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	return &model.UploadLinkResponse{
		FolderPath: sampleFolderPath(auditName, controlNumber),
		ExpiresAt:  time.Now().UTC().Add(4 * time.Hour),
	}, nil
}

func (s *evidenceService) ValidateEvidenceFolderPath(ctx context.Context, auditID, controlID int, folderPath string) error {
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if folderPath != evidenceFolderPath(auditName, controlNumber) {
		return errFolderPathMismatch
	}
	return nil
}

func (s *evidenceService) ValidatePopulationFolderPath(ctx context.Context, auditID, controlID int, folderPath string) error {
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if folderPath != populationFolderPath(auditName, controlNumber) {
		return errFolderPathMismatch
	}
	return nil
}

func (s *evidenceService) ValidateSampleFolderPath(ctx context.Context, auditID, controlID int, folderPath string) error {
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	if folderPath != sampleFolderPath(auditName, controlNumber) {
		return errFolderPathMismatch
	}
	return nil
}

func (s *evidenceService) UploadFile(ctx context.Context, folderPath, fileName, contentType string, data []byte) (string, error) {
	if folderPath == "" {
		return "", &apierror.Error{StatusCode: http.StatusBadRequest, Body: "folderPath is required"}
	}
	if len(data) == 0 {
		return "", &apierror.Error{StatusCode: http.StatusBadRequest, Body: "file is empty"}
	}
	stem, ext := sanitizeFileName(fileName)
	blobName := folderPath + buildBlobName(stem, ext)
	if err := s.storage.UploadBlob(ctx, blobName, contentType, data); err != nil {
		return "", err
	}
	return blobName, nil
}

// displayFileName reconstructs a human-readable name from a stored blob name by
// stripping the trailing "-<8-hex-uuid>" suffix buildBlobName appended on
// upload. Used at Submit time, where only the blob name (not the original
// upload-time file name) is available for files the client references.
func displayFileName(blobName string) string {
	base := filepath.Base(blobName)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if idx := strings.LastIndex(stem, "-"); idx > 0 {
		if suffix := stem[idx+1:]; len(suffix) == 8 && isHex(suffix) {
			stem = stem[:idx]
		}
	}
	return stem + ext
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func (s *evidenceService) Submit(ctx context.Context, auditID, controlID int, files []model.EvidenceFileRef, attestation string, isAdmin bool, submittedBy string) (*model.AuditEvidence, error) {
	fileless := len(files) == 0
	attestation = strings.TrimSpace(attestation)
	if fileless && !(isAdmin && attestation != "") {
		return nil, &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "no files provided — upload files first",
		}
	}
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	prefix := evidenceFolderPath(auditName, controlNumber)
	for _, f := range files {
		if !strings.HasPrefix(f.BlobName, prefix) {
			return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: "blob name does not match this audit/control"}
		}
	}

	roundAttestation := ""
	if fileless {
		roundAttestation = attestation
	}
	evidenceID, err := s.repo.Create(ctx, auditID, controlID, prefix, roundAttestation, submittedBy)
	if err != nil {
		return nil, err
	}

	evidenceFiles := make([]*model.AuditEvidenceFile, 0, len(files))
	for _, f := range files {
		fileName := f.FileName
		if fileName == "" {
			fileName = displayFileName(f.BlobName)
		}
		if err := s.repo.AddFile(ctx, evidenceID, fileName, f.BlobName, nil, nil, submittedBy); err != nil {
			// Best-effort rollback: remove the evidence record so no empty submission is persisted.
			_ = s.repo.DeleteEvidence(ctx, evidenceID)
			return nil, err
		}
		evidenceFiles = append(evidenceFiles, &model.AuditEvidenceFile{
			EvidenceID: evidenceID,
			FileName:   fileName,
			FilePath:   f.BlobName,
			CreatedBy:  submittedBy,
		})
	}

	return &model.AuditEvidence{
		ID:          evidenceID,
		ControlID:   controlID,
		Status:      "SUBMITTED",
		FolderPath:  prefix,
		Files:       evidenceFiles,
		Attestation: roundAttestation,
		CreatedBy:   submittedBy,
		CreatedAt:   time.Now(),
	}, nil
}

func (s *evidenceService) AddFiles(ctx context.Context, auditID, controlID int, files []model.EvidenceFileRef, actor string) (*model.AuditEvidence, error) {
	if len(files) == 0 {
		return nil, &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "no files provided — upload files first",
		}
	}
	auditName, controlNumber, err := s.resolveNames(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	prefix := evidenceFolderPath(auditName, controlNumber)
	for _, f := range files {
		if !strings.HasPrefix(f.BlobName, prefix) {
			return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: "blob name does not match this audit/control"}
		}
	}

	round, err := s.LatestRound(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if round.Status != "SUBMITTED" {
		return nil, &apierror.Error{
			StatusCode: http.StatusConflict,
			Body:       "the current evidence round has already been decided — resubmit to start a new one",
		}
	}

	evidenceFiles := make([]*model.AuditEvidenceFile, 0, len(files))
	for _, f := range files {
		fileName := f.FileName
		if fileName == "" {
			fileName = displayFileName(f.BlobName)
		}
		if err := s.repo.AddFile(ctx, round.ID, fileName, f.BlobName, nil, nil, actor); err != nil {
			return nil, err
		}
		evidenceFiles = append(evidenceFiles, &model.AuditEvidenceFile{
			EvidenceID: round.ID,
			FileName:   fileName,
			FilePath:   f.BlobName,
			CreatedBy:  actor,
		})
	}
	round.Files = append(round.Files, evidenceFiles...)
	return round, nil
}

func (s *evidenceService) List(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error) {
	evidence, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	// Attach a backend download URL to each file. The reviewer's browser fetches
	// this authenticated endpoint, which proxies the bytes from the Compliance
	// Entity (the browser never contacts Azure directly).
	for _, e := range evidence {
		for _, f := range e.Files {
			if f.ID == 0 {
				continue
			}
			downloadURL := fmt.Sprintf("/api/v1/evidence/files/%d/download", f.ID)
			f.ReadURL = &downloadURL
		}
	}
	return evidence, nil
}

// DownloadFile fetches one evidence file's bytes (proxied via the Compliance
// Entity) by file ID, for the authenticated download endpoint.
func (s *evidenceService) LatestRound(ctx context.Context, auditID, controlID int) (*model.AuditEvidence, error) {
	rounds, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if len(rounds) == 0 {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "no evidence round found for this control"}
	}
	return rounds[0], nil // ListByControl returns newest first.
}

func (s *evidenceService) UpdateRoundStatus(ctx context.Context, evidenceID int, status, updatedBy string) error {
	return s.repo.UpdateStatus(ctx, evidenceID, status, updatedBy)
}

func (s *evidenceService) DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error) {
	f, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, "", "", err
	}
	data, ct, err := s.storage.ReadBlob(ctx, f.FilePath)
	if err != nil {
		return nil, "", "", err
	}
	if ct == "" && f.FileType != nil {
		ct = *f.FileType
	}
	return data, f.FileName, ct, nil
}

func (s *evidenceService) FileAuditorEmail(ctx context.Context, fileID int) (auditorEmail *string, teamID *int, err error) {
	f, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}
	return f.AuditorEmail, f.TeamID, nil
}

func (s *evidenceService) DeleteFile(ctx context.Context, fileID int, actor string, isAdmin bool) error {
	f, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "file not found"}
	}
	if !isAdmin && f.CreatedBy != actor {
		return &apierror.Error{StatusCode: http.StatusForbidden, Body: "forbidden"}
	}
	return s.repo.DeleteFile(ctx, fileID)
}

func (s *evidenceService) DeleteRound(ctx context.Context, auditID, controlID, evidenceID int, actor string, isAdmin bool) error {
	rounds, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return err
	}
	var round *model.AuditEvidence
	for _, r := range rounds {
		if r.ID == evidenceID {
			round = r
			break
		}
	}
	if round == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "evidence round not found"}
	}
	if len(round.Files) > 0 {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: "round has files — delete them individually"}
	}
	if !isAdmin && round.CreatedBy != actor {
		return &apierror.Error{StatusCode: http.StatusForbidden, Body: "forbidden"}
	}
	return s.repo.DeleteEvidence(ctx, evidenceID)
}
