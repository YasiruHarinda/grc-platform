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
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// stubEvidenceRepo answers ListByControl from a fixed set of rounds; every
// other method is unused by the List path under test.
type stubEvidenceRepo struct{ rounds []*model.AuditEvidence }

func (s *stubEvidenceRepo) ListByControl(context.Context, int, int) ([]*model.AuditEvidence, error) {
	return s.rounds, nil
}
func (s *stubEvidenceRepo) Create(context.Context, int, int, string, string, string) (int, error) {
	panic("unused")
}
func (s *stubEvidenceRepo) AddFile(context.Context, int, string, string, *string, *int64, string) error {
	panic("unused")
}
func (s *stubEvidenceRepo) DeleteEvidence(context.Context, int) error { panic("unused") }
func (s *stubEvidenceRepo) GetFileByID(context.Context, int) (*model.AuditEvidenceFile, error) {
	panic("unused")
}
func (s *stubEvidenceRepo) DeleteFile(context.Context, int) error                   { panic("unused") }
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
