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

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

const (
	visAuditID   = 2
	visControlID = 7
	visTeamID    = 5
	// visAuditorUserID is the control's assigned auditor: an external user who
	// holds none of the internal-audience privileges.
	visAuditorUserID = 42
)

type stubControlService struct {
	service.ControlService
	control *model.AuditControl
}

func (s stubControlService) GetByID(context.Context, int, int) (*model.AuditControl, error) {
	return s.control, nil
}

type stubPopulationService struct {
	service.PopulationService
	rounds []*model.AuditPopulation
	files  map[int][]*model.PopulationFile // by round id
}

func (s stubPopulationService) ListRounds(context.Context, int, int) ([]*model.AuditPopulation, error) {
	return s.rounds, nil
}

func (s stubPopulationService) ListFiles(_ context.Context, roundID int) ([]*model.PopulationFile, error) {
	return s.files[roundID], nil
}

func (s stubPopulationService) GetFileByID(_ context.Context, fileID int) (*model.PopulationFile, error) {
	for _, files := range s.files {
		for _, f := range files {
			if f.ID == fileID {
				return f, nil
			}
		}
	}
	return nil, nil
}

func (s stubPopulationService) DownloadFile(_ context.Context, fileID int) ([]byte, string, string, error) {
	return []byte("file-bytes"), "f.csv", "text/csv", nil
}

// Each file's name says which round it is on, so a leak is visible in the body.
func popFile(id, roundID int, kind string) *model.PopulationFile {
	teamID, auditorID := visTeamID, visAuditorUserID
	return &model.PopulationFile{
		ID: id, PopulationID: roundID, FileKind: kind, TeamID: &teamID, AuditorID: &auditorID,
		FileName: strings.ToLower(kind) + "-in-round-" + string(rune('0'+roundID)) + ".csv",
	}
}

func note(s string) *string { return &s }

// rejectedHistory is a control whose team was rejected twice before the
// current round: round 1 COMPLIANCE_REJECTED, round 2 AUDITOR_REJECTED, round 3
// SUBMITTED (current, awaiting review).
func rejectedHistory() ([]*model.AuditPopulation, map[int][]*model.PopulationFile) {
	rounds := []*model.AuditPopulation{
		{ID: 1, ControlID: visControlID, Status: "COMPLIANCE_REJECTED", Attestation: note("note-on-rejected-round-1")},
		{ID: 2, ControlID: visControlID, Status: "AUDITOR_REJECTED"},
		{ID: 3, ControlID: visControlID, Status: "SUBMITTED"},
	}
	files := map[int][]*model.PopulationFile{
		1: {popFile(10, 1, "POPULATION")},
		2: {popFile(20, 2, "POPULATION")},
		3: {popFile(30, 3, "POPULATION")},
	}
	return rounds, files
}

func newVisibilityHandler(rounds []*model.AuditPopulation, files map[int][]*model.PopulationFile) *evidenceHandler {
	teamID, auditorID := visTeamID, visAuditorUserID
	return &evidenceHandler{
		controlSvc: stubControlService{control: &model.AuditControl{ID: visControlID, AuditID: visAuditID, TeamID: &teamID, AuditorID: &auditorID}},
		popSvc:     stubPopulationService{rounds: rounds, files: files},
	}
}

// requestAs builds a request for the given caller. The privilege map is always
// non-nil, so the caller is judged by their grants rather than falling into the
// "no privilege store configured" allow-all mode.
func requestAs(t *testing.T, userID int, global map[string]bool, path string, pathValues map[string]string) *http.Request {
	t.Helper()
	set := grant.NewForTest(global, nil, 1)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "uuid", UserID: userID})
	ctx = grant.WithContext(ctx, set)
	ctx = privilege.WithContext(ctx, set.PrivilegeMap())
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	req.SetPathValue("id", "2")
	req.SetPathValue("controlId", "7")
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	return req
}

var (
	externalAuditor = map[string]bool{privilege.ValidateEvidence: true, privilege.SelectSample: true}
	internalTeam    = map[string]bool{privilege.SubmitEvidence: true}
)

const listPath = "/api/v1/audits/2/controls/7/population"

func listAs(t *testing.T, h *evidenceHandler, userID int, global map[string]bool) (*httptest.ResponseRecorder, model.PopulationView) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.listPopulation(rec, requestAs(t, userID, global, listPath, nil))
	var view model.PopulationView
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec, view
}

func TestExternalAuditorListDropsRejectedRounds(t *testing.T) {
	rounds, files := rejectedHistory()
	h := newVisibilityHandler(rounds, files)

	rec, view := listAs(t, h, visAuditorUserID, externalAuditor)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(view.EarlierRounds) != 0 {
		t.Errorf("earlierRounds = %d, want none (both earlier rounds were rejected)", len(view.EarlierRounds))
	}
	if view.Round.ID != 3 || len(view.PopulationFiles) != 1 || view.PopulationFiles[0].ID != 30 {
		t.Errorf("current round = %d with files %+v, want round 3 with its own file", view.Round.ID, view.PopulationFiles)
	}
	body := rec.Body.String()
	for _, leaked := range []string{"population-in-round-1", "population-in-round-2", "note-on-rejected-round-1"} {
		if strings.Contains(body, leaked) {
			t.Errorf("response leaks %q to an external auditor:\n%s", leaked, body)
		}
	}
}

// A rejected round that is still the current one (the team hasn't resubmitted
// yet) must not show its files or note either. The auditor's own sample files
// are not the team's rejected submission and stay.
func TestExternalAuditorListHidesRejectedCurrentRound(t *testing.T) {
	rounds := []*model.AuditPopulation{
		{ID: 1, ControlID: visControlID, Status: "AUDITOR_REJECTED", Attestation: note("note-on-rejected-round-1")},
	}
	files := map[int][]*model.PopulationFile{
		1: {popFile(10, 1, "POPULATION"), popFile(11, 1, "SAMPLE")},
	}
	h := newVisibilityHandler(rounds, files)

	rec, view := listAs(t, h, visAuditorUserID, externalAuditor)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(view.PopulationFiles) != 0 {
		t.Errorf("populationFiles = %+v, want none while the round is rejected", view.PopulationFiles)
	}
	if view.Round.Attestation != nil {
		t.Errorf("attestation = %q, want it stripped", *view.Round.Attestation)
	}
	// The status literal is the same leak as the attestation/files: without
	// masking it too, an external auditor still reads AUDITOR_REJECTED /
	// COMPLIANCE_REJECTED off the round and learns of the internal rejection.
	if view.Round.Status != "PENDING" {
		t.Errorf("status = %q, want it masked to PENDING like the attestation and files", view.Round.Status)
	}
	if len(view.SampleFiles) != 1 {
		t.Errorf("sampleFiles = %d, want the auditor's own sample kept", len(view.SampleFiles))
	}
	if strings.Contains(rec.Body.String(), "population-in-round-1") || strings.Contains(rec.Body.String(), "note-on-rejected-round-1") ||
		strings.Contains(rec.Body.String(), "AUDITOR_REJECTED") || strings.Contains(rec.Body.String(), "COMPLIANCE_REJECTED") {
		t.Errorf("response leaks the rejected round:\n%s", rec.Body.String())
	}
}

// Control case: the same data does reach an internal team member, so the
// external-auditor assertions above are not passing on an empty fixture.
func TestInternalViewerListKeepsRejectedRounds(t *testing.T) {
	rounds, files := rejectedHistory()
	h := newVisibilityHandler(rounds, files)

	rec, view := listAs(t, h, 1, internalTeam)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(view.EarlierRounds) != 2 {
		t.Fatalf("earlierRounds = %d, want both rejected rounds", len(view.EarlierRounds))
	}
	for i, want := range []string{"COMPLIANCE_REJECTED", "AUDITOR_REJECTED"} {
		if got := view.EarlierRounds[i].Round.Status; got != want || len(view.EarlierRounds[i].PopulationFiles) != 1 {
			t.Errorf("earlier round %d: status %q with %d files, want %q with 1", i, got, len(view.EarlierRounds[i].PopulationFiles), want)
		}
	}
}

func TestListRefusesACallerWithNoAccess(t *testing.T) {
	rounds, files := rejectedHistory()
	h := newVisibilityHandler(rounds, files)

	rec, _ := listAs(t, h, 999, externalAuditor) // an auditor, but not this control's

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func downloadAs(t *testing.T, h *evidenceHandler, userID int, global map[string]bool, fileID string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	pv := map[string]string{"fileId": fileID}
	for k, v := range pathValues {
		pv[k] = v
	}
	rec := httptest.NewRecorder()
	h.downloadPopulationFile(rec, requestAs(t, userID, global, listPath+"/files/"+fileID+"/download", pv))
	return rec
}

func TestExternalAuditorDownload(t *testing.T) {
	rounds, files := rejectedHistory()
	// A sample the auditor selected on the (rejected) round 2 stays readable.
	files[2] = append(files[2], popFile(21, 2, "SAMPLE"))
	h := newVisibilityHandler(rounds, files)

	cases := []struct {
		name       string
		fileID     string
		pathValues map[string]string
		want       int
	}{
		{"file of a COMPLIANCE_REJECTED round", "10", nil, http.StatusForbidden},
		{"file of an AUDITOR_REJECTED round", "20", nil, http.StatusForbidden},
		{"file of the current SUBMITTED round", "30", nil, http.StatusOK},
		{"auditor's own sample on a rejected round", "21", nil, http.StatusOK},
		// The file's round has to belong to the control in the URL; otherwise a
		// rejected file could be reached through another control's route.
		{"rejected file through another control's URL", "10", map[string]string{"controlId": "8"}, http.StatusForbidden},
		{"current-round file through another control's URL", "30", map[string]string{"controlId": "8"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// rounds are looked up by the URL's control, so another control has none
			hh := h
			if tc.pathValues["controlId"] == "8" {
				hh = newVisibilityHandler(nil, files)
			}
			rec := downloadAs(t, hh, visAuditorUserID, externalAuditor, tc.fileID, tc.pathValues)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusForbidden && strings.Contains(rec.Body.String(), "file-bytes") {
				t.Errorf("a 403 response still carried the file bytes")
			}
		})
	}
}

func TestInternalViewerCanDownloadRejectedFiles(t *testing.T) {
	rounds, files := rejectedHistory()
	h := newVisibilityHandler(rounds, files)

	for _, fileID := range []string{"10", "20"} {
		if rec := downloadAs(t, h, 1, internalTeam, fileID, nil); rec.Code != http.StatusOK {
			t.Errorf("internal download of file %s: status = %d, want 200", fileID, rec.Code)
		}
	}
}
