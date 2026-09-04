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

package integration

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository/entity"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	portalhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/portal/handler"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
)

const (
	epIssuer   = "https://idp.example/oauth2/token"
	epWebAud   = "webapp-consumer-key"
	epPort3Aud = "evidence-portal-consumer-key"
	epClientID = "evidence-portal-consumer-key"
	epTeamID   = 7
	epOwnerID  = 5
	epOwnerUID = "owner-uuid"
	epOwnerEml = "owner@wso2.com"
)

var epKey = func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}()

func epToken(t *testing.T, aud, sub string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": epIssuer, "aud": aud, "sub": sub,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}).SignedString(epKey)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// fakeEntity stands in for the Compliance Entity. It answers only the routes
// the portal flow touches; everything else returns 200 {} so the best-effort
// notification calls in the submit pipeline never fail the request.
type fakeEntity struct {
	mu              sync.Mutex
	controlStatus   string
	controlTeamID   *int
	statusPatched   string
	trailPosted     int
	evidenceCreated int
	blobUploaded    int
}

func newFakeEntity(status string, teamID *int) *fakeEntity {
	return &fakeEntity{controlStatus: status, controlTeamID: teamID}
}

func (f *fakeEntity) control() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := map[string]any{
		"id": 811, "auditId": 42, "status": f.controlStatus,
		"ownerId": epOwnerID, "controlNumber": "CC6.1",
		"description": "Logical access provisioning",
	}
	if f.controlTeamID != nil {
		c["teamId"] = *f.controlTeamID
	}
	return c
}

func (f *fakeEntity) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /controls/search", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ControlIDs []int `json:"controlIds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.ControlIDs) > 0 { // single-control lookup (POST path)
			writeJSON(w, map[string]any{"controls": []any{f.control()}})
			return
		}
		// worklist: one control in an ACTIVE audit, one in a CLOSED audit.
		writeJSON(w, map[string]any{"controls": []any{
			f.control(),
			map[string]any{"id": 812, "auditId": 99, "status": "EVIDENCE_PENDING", "teamId": epTeamID, "controlNumber": "CC7.2", "description": "Change management"},
		}})
	})

	mux.HandleFunc("POST /audits/search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"audits": []any{
			map[string]any{"id": 42, "name": "SOC 2 2026", "status": "ACTIVE", "frameworkName": "SOC 2", "productName": "Choreo"},
			map[string]any{"id": 99, "name": "Old Audit", "status": "CLOSED", "frameworkName": "SOC 2", "productName": "Choreo"},
		}})
	})

	mux.HandleFunc("GET /audits/42", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"id": 42, "name": "SOC 2 2026", "status": "ACTIVE", "frameworkName": "SOC 2", "productName": "Choreo"})
	})
	mux.HandleFunc("GET /audits/42/controls/811", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, f.control())
	})
	mux.HandleFunc("PATCH /audits/42/controls/811", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.statusPatched = body.Status
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /audits/42/controls/811/evidence", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.evidenceCreated++
		f.mu.Unlock()
		writeJSON(w, map[string]any{"id": 555, "controlId": 811, "status": "SUBMITTED"})
	})
	mux.HandleFunc("POST /evidence/555/files", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /audits/42/trail", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.trailPosted++
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /files", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.blobUploaded++
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /grants/by-uuid/"+epOwnerUID, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"userId": epOwnerID, "userType": "INTERNAL", "grants": []any{}})
	})

	// Catch-all: best-effort notification lookups must not 5xx the request.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"candidates": []any{}})
	})
	return mux
}

func fakeSCIM(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("POST /t/wso2/scim2/Users/.search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Filter string `json:"filter"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if !strings.Contains(in.Filter, epOwnerEml) {
			writeJSON(w, map[string]any{"totalResults": 0, "Resources": []any{}})
			return
		}
		writeJSON(w, map[string]any{
			"totalResults": 1,
			"Resources": []any{map[string]any{
				"id": epOwnerUID, "userName": epOwnerEml,
				"name": map[string]any{"givenName": "Owner", "familyName": "Person"},
			}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// buildPortalStack wires the same handler chain main.go builds: shared IdP
// verifier, the prefix fork, both rate limiters, ClientCredentials, and the
// real portal handler over real audit services pointed at fakeEnt.
func buildPortalStack(t *testing.T, fakeEnt *fakeEntity, withDirectory bool) *httptest.Server {
	t.Helper()
	entURL := httptest.NewServer(fakeEnt.handler())
	t.Cleanup(entURL.Close)
	ec := entityclient.New(entURL.URL)

	var dirSvc *directory.Service
	if withDirectory {
		sc := fakeSCIM(t)
		dirSvc = directory.New(scim.NewClient(sc.URL, sc.URL+"/token", "id", "secret", "", "wso2"), time.Hour)
	}

	trailRepo := auditentity.NewTrailRepository(ec)
	trailSvc := auditservice.NewTrailService(trailRepo)
	auditRepo := auditentity.NewAuditRepository(ec)
	controlRepo := auditentity.NewControlRepository(ec)
	populationRepo := auditentity.NewPopulationRepository(ec)
	evidenceRepo := auditentity.NewEvidenceRepository(ec)
	fileSvc := file.NewService(entURL.URL)

	auditDeps := audithandler.Deps{
		Audit:      auditservice.NewAuditService(auditRepo, auditentity.NewFrameworkRepository(ec), auditentity.NewProductRepository(ec), trailSvc),
		Control:    auditservice.NewControlService(controlRepo, populationRepo, trailSvc, dirSvc),
		Evidence:   auditservice.NewEvidenceService(evidenceRepo, auditRepo, controlRepo, fileSvc),
		Population: auditservice.NewPopulationService(populationRepo, fileSvc),
		Trail:      trailSvc,
		Directory:  dirSvc,
		Grants:     grant.NewRepository(ec),
		Users:      auditentity.NewUserRepository(ec),
		Email:      emailer.New("", "", "", "", "", false),
	}

	verifier, err := middleware.NewIdPVerifierWithKeyFuncs(
		[]config.IdPConfig{{Issuer: epIssuer, Audience: epWebAud}},
		map[string]jwt.Keyfunc{epIssuer: func(*jwt.Token) (any, error) { return &epKey.PublicKey, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}

	var handler http.Handler = middleware.Auth(middleware.Config{
		TokenValidatorEnabled: true, Verifier: verifier, ClockSkew: 5 * time.Second,
	})(http.NewServeMux())

	portalMux := http.NewServeMux()
	portalhandler.RegisterRoutes(portalMux, portalhandler.Deps{
		Submit: &auditDeps, Audits: auditDeps.Audit,
		Directory: dirSvc, Grants: auditDeps.Grants, Controls: auditentity.NewPortalControlReader(ec),
	})
	portalChain := middleware.PortalPerRemoteAddrRateLimit(
		middleware.ClientCredentials(middleware.ClientCredConfig{
			Verifier: verifier, Audience: epPort3Aud,
			Clients: map[string]int{epClientID: epTeamID}, ClockSkew: 5 * time.Second,
		})(middleware.PortalPerClientRateLimit(portalMux)),
	)
	base := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/evidence-portal/") {
			if strings.Contains(r.URL.Path, "..") {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			portalChain.ServeHTTP(w, r)
			return
		}
		base.ServeHTTP(w, r)
	})
	handler = middleware.SecurityHeaders(
		middleware.CORS("http://localhost:3000")(
			middleware.CorrelationID(middleware.Logger(handler))))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func doPortal(t *testing.T, srv *httptest.Server, method, path, token string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func evidenceMultipart(t *testing.T, email string, fileNames ...string) (*bytes.Buffer, string) {
	t.Helper()
	if len(fileNames) == 0 {
		fileNames = []string{"shot.png"}
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if email != "" {
		_ = mw.WriteField("email", email)
	}
	for _, name := range fileNames {
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\nfake-bytes"))
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

// --- tests -----------------------------------------------------------------

func TestPortalIntegration_WorklistHappyPath(t *testing.T) {
	team := epTeamID
	srv := buildPortalStack(t, newFakeEntity("EVIDENCE_PENDING", &team), false)

	resp := doPortal(t, srv, http.MethodGet, "/api/v1/evidence-portal/controls",
		epToken(t, epPort3Aud, epClientID), nil, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var rows []struct {
		Audit struct {
			ID            int
			Name, Product string
			Framework     string
		}
		Control struct {
			ID         int
			Ref, Title string
			Status     string
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row (CLOSED audit filtered out), got %d: %+v", len(rows), rows)
	}
	if rows[0].Audit.ID != 42 || rows[0].Audit.Name != "SOC 2 2026" ||
		rows[0].Audit.Product != "Choreo" || rows[0].Audit.Framework != "SOC 2" {
		t.Fatalf("audit not enriched: %+v", rows[0].Audit)
	}
	if rows[0].Control.ID != 811 || rows[0].Control.Ref != "CC6.1" {
		t.Fatalf("control fields wrong: %+v", rows[0].Control)
	}
}

func TestPortalIntegration_WebappTokenRejectedThroughFullChain(t *testing.T) {
	team := epTeamID
	srv := buildPortalStack(t, newFakeEntity("EVIDENCE_PENDING", &team), false)

	resp := doPortal(t, srv, http.MethodGet, "/api/v1/evidence-portal/controls",
		epToken(t, epWebAud, "some-user"), nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("webapp-audience token must be 401 on a portal route, got %d", resp.StatusCode)
	}
}

func TestPortalIntegration_DotSegmentRejected(t *testing.T) {
	team := epTeamID
	srv := buildPortalStack(t, newFakeEntity("EVIDENCE_PENDING", &team), false)
	resp := doPortal(t, srv, http.MethodGet, "/api/v1/evidence-portal/../secrets",
		epToken(t, epPort3Aud, epClientID), nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for a dot-segment path, got %d", resp.StatusCode)
	}
}

func TestPortalIntegration_SubmitWrongTeamIs404(t *testing.T) {
	other := 99
	srv := buildPortalStack(t, newFakeEntity("EVIDENCE_PENDING", &other), true)

	body, ct := evidenceMultipart(t, epOwnerEml)
	resp := doPortal(t, srv, http.MethodPost, "/api/v1/evidence-portal/controls/811/evidences",
		epToken(t, epPort3Aud, epClientID), body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestPortalIntegration_SubmitSampleStatusIs409(t *testing.T) {
	team := epTeamID
	srv := buildPortalStack(t, newFakeEntity("SUBMITTED_SAMPLE", &team), true)

	body, ct := evidenceMultipart(t, epOwnerEml)
	resp := doPortal(t, srv, http.MethodPost, "/api/v1/evidence-portal/controls/811/evidences",
		epToken(t, epPort3Aud, epClientID), body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409, got %d", resp.StatusCode)
	}
}

func TestPortalIntegration_SubmitHappyPath_MultipleFilesOneRound(t *testing.T) {
	team := epTeamID
	fe := newFakeEntity("EVIDENCE_PENDING", &team)
	srv := buildPortalStack(t, fe, true)

	body, ct := evidenceMultipart(t, epOwnerEml, "a.png", "b.png", "c.png")
	resp := doPortal(t, srv, http.MethodPost, "/api/v1/evidence-portal/controls/811/evidences",
		epToken(t, epPort3Aud, epClientID), body, ct)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 201, got %d: %s", resp.StatusCode, b)
	}
	fe.mu.Lock()
	defer fe.mu.Unlock()
	if fe.blobUploaded != 3 {
		t.Errorf("blob uploaded %d times, want 3 (one per file)", fe.blobUploaded)
	}
	if fe.evidenceCreated != 1 {
		t.Errorf("evidence rounds created %d, want 1 (all files in one round)", fe.evidenceCreated)
	}
	if fe.statusPatched != "EVIDENCE_INTERNAL_REVIEW" {
		t.Errorf("control status patched to %q, want EVIDENCE_INTERNAL_REVIEW", fe.statusPatched)
	}
	if fe.trailPosted != 1 {
		t.Errorf("trail entries %d, want 1", fe.trailPosted)
	}
}

func TestPortalIntegration_SubmitNoFileIs400(t *testing.T) {
	team := epTeamID
	srv := buildPortalStack(t, newFakeEntity("EVIDENCE_PENDING", &team), true)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("email", epOwnerEml)
	_ = mw.Close()

	resp := doPortal(t, srv, http.MethodPost, "/api/v1/evidence-portal/controls/811/evidences",
		epToken(t, epPort3Aud, epClientID), &buf, mw.FormDataContentType())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 with no file part, got %d", resp.StatusCode)
	}
}

func TestPortalIntegration_SubmitWrongEmailIs400(t *testing.T) {
	team := epTeamID
	srv := buildPortalStack(t, newFakeEntity("EVIDENCE_PENDING", &team), true)

	body, ct := evidenceMultipart(t, "stranger@wso2.com")
	resp := doPortal(t, srv, http.MethodPost, "/api/v1/evidence-portal/controls/811/evidences",
		epToken(t, epPort3Aud, epClientID), body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for an email that does not resolve, got %d", resp.StatusCode)
	}
}
