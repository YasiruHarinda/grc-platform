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
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// PopulationService defines the OE-control population submission flow used by the
// Evidence Portal. File uploads reuse EvidenceService.UploadFile (phase-agnostic
// blob write); this service records the uploaded blobs and advances the round.
type PopulationService interface {
	// SubmitPopulation records every blob at folderPath as a POPULATION file on the
	// population round and advances it to SUBMITTED. The caller (handler) advances
	// the control to POPULATION_INTERNAL_REVIEW afterwards.
	SubmitPopulation(ctx context.Context, controlID, populationID int, folderPath, submittedBy string) (*model.PopulationSubmitResult, error)

	// LatestRound returns a control's most recent population round. A control
	// normally has exactly one round for its whole lifecycle — both internal-review
	// and auditor rejections resubmit the same round rather than starting a new
	// one (see OE-Sample-Evidence-Flow-Design.md §3.2/§8). Returns a 404 apierror
	// if the control has no population round yet.
	LatestRound(ctx context.Context, auditID, controlID int) (*model.AuditPopulation, error)

	// ListFiles returns every file on a population round, newest first.
	ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error)

	// GetFileByID returns file metadata (kind, population id) so the caller can
	// apply the correct authorization gate before acting on it.
	GetFileByID(ctx context.Context, fileID int) (*model.PopulationFile, error)

	// DownloadFile returns one population/sample file's bytes (proxied via the
	// Compliance Entity) plus its name and content type, by file ID.
	DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error)

	// DeleteFile removes a single population/sample file's DB record and
	// best-effort deletes its underlying blob too — see the implementation
	// comment for why that differs from evidence's DeleteFile.
	DeleteFile(ctx context.Context, fileID int) error

	// UpdateRoundStatus advances the population round's own status (distinct from
	// the control's status) — e.g. SUBMITTED → COMPLIANCE_APPROVED.
	UpdateRoundStatus(ctx context.Context, populationID int, status, updatedBy string) error

	// SubmitSample records every blob at folderPath as a SAMPLE file on the round.
	// The caller (handler) sets the control's sample note and advances its status
	// afterwards — the round's own status is not affected by a sample submission.
	SubmitSample(ctx context.Context, populationID int, folderPath, submittedBy string) (fileCount int, err error)
}

type populationService struct {
	repo    repository.PopulationRepository
	storage *file.Service
}

// NewPopulationService constructs a PopulationService.
func NewPopulationService(repo repository.PopulationRepository, storage *file.Service) PopulationService {
	return &populationService{repo: repo, storage: storage}
}

// addNewBlobsAsFiles records blobs as populationID's files, skipping any blob
// whose path is already recorded under the given kind. The population/sample
// upload folders are stable (reused across a resubmission or a later
// "add more files" call, not a fresh per-submission folder like evidence's),
// so re-listing it must not re-insert a row for a blob a previous submit call
// already recorded — that would duplicate the file in every list/count.
func (s *populationService) addNewBlobsAsFiles(ctx context.Context, populationID int, kind string, blobs []file.BlobItem, submittedBy string) (added int, err error) {
	existing, err := s.repo.ListFiles(ctx, populationID)
	if err != nil {
		return 0, err
	}
	recorded := make(map[string]bool, len(existing))
	for _, f := range existing {
		if strings.EqualFold(f.FileKind, kind) {
			recorded[f.FilePath] = true
		}
	}
	for _, blob := range blobs {
		if recorded[blob.Name] {
			continue
		}
		ct := blob.ContentType
		sz := blob.Size
		if err := s.repo.AddFile(ctx, populationID, kind, blob.FileName(), blob.Name, &ct, &sz, submittedBy); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

func (s *populationService) SubmitPopulation(ctx context.Context, controlID, populationID int, folderPath, submittedBy string) (*model.PopulationSubmitResult, error) {
	blobs, err := s.storage.ListBlobs(ctx, folderPath)
	if err != nil {
		return nil, err
	}
	if len(blobs) == 0 {
		return nil, &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "no files found at the specified folderPath — upload files first",
		}
	}

	if _, err := s.addNewBlobsAsFiles(ctx, populationID, "POPULATION", blobs, submittedBy); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, populationID, "SUBMITTED", submittedBy); err != nil {
		return nil, err
	}

	return &model.PopulationSubmitResult{
		PopulationID: populationID,
		ControlID:    controlID,
		Status:       "SUBMITTED",
		FolderPath:   folderPath,
		FileCount:    len(blobs),
	}, nil
}

func (s *populationService) LatestRound(ctx context.Context, auditID, controlID int) (*model.AuditPopulation, error) {
	rounds, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	if len(rounds) == 0 {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "no population round found for this control"}
	}
	return rounds[len(rounds)-1], nil
}

func (s *populationService) ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error) {
	return s.repo.ListFiles(ctx, populationID)
}

func (s *populationService) GetFileByID(ctx context.Context, fileID int) (*model.PopulationFile, error) {
	return s.repo.GetFileByID(ctx, fileID)
}

func (s *populationService) DownloadFile(ctx context.Context, fileID int) ([]byte, string, string, error) {
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

// DeleteFile removes a population/sample file's DB record and best-effort
// deletes its underlying blob. Unlike evidence (a fresh timestamped folder per
// submission), population/sample uploads reuse one stable folder for the whole
// round, so a blob left behind after a DB-only delete would resurface — get
// re-inserted as "new" — the next time that folder is listed on submit.
func (s *populationService) DeleteFile(ctx context.Context, fileID int) error {
	f, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteFile(ctx, fileID); err != nil {
		return err
	}
	if f != nil {
		if err := s.storage.Delete(ctx, f.FilePath); err != nil {
			slog.WarnContext(ctx, "delete population blob failed", "fileId", fileID, "path", f.FilePath, "err", err)
		}
	}
	return nil
}

func (s *populationService) UpdateRoundStatus(ctx context.Context, populationID int, status, updatedBy string) error {
	return s.repo.UpdateStatus(ctx, populationID, status, updatedBy)
}

// SubmitSample records every blob at folderPath as a SAMPLE file. A sample may
// be a note alone (see design doc — files and the note are each optional, but
// at least one is required; the handler enforces that combined check once it
// has both this file count and the note).
func (s *populationService) SubmitSample(ctx context.Context, populationID int, folderPath, submittedBy string) (int, error) {
	blobs, err := s.storage.ListBlobs(ctx, folderPath)
	if err != nil {
		return 0, err
	}
	if len(blobs) == 0 {
		return 0, nil
	}
	if _, err := s.addNewBlobsAsFiles(ctx, populationID, "SAMPLE", blobs, submittedBy); err != nil {
		return 0, err
	}
	// Report the round's total sample file count (existing + newly added), not
	// just what this call inserted — the caller uses it to validate "at least
	// one file or a note", which should hold for an edit that only added a note.
	return len(blobs), nil
}
