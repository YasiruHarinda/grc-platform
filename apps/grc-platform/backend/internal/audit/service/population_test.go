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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// fakePopulationRepo is an in-memory PopulationRepository for the submit paths.
type fakePopulationRepo struct {
	rounds      []*model.AuditPopulation
	files       map[int][]*model.PopulationFile // by round id
	nextID      int
	nextFileID  int
	createdFrom []*model.AuditPopulation
}

func newFakePopulationRepo(rounds ...*model.AuditPopulation) *fakePopulationRepo {
	r := &fakePopulationRepo{files: map[int][]*model.PopulationFile{}, nextID: 100, nextFileID: 1000}
	r.rounds = rounds
	return r
}

func (r *fakePopulationRepo) round(id int) *model.AuditPopulation {
	for _, p := range r.rounds {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (r *fakePopulationRepo) addFile(roundID int, kind, path string) {
	r.nextFileID++
	r.files[roundID] = append(r.files[roundID], &model.PopulationFile{
		ID: r.nextFileID, PopulationID: roundID, FileKind: kind, FileName: path, FilePath: path,
	})
}

func (r *fakePopulationRepo) AddFile(_ context.Context, populationID int, kind, name, path string, _ *string, _ *int64, _ string) error {
	r.addFile(populationID, kind, path)
	return nil
}
func (r *fakePopulationRepo) CreateRound(_ context.Context, _, controlID int, from *model.AuditPopulation, _ string) (*model.AuditPopulation, error) {
	r.nextID++
	created := &model.AuditPopulation{
		ID: r.nextID, ControlID: controlID, Status: "PENDING",
		OwnerID: from.OwnerID, TeamID: from.TeamID, ReferenceNumber: from.ReferenceNumber,
		Description: from.Description, DueDate: from.DueDate, Comments: from.Comments,
	}
	r.rounds = append(r.rounds, created)
	r.createdFrom = append(r.createdFrom, from)
	return created, nil
}
func (r *fakePopulationRepo) UpdateStatus(_ context.Context, populationID int, status, _ string) error {
	r.round(populationID).Status = status
	return nil
}
func (r *fakePopulationRepo) UpdateStatusWithAttestation(_ context.Context, populationID int, status, attestation, _ string) error {
	p := r.round(populationID)
	p.Status = status
	if attestation != "" {
		p.Attestation = &attestation
	}
	return nil
}
func (r *fakePopulationRepo) ClearAttestation(context.Context, int, string) error { panic("unused") }
func (r *fakePopulationRepo) UpdateDetails(context.Context, int, model.PopulationDetails, string) error {
	panic("unused")
}
func (r *fakePopulationRepo) ListByControl(context.Context, int, int) ([]*model.AuditPopulation, error) {
	return r.rounds, nil
}
func (r *fakePopulationRepo) ListFiles(_ context.Context, populationID int) ([]*model.PopulationFile, error) {
	return r.files[populationID], nil
}
func (r *fakePopulationRepo) GetFileByID(context.Context, int) (*model.PopulationFile, error) {
	panic("unused")
}
func (r *fakePopulationRepo) DeleteFile(context.Context, int) error { panic("unused") }

// blobStorage serves /files/list with the given blob names, standing in for the
// Compliance Entity that file.Service talks to.
func blobStorage(t *testing.T, names ...string) *file.Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		type blob struct {
			Name string `json:"name"`
		}
		files := make([]blob, 0, len(names))
		for _, n := range names {
			files = append(files, blob{Name: n})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": files})
	}))
	t.Cleanup(srv.Close)
	return file.NewService(srv.URL)
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

const popFolder = "audit/a/CA-01/population/"

func TestSubmitPopulationRejectedRoundStartsNewRound(t *testing.T) {
	rejected := &model.AuditPopulation{
		ID: 1, ControlID: 7, Status: "COMPLIANCE_REJECTED",
		OwnerID: intPtr(4), TeamID: intPtr(5), Description: strPtr("all accounts"),
		DueDate: strPtr("2026-10-01"), Comments: strPtr("include contractors"),
	}
	repo := newFakePopulationRepo(rejected)
	repo.addFile(1, "POPULATION", popFolder+"old-abc.csv")
	svc := NewPopulationService(repo, blobStorage(t, popFolder+"old-abc.csv", popFolder+"new-def.csv"))

	got, err := svc.SubmitPopulation(context.Background(), 2, 7, 1, popFolder, "", "actor")
	if err != nil {
		t.Fatalf("SubmitPopulation: %v", err)
	}

	if got.PopulationID == 1 || len(repo.rounds) != 2 {
		t.Fatalf("expected a new round, got id %d with %d rounds", got.PopulationID, len(repo.rounds))
	}
	if rejected.Status != "COMPLIANCE_REJECTED" {
		t.Errorf("rejected round status = %q, want it left as COMPLIANCE_REJECTED", rejected.Status)
	}
	if len(repo.files[1]) != 1 {
		t.Errorf("rejected round has %d files, want its original 1", len(repo.files[1]))
	}
	created := repo.round(got.PopulationID)
	if created.Status != "SUBMITTED" {
		t.Errorf("new round status = %q, want SUBMITTED", created.Status)
	}
	if created.OwnerID == nil || *created.OwnerID != 4 || created.TeamID == nil || *created.TeamID != 5 ||
		created.Description == nil || *created.Description != "all accounts" ||
		created.DueDate == nil || *created.DueDate != "2026-10-01" ||
		created.Comments == nil || *created.Comments != "include contractors" {
		t.Errorf("new round did not carry over the requirement details: %+v", created)
	}
	files := repo.files[got.PopulationID]
	if len(files) != 1 || files[0].FilePath != popFolder+"new-def.csv" {
		t.Errorf("new round files = %+v, want only the newly uploaded blob", files)
	}
	if got.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1", got.FileCount)
	}
}

func TestSubmitPopulationRejectedRoundWithNoteOnly(t *testing.T) {
	repo := newFakePopulationRepo(&model.AuditPopulation{ID: 1, Status: "AUDITOR_REJECTED", DueDate: strPtr("2026-10-01")})
	repo.addFile(1, "POPULATION", popFolder+"old.csv")
	svc := NewPopulationService(repo, blobStorage(t, popFolder+"old.csv"))

	got, err := svc.SubmitPopulation(context.Background(), 2, 7, 1, popFolder, "no in-scope items this period", "actor")
	if err != nil {
		t.Fatalf("SubmitPopulation: %v", err)
	}
	created := repo.round(got.PopulationID)
	if got.PopulationID == 1 || created.Attestation == nil || *created.Attestation != "no in-scope items this period" {
		t.Errorf("want a new round carrying the note, got %+v", created)
	}
	if len(repo.files[got.PopulationID]) != 0 {
		t.Errorf("new round got %d files, want none (old blobs stay on the rejected round)", len(repo.files[got.PopulationID]))
	}
}

func TestSubmitPopulationRejectedRoundNeedsNewFilesOrNote(t *testing.T) {
	repo := newFakePopulationRepo(&model.AuditPopulation{ID: 1, Status: "COMPLIANCE_REJECTED", DueDate: strPtr("2026-10-01")})
	repo.addFile(1, "POPULATION", popFolder+"old.csv")
	svc := NewPopulationService(repo, blobStorage(t, popFolder+"old.csv"))

	_, err := svc.SubmitPopulation(context.Background(), 2, 7, 1, popFolder, "  ", "actor")

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("err = %v, want a 422", err)
	}
	if len(repo.rounds) != 1 {
		t.Errorf("a round was created despite the rejected submission")
	}
}

func TestSubmitPopulationReusesOpenRound(t *testing.T) {
	for _, status := range []string{"PENDING", "SUBMITTED"} {
		t.Run(status, func(t *testing.T) {
			repo := newFakePopulationRepo(&model.AuditPopulation{ID: 1, Status: status})
			repo.addFile(1, "POPULATION", popFolder+"first.csv")
			svc := NewPopulationService(repo, blobStorage(t, popFolder+"first.csv", popFolder+"added.csv"))

			got, err := svc.SubmitPopulation(context.Background(), 2, 7, 1, popFolder, "", "actor")
			if err != nil {
				t.Fatalf("SubmitPopulation: %v", err)
			}
			if got.PopulationID != 1 || len(repo.rounds) != 1 {
				t.Errorf("round %d of %d, want the open round reused", got.PopulationID, len(repo.rounds))
			}
			if len(repo.files[1]) != 2 {
				t.Errorf("round has %d files, want the existing one plus the added one", len(repo.files[1]))
			}
			if got.FileCount != 2 || repo.rounds[0].Status != "SUBMITTED" {
				t.Errorf("FileCount = %d, status = %q; want 2 and SUBMITTED", got.FileCount, repo.rounds[0].Status)
			}
		})
	}
}

func TestSubmitPopulationRejectsAnEarlierRound(t *testing.T) {
	repo := newFakePopulationRepo(
		&model.AuditPopulation{ID: 1, Status: "COMPLIANCE_REJECTED"},
		&model.AuditPopulation{ID: 2, Status: "APPROVED"},
	)
	svc := NewPopulationService(repo, blobStorage(t, popFolder+"x.csv"))

	_, err := svc.SubmitPopulation(context.Background(), 2, 7, 1, popFolder, "", "actor")

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v, want a 409", err)
	}
	if len(repo.rounds) != 2 {
		t.Errorf("a round was created from a superseded one")
	}
}

func TestSubmitSampleSkipsBlobsRecordedOnEarlierRounds(t *testing.T) {
	const sampleFolder = popFolder + "sample/"
	repo := newFakePopulationRepo(
		&model.AuditPopulation{ID: 1, Status: "AUDITOR_REJECTED"},
		&model.AuditPopulation{ID: 2, Status: "APPROVED"},
	)
	repo.addFile(1, "SAMPLE", sampleFolder+"old-sample.csv")
	svc := NewPopulationService(repo, blobStorage(t, sampleFolder+"old-sample.csv", sampleFolder+"new-sample.csv"))

	count, err := svc.SubmitSample(context.Background(), 2, 7, 2, sampleFolder, "auditor")
	if err != nil {
		t.Fatalf("SubmitSample: %v", err)
	}
	files := repo.files[2]
	if len(files) != 1 || files[0].FilePath != sampleFolder+"new-sample.csv" {
		t.Errorf("round 2 sample files = %+v, want only the new blob", files)
	}
	if count != 2 {
		t.Errorf("count = %d, want the folder's total of 2", count)
	}
}
