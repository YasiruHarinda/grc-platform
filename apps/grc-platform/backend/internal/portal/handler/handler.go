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

// Package handler serves the Evidence Portal machine-to-machine ingress
// (/api/v1/evidence-portal/*): its own ServeMux, ClientCredentials auth, no
// callerGuard or grant load. Authorization is the token-derived team scope.
package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	auditrepo "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
)

const (
	// maxEvidenceFileBytes mirrors the web-app per-file evidence cap.
	maxEvidenceFileBytes = 25 << 20
	// maxPortalFilesPerSubmit caps how many screenshots one submission carries;
	// all of them go into a single evidence round.
	maxPortalFilesPerSubmit = 20
	// maxPortalRequestBytes bounds the whole multipart body: the full file
	// count at the per-file cap, plus slack for multipart framing. Derived so
	// it cannot drift below a submission the per-file rules already allow.
	maxPortalRequestBytes = maxPortalFilesPerSubmit*maxEvidenceFileBytes + 8<<20
	// sniffSize is how many leading bytes http.DetectContentType inspects —
	// all a type check needs to read, so a part is never pulled into memory
	// whole just to classify it.
	sniffSize = 512
)

// worklistStatuses are the control statuses the GET worklist returns (all
// members of the audit_control status enum). The sample-phase pair is returned
// for visibility only; the POST accepts just the evidence-phase pair.
var worklistStatuses = []string{
	"EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION",
	"AWAITING_SAMPLE", "SUBMITTED_SAMPLE",
}

// evidenceUploadStatuses are the only statuses a portal upload may target.
var evidenceUploadStatuses = map[string]bool{
	"EVIDENCE_PENDING":            true,
	"EVIDENCE_NEED_CLARIFICATION": true,
}

// Deps are the reused audit services the portal handler composes.
type Deps struct {
	// Submit runs the web-app evidence-submission pipeline (upload + submit +
	// advance + notify + trail + AI) for one portal file.
	Submit *audithandler.Deps
	// Audits lists audits for the ACTIVE filter and the audit/product/framework
	// enrichment on the worklist.
	Audits auditservice.AuditService
	// Directory resolves an email to exactly one person (attribution).
	Directory *directory.Service
	// Grants resolves a person's uuid to their internal user.id for the
	// owner comparison.
	Grants grant.Repository
	// Controls reads the cross-audit, team-scoped control worklist and a
	// single control by id.
	Controls auditrepo.PortalControlReader
}

type portalHandler struct{ deps Deps }

// RegisterRoutes mounts the portal routes on mux. This mux is NOT the
// routeguard Router — the portal is deliberately outside the callerGuard /
// external-auditor regime.
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	h := &portalHandler{deps: deps}
	mux.HandleFunc("GET /api/v1/evidence-portal/controls", h.listControls)
	mux.HandleFunc("POST /api/v1/evidence-portal/controls/{controlId}/evidences", h.submitEvidence)
}

// ── GET /api/v1/evidence-portal/controls ─────────────────────────────────────

type portalAuditRef struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Product   string `json:"product"`
	Framework string `json:"framework"`
}

type portalControlRef struct {
	ID      int     `json:"id"`
	Ref     string  `json:"ref"`
	Title   string  `json:"title"`
	Status  string  `json:"status"`
	DueDate *string `json:"dueDate"`
	OwnerID *int    `json:"ownerId"`
}

type portalControlRow struct {
	Audit   portalAuditRef   `json:"audit"`
	Control portalControlRef `json:"control"`
}

func (h *portalHandler) listControls(w http.ResponseWriter, r *http.Request) {
	caller := middleware.PortalCallerFromContext(r.Context())
	if caller == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}

	// email narrows the worklist to one owner. An address that does not resolve
	// must never widen the answer back to the whole team: this endpoint is how
	// the portal decides what to show one person, so a typo would otherwise
	// hand them everyone's work. An unknown address gets an empty worklist —
	// empty rather than an error, so a caller cannot tell a directory miss
	// apart from a person with nothing to do (a distinguishable answer would be
	// a directory-enumeration oracle). A lookup that could not be COMPLETED is
	// a different thing and must not be reported as "nothing to do".
	var ownerIDs []int
	if email := strings.TrimSpace(r.URL.Query().Get("email")); email != "" {
		id, rErr := h.resolveOwnerID(r.Context(), email)
		switch {
		case errors.Is(rErr, errOwnerUnknown):
			response.WriteJSONValue(w, http.StatusOK, []portalControlRow{})
			return
		case rErr != nil:
			slog.ErrorContext(r.Context(), "portal worklist email filter lookup failed", "err", rErr)
			response.WriteError(w, http.StatusServiceUnavailable, "could not resolve the email filter")
			return
		}
		ownerIDs = []int{id}
	}

	controls, err := h.deps.Controls.TeamControls(r.Context(), caller.TeamID, worklistStatuses, ownerIDs)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	audits, err := h.deps.Audits.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	activeAudits := make(map[int]*model.Audit, len(audits))
	for _, a := range audits {
		if a.Status == "ACTIVE" {
			activeAudits[a.ID] = a
		}
	}

	rows := make([]portalControlRow, 0, len(controls))
	for _, c := range controls {
		a, ok := activeAudits[c.AuditID]
		if !ok {
			continue
		}
		rows = append(rows, portalControlRow{
			Audit: portalAuditRef{
				ID: a.ID, Name: a.Name,
				Product: a.Product.Name, Framework: a.Framework.Name,
			},
			Control: portalControlRef{
				ID: c.ID, Ref: c.ControlNumber, Title: c.Description,
				Status: c.Status, DueDate: c.DueDate, OwnerID: c.OwnerID,
			},
		})
	}
	response.WriteJSONValue(w, http.StatusOK, rows)
}

// ── POST /api/v1/evidence-portal/controls/{controlId}/evidences ──────────────

func (h *portalHandler) submitEvidence(w http.ResponseWriter, r *http.Request) {
	caller := middleware.PortalCallerFromContext(r.Context())
	if caller == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil || controlID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "controlId must be a positive integer")
		return
	}

	// 1. Authorization — team scope from the verified token, decided before any
	// request-body byte is read so the access decision never depends on a
	// caller-supplied field. Missing control, NULL team, or another team's
	// control are one indistinguishable 404.
	control, err := h.deps.Controls.ControlByID(r.Context(), controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control == nil || control.TeamID == nil || *control.TeamID != caller.TeamID {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}

	// 2. Status — a sample-phase control can appear in the worklist but cannot
	// take an evidence upload.
	if !evidenceUploadStatuses[control.Status] {
		response.WriteError(w, http.StatusConflict, "control is not in an evidence-phase status")
		return
	}

	// 3. Body — bounded, then parsed. Only past the team boundary is a
	// caller-supplied byte read.
	r.Body = http.MaxBytesReader(w, r.Body, maxPortalRequestBytes)
	if err := r.ParseMultipartForm(maxEvidenceFileBytes); err != nil { // #nosec G120 -- bounded by MaxBytesReader
		response.WriteError(w, http.StatusRequestEntityTooLarge, "upload too large or malformed (25 MiB per file)")
		return
	}

	// 4. Attribution — resolve email and confirm control ownership. Unresolvable,
	// ambiguous, and not-the-owner all return one 400 (a distinguishable
	// response would be a directory-enumeration oracle); the reason is logged.
	email := strings.TrimSpace(r.FormValue("email"))
	person, err := h.deps.Directory.ResolveEmail(r.Context(), email)
	if err != nil {
		slog.WarnContext(r.Context(), "portal attribution failed", "reason", err.Error())
		response.WriteError(w, http.StatusBadRequest, "could not attribute this submission")
		return
	}
	submitter, gErr := h.deps.Grants.ForUUID(r.Context(), person.UUID)
	if gErr != nil {
		response.MapServiceError(r.Context(), w, gErr, response.ErrMsgInternal)
		return
	}
	if submitter.UserID == 0 || control.OwnerID == nil || *control.OwnerID != submitter.UserID {
		slog.WarnContext(r.Context(), "portal attribution failed", "reason", "email is not the control owner")
		response.WriteError(w, http.StatusBadRequest, "could not attribute this submission")
		return
	}

	// 5. Files — one or more `file` parts; all land in a single evidence round.
	// Same type/size checks the web-app upload applies (neither is malware
	// scanning; this platform performs none).
	var headers []*multipart.FileHeader
	if r.MultipartForm != nil {
		headers = r.MultipartForm.File["file"]
	}
	if len(headers) == 0 {
		response.WriteError(w, http.StatusBadRequest, "at least one file is required")
		return
	}
	if len(headers) > maxPortalFilesPerSubmit {
		response.WriteError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d per submission)", maxPortalFilesPerSubmit))
		return
	}
	files := make([]audithandler.PortalEvidenceFile, 0, len(headers))
	for _, hdr := range headers {
		// hdr.Size is the part's real length, counted by the multipart reader —
		// so the oversize check costs no read.
		if hdr.Size > maxEvidenceFileBytes {
			response.WriteError(w, http.StatusRequestEntityTooLarge, "a file exceeds the 25 MiB limit")
			return
		}
		head, err := readFileHead(hdr)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "could not read uploaded file")
			return
		}
		fileName := filepath.Base(hdr.Filename)
		sniffed := http.DetectContentType(head)
		contentType := hdr.Header.Get("Content-Type")
		if contentType == "" {
			contentType = sniffed
		}
		// Both the declared and the sniffed type must pass. A machine client
		// writes its own Content-Type header, so the declared value alone is
		// evidence of nothing: HTML sent as "application/pdf" under a .pdf name
		// would otherwise clear both the extension and the type check.
		if err := audithandler.ValidateUploadFileType(fileName, contentType); err != nil {
			response.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := audithandler.ValidateUploadFileType(fileName, sniffed); err != nil {
			response.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		// The part is handed over as an opener, not as bytes: the bridge reads
		// one file at a time, so a full 20-file submission costs one file of
		// memory instead of twenty.
		files = append(files, audithandler.PortalEvidenceFile{
			FileName:    fileName,
			ContentType: contentType,
			Open:        func() (io.ReadCloser, error) { return hdr.Open() },
		})
	}

	// 6. The web-app submission pipeline, with the resolved uuid as the actor.
	evidence, err := h.deps.Submit.SubmitPortalEvidence(r.Context(), control.AuditID, controlID,
		files, person.UUID, caller.ClientID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, evidence)
}

// readFileHead reads just the leading bytes a content-type sniff needs. A
// short part is not an error — it is classified on what it has.
func readFileHead(hdr *multipart.FileHeader) ([]byte, error) {
	f, err := hdr.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, sniffSize)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return head[:n], nil
}

// errOwnerUnknown means the email resolved to nobody this platform can scope a
// worklist to — the directory does not know the address, or the person it names
// has no user row. Kept distinct from a lookup that could not be completed at
// all, because the two must not produce the same response.
var errOwnerUnknown = errors.New("email does not resolve to a known owner")

// resolveOwnerID resolves an email to the internal user.id used as
// audit_control.owner_id. Returns errOwnerUnknown for "no such owner"; any
// other error means the question went unanswered.
func (h *portalHandler) resolveOwnerID(ctx context.Context, email string) (int, error) {
	person, err := h.deps.Directory.ResolveEmail(ctx, email)
	if err != nil {
		if errors.Is(err, directory.ErrEmailUnresolved) {
			return 0, errOwnerUnknown
		}
		return 0, fmt.Errorf("resolve email: %w", err)
	}
	caller, err := h.deps.Grants.ForUUID(ctx, person.UUID)
	if err != nil {
		return 0, fmt.Errorf("resolve owner grant: %w", err)
	}
	if caller.UserID == 0 {
		return 0, errOwnerUnknown
	}
	return caller.UserID, nil
}
