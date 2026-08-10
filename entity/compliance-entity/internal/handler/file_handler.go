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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/storage"
)

// blobSegment matches one path-safe audit-name or control-number segment, as
// produced by the GRC Backend's sanitizeSegment (internal/audit/service/pathsafe.go):
// letters, digits, spaces, "_", and "-" — notably no "/", so it can never itself
// contain a path separator or "..".
const blobSegment = `[A-Za-z0-9 _-]+`

// validBlobPath enforces the allowed Azure Blob layouts (defense in depth: even a
// buggy or compromised backend cannot write outside these trees). The pattern is
// fully anchored (^ … $); the final filename segment may contain any character
// except "/" (real filenames with parentheses, @, commas, +, etc. — the backend
// sanitizes and UUID-suffixes it before it ever reaches here). Backslash and
// ".." are rejected explicitly in guardBlobPath. Every current caller of
// Upload/Read/Delete is enumerated here (see Human-Readable-Evidence-Blob-Paths
// design in the GRC Backend's Resources/Docs). Audit Hub evidence lives under a
// literal "audit/" top-level folder, kept separate from the Risk module's own
// top-level tree — "risk/" (singular, matching audit/'s convention). The Risk
// module's folder segment is the risk_code (e.g. "2026-ASG-Q2-001"), not the
// numeric risk id, so it reuses the same blobSegment charset as audit names:
//   - audit evidence:            audit/{auditName}/{controlNumber}/evidence/{file}
//   - audit population:          audit/{auditName}/{controlNumber}/population/{file}
//   - auditor sample:            audit/{auditName}/{controlNumber}/population/sample/{file}
//   - risk evidence attachment:  risk/{riskCode}/risk-evidence-attachment/{file}
//   - risk completion attachment: risk/{riskCode}/risk-action-plan-completion-attachment/{file}
var validBlobPath = regexp.MustCompile(
	`^(?:audit/` + blobSegment + `/` + blobSegment + `/(?:evidence|population)/[^/]+` +
		`|audit/` + blobSegment + `/` + blobSegment + `/population/sample/[^/]+` +
		`|risk/` + blobSegment + `/(?:risk-evidence-attachment|risk-action-plan-completion-attachment)/[^/]+)$`)

// validBlobPrefix permits the folder prefixes used for listing (they end in "/"):
//   - audit/{auditName}/{controlNumber}/evidence/
//   - audit/{auditName}/{controlNumber}/population/
//   - audit/{auditName}/{controlNumber}/population/sample/
var validBlobPrefix = regexp.MustCompile(
	`^audit/` + blobSegment + `/` + blobSegment + `/(?:evidence|population(?:/sample)?)/?$`)

func guardBlobPath(path string) bool {
	return !strings.Contains(path, "..") &&
		!strings.Contains(path, `\`) &&
		validBlobPath.MatchString(path)
}

// guardBlobPrefix validates a folder prefix for listing.
func guardBlobPrefix(prefix string) bool {
	return !strings.Contains(prefix, "..") &&
		!strings.Contains(prefix, `\`) &&
		validBlobPrefix.MatchString(prefix)
}

// maxFileUploadBytes caps a single proxied file upload; the GRC Backend already
// validates size/type before forwarding, this is a defensive backstop.
const maxFileUploadBytes = 25 << 20 // 25 MiB

// FileHandler serves the Azure Blob byte operations (upload/download/list/delete).
// It is the entity's file-storage surface: the GRC Backend proxies evidence and
// risk file bytes through these endpoints — the browser never contacts Azure.
type FileHandler struct {
	storage *storage.Service
}

// NewFileHandler constructs a FileHandler over the given storage service.
func NewFileHandler(s *storage.Service) *FileHandler { return &FileHandler{storage: s} }

// UploadFile handles POST /files — multipart form (blobName + file). Writes the
// bytes to Azure Blob at the given blob name.
func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileUploadBytes)
	if err := r.ParseMultipartForm(maxFileUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		apierror.WriteJSON(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload")
		return
	}
	blobName := strings.TrimSpace(r.FormValue("blobName"))
	if blobName == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "blobName is required")
		return
	}
	if !guardBlobPath(blobName) {
		apierror.WriteJSON(w, http.StatusBadRequest, "blobName must be within an allowed storage layout")
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	if err := validateUploadFileType(filepath.Base(header.Filename), contentType); err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.storage.UploadBlob(r.Context(), blobName, contentType, data); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(domain.UploadFileResponse{BlobName: blobName, Size: len(data)})
}

// DownloadFile handles GET /files?path=<blobName> — streams the blob bytes back
// (fully proxied). Risky types are served as attachments.
func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	blobName := r.URL.Query().Get("path")
	if blobName == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "path is required")
		return
	}
	if !guardBlobPath(blobName) {
		apierror.WriteJSON(w, http.StatusBadRequest, "path must be within an allowed storage layout")
		return
	}
	data, contentType, err := h.storage.ReadBlob(r.Context(), blobName)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(blobName)+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // #nosec G705 -- blob served with nosniff + attachment disposition, browser won't execute it inline
}

// ListFiles handles GET /files/list?prefix=<folderPath> — lists blobs in a folder.
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "prefix is required")
		return
	}
	if !guardBlobPrefix(prefix) {
		apierror.WriteJSON(w, http.StatusBadRequest, "prefix must be within an allowed storage layout")
		return
	}
	items, err := h.storage.ListBlobs(r.Context(), prefix)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]domain.BlobFileItem, 0, len(items))
	for _, it := range items {
		out = append(out, domain.BlobFileItem{
			Name:        it.Name,
			FileName:    it.FileName(),
			ContentType: it.ContentType,
			Size:        it.Size,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(domain.ListFilesResponse{Files: out})
}

// EvidenceFileContentHandler streams one evidence file's bytes by fileId
// (GET /evidence-files/{fileId}/content). fileId-keyed by design: callers (the
// AI validation MCP server) never construct blob paths, so there is no
// path-injection surface — the stored file_path is looked up server-side and
// still passes the blob-path guard as defense in depth.
type EvidenceFileContentHandler struct {
	evidence service.EvidenceService
	storage  *storage.Service
}

// NewEvidenceFileContentHandler constructs an EvidenceFileContentHandler.
func NewEvidenceFileContentHandler(evidence service.EvidenceService, s *storage.Service) *EvidenceFileContentHandler {
	return &EvidenceFileContentHandler{evidence: evidence, storage: s}
}

// GetContent handles GET /evidence-files/{fileId}/content.
func (h *EvidenceFileContentHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	f, err := h.evidence.GetEvidenceFileByID(r.Context(), fileID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if !guardBlobPath(f.FilePath) {
		apierror.WriteJSON(w, http.StatusConflict, "stored file path is outside the allowed evidence layout")
		return
	}
	data, contentType, err := h.storage.ReadBlob(r.Context(), f.FilePath)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if contentType == "" && f.FileType != nil {
		contentType = *f.FileType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-File-Name", filepath.Base(f.FileName))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(f.FileName)+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // #nosec G705 -- blob served with nosniff + attachment disposition, browser won't execute it inline
}

// DeleteFile handles DELETE /files?path=<blobName>.
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	blobName := r.URL.Query().Get("path")
	if blobName == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "path is required")
		return
	}
	if !guardBlobPath(blobName) {
		apierror.WriteJSON(w, http.StatusBadRequest, "path must be within an allowed storage layout")
		return
	}
	if err := h.storage.Delete(r.Context(), blobName); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
