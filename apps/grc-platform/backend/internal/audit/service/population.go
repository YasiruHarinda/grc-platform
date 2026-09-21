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
	// SubmitPopulation records the blobs at folderPath that no round has recorded
	// yet as POPULATION files and advances the round to SUBMITTED. populationID
	// must be the control's latest round. While that round is PENDING or SUBMITTED
	// (first submit, or "Add Files" during internal review) it is reused; once it
	// has been rejected, a new round is started so the rejected one stays on
	// record with its own status, the way evidence resubmissions work. The caller
	// (handler) advances the control to POPULATION_INTERNAL_REVIEW afterwards.
	// attestation is a written note standing in for files — a submission needs
	// new files, a note, or both, same rule as SubmitSample/
	// SampleSubmitRequest.Note, with no privilege gate (unlike evidence's
	// admin-only fileless completion).
	SubmitPopulation(ctx context.Context, auditID, controlID, populationID int, folderPath, attestation, submittedBy string) (*model.PopulationSubmitResult, error)

	// LatestRound returns a control's most recent population round. Returns a
	// 404 apierror if the control has no population round yet.
	LatestRound(ctx context.Context, auditID, controlID int) (*model.AuditPopulation, error)

	// ListRounds returns every population round for a control, oldest first.
	ListRounds(ctx context.Context, auditID, controlID int) ([]*model.AuditPopulation, error)

	// ListFiles returns every file on a population round, newest first.
	ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error)

	// GetFileByID returns file metadata (kind, population id, and the owning
	// control's team_id / auditor_id) so the caller can apply the correct
	// authorization gate before acting on it.
	GetFileByID(ctx context.Context, fileID int) (*model.PopulationFile, error)

	// DownloadFile returns one population/sample file's bytes (proxied via the
	// Compliance Entity) plus its name and content type, by file ID.
	DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error)

	// DeleteFile removes a single population/sample file's DB record and
	// best-effort deletes its underlying blob too — see the implementation
	// comment for why that differs from evidence's DeleteFile.
	DeleteFile(ctx context.Context, fileID int) error

	// ClearAttestation blanks a population round's note (a fileless submit, or
	// a note alongside files) without touching its files or status — the
	// counterpart to evidence's DeleteRound for a round that still has files
	// left after the note goes, where deleting the whole round isn't right.
	ClearAttestation(ctx context.Context, populationID int, updatedBy string) error

	// UpdateRoundStatus advances the population round's own status (distinct from
	// the control's status) — e.g. SUBMITTED → COMPLIANCE_APPROVED.
	UpdateRoundStatus(ctx context.Context, populationID int, status, updatedBy string) error

	// SubmitSample records the blobs at folderPath that no round has recorded yet
	// as SAMPLE files on the round. The caller (handler) sets the control's
	// sample note and advances its status afterwards — the round's own status is
	// not affected by a sample submission.
	SubmitSample(ctx context.Context, auditID, controlID, populationID int, folderPath, submittedBy string) (fileCount int, err error)
}

type populationService struct {
	repo    repository.PopulationRepository
	storage *file.Service
}

// NewPopulationService constructs a PopulationService.
func NewPopulationService(repo repository.PopulationRepository, storage *file.Service) PopulationService {
	return &populationService{repo: repo, storage: storage}
}

// recordedPaths returns every blob path already recorded as a file of the given
// kind on any of the control's rounds, plus how many such files are on round
// currentID. The population/sample upload folders are stable (reused across
// rounds and "add more files" calls, not a fresh per-submission folder like
// evidence's), so re-listing one must not re-insert a row for a blob an earlier
// call or round already recorded — that would duplicate the file in every
// list/count, or pull a rejected round's files into the new round.
func (s *populationService) recordedPaths(ctx context.Context, rounds []*model.AuditPopulation, kind string, currentID int) (recorded map[string]bool, onCurrent int, err error) {
	recorded = make(map[string]bool)
	for _, round := range rounds {
		files, err := s.repo.ListFiles(ctx, round.ID)
		if err != nil {
			return nil, 0, err
		}
		for _, f := range files {
			if !strings.EqualFold(f.FileKind, kind) {
				continue
			}
			recorded[f.FilePath] = true
			if round.ID == currentID {
				onCurrent++
			}
		}
	}
	return recorded, onCurrent, nil
}

// unrecordedBlobs drops the blobs already in recorded.
func unrecordedBlobs(blobs []file.BlobItem, recorded map[string]bool) []file.BlobItem {
	out := make([]file.BlobItem, 0, len(blobs))
	for _, blob := range blobs {
		if !recorded[blob.Name] {
			out = append(out, blob)
		}
	}
	return out
}

func (s *populationService) addBlobsAsFiles(ctx context.Context, populationID int, kind string, blobs []file.BlobItem, submittedBy string) error {
	for _, blob := range blobs {
		ct := blob.ContentType
		sz := blob.Size
		if err := s.repo.AddFile(ctx, populationID, kind, blob.FileName(), blob.Name, &ct, &sz, submittedBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *populationService) SubmitPopulation(ctx context.Context, auditID, controlID, populationID int, folderPath, attestation, submittedBy string) (*model.PopulationSubmitResult, error) {
	blobs, err := s.storage.ListBlobs(ctx, folderPath)
	if err != nil {
		return nil, err
	}
	rounds, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	// Only the latest round is open to the team; an earlier one is history.
	if len(rounds) == 0 || rounds[len(rounds)-1].ID != populationID {
		return nil, &apierror.Error{
			StatusCode: http.StatusConflict,
			Body:       "this control has no active population phase; use the evidence endpoints",
		}
	}
	current := rounds[len(rounds)-1]

	recorded, onCurrent, err := s.recordedPaths(ctx, rounds, "POPULATION", current.ID)
	if err != nil {
		return nil, err
	}
	newBlobs := unrecordedBlobs(blobs, recorded)

	rejected := model.IsRejectedPopulationStatus(current.Status)
	if rejected {
		// A rejected round is history: its files are not carried over, so the
		// resubmission has to bring its own files or a note.
		onCurrent = 0
	}
	// Files, a note, or both — at least one required (same rule as
	// SubmitSample/SampleSubmitRequest.Note; no privilege gate, unlike
	// evidence's ManageControls-only fileless completion).
	if len(newBlobs) == 0 && onCurrent == 0 && strings.TrimSpace(attestation) == "" {
		return nil, &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "provide population files, a note, or both",
		}
	}

	if rejected {
		// If a later step fails the new PENDING round is left behind as the
		// latest round; the retry reuses it (the dedupe above skips whatever
		// was already recorded), so there is nothing to compensate.
		created, err := s.repo.CreateRound(ctx, auditID, controlID, current, submittedBy)
		if err != nil {
			return nil, err
		}
		populationID = created.ID
	}

	if err := s.addBlobsAsFiles(ctx, populationID, "POPULATION", newBlobs, submittedBy); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateStatusWithAttestation(ctx, populationID, "SUBMITTED", attestation, submittedBy); err != nil {
		return nil, err
	}

	return &model.PopulationSubmitResult{
		PopulationID: populationID,
		ControlID:    controlID,
		Status:       "SUBMITTED",
		FolderPath:   folderPath,
		FileCount:    onCurrent + len(newBlobs),
	}, nil
}

func (s *populationService) ListRounds(ctx context.Context, auditID, controlID int) ([]*model.AuditPopulation, error) {
	return s.repo.ListByControl(ctx, auditID, controlID)
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
// control, so a blob left behind after a DB-only delete would resurface — get
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

func (s *populationService) ClearAttestation(ctx context.Context, populationID int, updatedBy string) error {
	return s.repo.ClearAttestation(ctx, populationID, updatedBy)
}

func (s *populationService) UpdateRoundStatus(ctx context.Context, populationID int, status, updatedBy string) error {
	return s.repo.UpdateStatus(ctx, populationID, status, updatedBy)
}

// SubmitSample records the blobs at folderPath that no round has recorded yet as
// SAMPLE files. A sample may be a note alone — files and the note are each
// optional, but at least one is required; the handler enforces that combined
// check once it has both this file count and the note.
func (s *populationService) SubmitSample(ctx context.Context, auditID, controlID, populationID int, folderPath, submittedBy string) (int, error) {
	blobs, err := s.storage.ListBlobs(ctx, folderPath)
	if err != nil {
		return 0, err
	}
	if len(blobs) == 0 {
		return 0, nil
	}
	rounds, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return 0, err
	}
	recorded, _, err := s.recordedPaths(ctx, rounds, "SAMPLE", populationID)
	if err != nil {
		return 0, err
	}
	if err := s.addBlobsAsFiles(ctx, populationID, "SAMPLE", unrecordedBlobs(blobs, recorded), submittedBy); err != nil {
		return 0, err
	}
	// Report the folder's total sample file count (existing + newly added), not
	// just what this call inserted — the caller uses it to validate "at least
	// one file or a note", which should hold for an edit that only added a note.
	return len(blobs), nil
}
