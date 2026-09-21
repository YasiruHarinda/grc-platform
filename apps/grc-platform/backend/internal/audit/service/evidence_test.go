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
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// stubEvidenceRepo answers ListByControl from a fixed set of rounds. The delete
// tests also seed files (GetFileByID) and read back what was deleted; every
// other method is unused by the paths under test.
type stubEvidenceRepo struct {
	rounds        []*model.AuditEvidence
	files         map[int]*model.AuditEvidenceFile
	deletedFiles  []int
	deletedRounds []int
}

func (s *stubEvidenceRepo) ListByControl(context.Context, int, int) ([]*model.AuditEvidence, error) {
	return s.rounds, nil
}
func (s *stubEvidenceRepo) Create(context.Context, int, int, string, string, string) (int, error) {
	panic("unused")
}
func (s *stubEvidenceRepo) AddFile(context.Context, int, string, string, *string, *int64, string) error {
	panic("unused")
}
func (s *stubEvidenceRepo) DeleteEvidence(_ context.Context, id int) error {
	s.deletedRounds = append(s.deletedRounds, id)
	return nil
}
func (s *stubEvidenceRepo) GetFileByID(_ context.Context, id int) (*model.AuditEvidenceFile, error) {
	return s.files[id], nil
}
func (s *stubEvidenceRepo) DeleteFile(_ context.Context, id int) error {
	s.deletedFiles = append(s.deletedFiles, id)
	return nil
}
func (s *stubEvidenceRepo) UpdateStatus(context.Context, int, string, string) error { panic("unused") }
func (s *stubEvidenceRepo) EvidenceAuditorID(context.Context, int) (*int, *int, string, error) {
	panic("unused")
}

func round(id int, status string, fileIDs ...int) *model.AuditEvidence {
	files := make([]*model.AuditEvidenceFile, 0, len(fileIDs))
	for _, fid := range fileIDs {
		files = append(files, &model.AuditEvidenceFile{ID: fid, FileName: "evidence.pdf"})
	}
	return &model.AuditEvidence{ID: id, Status: status, Files: files}
}

// Rejected rounds stay on record but are internal-audience only: the handler
// passes includeRejected=false for an external auditor (see
// internalEvidenceViewer), and that must drop the round *and* its files —
// filenames and download URLs included.
func TestListDropsRejectedRoundsForExternalViewers(t *testing.T) {
	repo := &stubEvidenceRepo{rounds: []*model.AuditEvidence{
		round(3, "SUBMITTED", 30),
		round(2, "AUDITOR_REJECTED", 20),
		round(1, "COMPLIANCE_REJECTED", 10),
	}}
	svc := NewEvidenceService(repo, nil, nil, nil)

	got, err := svc.List(context.Background(), 1, 2, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rounds, want only the non-rejected one", len(got))
	}
	if got[0].ID != 3 {
		t.Errorf("kept round %d, want 3", got[0].ID)
	}
	for _, e := range got {
		if model.IsRejectedEvidenceStatus(e.Status) {
			t.Errorf("round %d (%s) reached an external viewer", e.ID, e.Status)
		}
	}
}

// The same call for an internal viewer keeps every round, so a rejected round
// stays visible alongside the resubmission that superseded it.
func TestListKeepsRejectedRoundsForInternalViewers(t *testing.T) {
	repo := &stubEvidenceRepo{rounds: []*model.AuditEvidence{
		round(3, "SUBMITTED", 30),
		round(2, "AUDITOR_REJECTED", 20),
		round(1, "COMPLIANCE_REJECTED", 10),
	}}
	svc := NewEvidenceService(repo, nil, nil, nil)

	got, err := svc.List(context.Background(), 1, 2, true)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rounds, want all 3", len(got))
	}
}

// A download URL is what actually reaches the bytes, so it must not be minted
// onto a round an external viewer never sees.
func TestListWithholdsDownloadURLsOfRejectedRounds(t *testing.T) {
	repo := &stubEvidenceRepo{rounds: []*model.AuditEvidence{
		round(2, "SUBMITTED", 20),
		round(1, "COMPLIANCE_REJECTED", 10),
	}}
	svc := NewEvidenceService(repo, nil, nil, nil)

	got, err := svc.List(context.Background(), 1, 2, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range got {
		for _, f := range e.Files {
			if f.ID == 10 {
				t.Fatal("a rejected round's file was returned to an external viewer")
			}
			if f.ReadURL == nil || !strings.Contains(*f.ReadURL, "/evidence/files/20/download") {
				t.Errorf("visible file %d has readUrl %v, want its download URL", f.ID, f.ReadURL)
			}
		}
	}
}

// deleteStub is a control with three rounds, newest first: round 3 (SUBMITTED,
// file 30), round 2 (AUDITOR_REJECTED, file 20) and round 1 (COMPLIANCE_REJECTED,
// file 10). File 99 belongs to a round of some other control.
func deleteStub() *stubEvidenceRepo {
	return &stubEvidenceRepo{
		rounds: []*model.AuditEvidence{
			round(3, "SUBMITTED", 30),
			round(2, "AUDITOR_REJECTED", 20),
			round(1, "COMPLIANCE_REJECTED", 10),
		},
		files: map[int]*model.AuditEvidenceFile{
			30: {ID: 30, EvidenceID: 3, CreatedBy: "alice"},
			20: {ID: 20, EvidenceID: 2, CreatedBy: "alice"},
			10: {ID: 10, EvidenceID: 1, CreatedBy: "alice"},
			99: {ID: 99, EvidenceID: 77, CreatedBy: "alice"},
		},
	}
}

func wantAPIStatus(t *testing.T, err error, want int) {
	t.Helper()
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != want {
		t.Fatalf("got error %v, want an apierror with status %d", err, want)
	}
}

// A superseded round was already reviewed, so its files stay on record — for
// the submitter and for an admin alike.
func TestDeleteFileRejectsEarlierRounds(t *testing.T) {
	for _, isAdmin := range []bool{false, true} {
		repo := deleteStub()
		svc := NewEvidenceService(repo, nil, nil, nil)

		err := svc.DeleteFile(context.Background(), 1, 2, 20, "alice", isAdmin)
		wantAPIStatus(t, err, http.StatusConflict)
		if len(repo.deletedFiles) != 0 {
			t.Errorf("admin=%v: file was deleted from an earlier round: %v", isAdmin, repo.deletedFiles)
		}
	}
}

func TestDeleteFileAllowsLatestRound(t *testing.T) {
	repo := deleteStub()
	svc := NewEvidenceService(repo, nil, nil, nil)

	if err := svc.DeleteFile(context.Background(), 1, 2, 30, "alice", false); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if len(repo.deletedFiles) != 1 || repo.deletedFiles[0] != 30 {
		t.Errorf("deleted %v, want [30]", repo.deletedFiles)
	}
}

// Emptying the newest round must not promote the rejected round before it to
// "latest" — the UI counts the empty round too.
func TestDeleteFileEmptyLatestRoundStillSupersedesEarlier(t *testing.T) {
	repo := deleteStub()
	repo.rounds = []*model.AuditEvidence{
		round(3, "SUBMITTED"),
		round(2, "AUDITOR_REJECTED", 20),
	}
	svc := NewEvidenceService(repo, nil, nil, nil)

	wantAPIStatus(t, svc.DeleteFile(context.Background(), 1, 2, 20, "alice", false), http.StatusConflict)
}

// The route names the control, so a file from another control's round is not
// reachable through it.
func TestDeleteFileOutsideControlIsNotFound(t *testing.T) {
	repo := deleteStub()
	svc := NewEvidenceService(repo, nil, nil, nil)

	wantAPIStatus(t, svc.DeleteFile(context.Background(), 1, 2, 99, "alice", true), http.StatusNotFound)
	if len(repo.deletedFiles) != 0 {
		t.Errorf("deleted %v, want nothing", repo.deletedFiles)
	}
}

// The ownership rule is unchanged: only the uploader or an admin, even on the
// latest round.
func TestDeleteFileOwnershipStillApplies(t *testing.T) {
	repo := deleteStub()
	svc := NewEvidenceService(repo, nil, nil, nil)

	wantAPIStatus(t, svc.DeleteFile(context.Background(), 1, 2, 30, "bob", false), http.StatusForbidden)
	if err := svc.DeleteFile(context.Background(), 1, 2, 30, "bob", true); err != nil {
		t.Fatalf("admin DeleteFile: %v", err)
	}
}

func TestDeleteRoundOnlyLatest(t *testing.T) {
	repo := &stubEvidenceRepo{rounds: []*model.AuditEvidence{
		{ID: 3, Status: "SUBMITTED", CreatedBy: "alice", Attestation: "n/a"},
		{ID: 2, Status: "COMPLIANCE_REJECTED", CreatedBy: "alice", Attestation: "n/a"},
	}}
	svc := NewEvidenceService(repo, nil, nil, nil)

	wantAPIStatus(t, svc.DeleteRound(context.Background(), 1, 2, 2, "alice", true), http.StatusConflict)
	if len(repo.deletedRounds) != 0 {
		t.Fatalf("deleted %v from an earlier round", repo.deletedRounds)
	}
	if err := svc.DeleteRound(context.Background(), 1, 2, 3, "alice", false); err != nil {
		t.Fatalf("DeleteRound latest: %v", err)
	}
	if len(repo.deletedRounds) != 1 || repo.deletedRounds[0] != 3 {
		t.Errorf("deleted %v, want [3]", repo.deletedRounds)
	}
}
