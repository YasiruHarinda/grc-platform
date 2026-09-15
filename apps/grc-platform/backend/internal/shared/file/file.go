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

// Package file is an HTTP client to the Compliance Entity's file (Azure Blob)
// endpoints. The GRC Backend no longer talks to Azure directly — the Compliance
// Entity holds the Azure account key and performs all reads/writes. The backend
// validates uploads and proxies the bytes to/from the entity.
package file

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
)

// Service is a client to the Compliance Entity's /files endpoints.
type Service struct {
	entityBaseURL string
	client        *http.Client
}

// NewService creates a file Service pointed at the Compliance Entity base URL
// (e.g. http://compliance-entity:8080).
func NewService(entityBaseURL string) *Service {
	return &Service{
		entityBaseURL: strings.TrimRight(entityBaseURL, "/"),
		client:        &http.Client{Timeout: 60 * time.Second},
	}
}

// BlobItem describes a single blob returned by ListBlobs.
type BlobItem struct {
	// Name is the full blob path within the container.
	Name        string
	ContentType string
	Size        int64
}

// FileName returns just the file name portion of the blob path.
func (b BlobItem) FileName() string {
	parts := strings.Split(b.Name, "/")
	return parts[len(parts)-1]
}

func baseName(blobName string) string {
	parts := strings.Split(blobName, "/")
	return parts[len(parts)-1]
}

// UploadBlob forwards the file bytes to the Compliance Entity, which writes them
// to Azure at blobName. contentType is preserved on the stored blob.
func (s *Service) UploadBlob(ctx context.Context, blobName, contentType string, data []byte) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("blobName", blobName); err != nil {
		return fmt.Errorf("file: build upload: %w", err)
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, baseName(blobName)))
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	if err != nil {
		return fmt.Errorf("file: build upload part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("file: write upload part: %w", err)
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("file: close upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.entityBaseURL+"/files", &buf)
	if err != nil {
		return fmt.Errorf("file: upload request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("file: upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		// Pass the entity's 400 reason (e.g. a blocked file type) to the caller instead of a 500.
		var e struct{ Message string `json:"message"` }
		if resp.StatusCode == http.StatusBadRequest && json.Unmarshal(body, &e) == nil && e.Message != "" {
			slog.WarnContext(ctx, "upload rejected by entity", "blobName", blobName, "reason", e.Message)
			return &apierror.Error{StatusCode: http.StatusBadRequest, Body: e.Message}
		}
		return fmt.Errorf("file: upload: entity returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ReadBlob fetches a blob's bytes and content type from the entity (proxied read).
func (s *Service) ReadBlob(ctx context.Context, blobName string) (data []byte, contentType string, err error) {
	u := s.entityBaseURL + "/files?path=" + url.QueryEscape(blobName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("file: read request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("file: read: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("file: read: entity returned %d: %s", resp.StatusCode, string(body))
	}
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("file: read body: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// ListBlobs lists blobs under the given folder prefix via the entity.
func (s *Service) ListBlobs(ctx context.Context, prefix string) ([]BlobItem, error) {
	u := s.entityBaseURL + "/files/list?prefix=" + url.QueryEscape(prefix)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("file: list request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("file: list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("file: list: entity returned %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Files []struct {
			Name        string `json:"name"`
			ContentType string `json:"contentType"`
			Size        int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("file: list decode: %w", err)
	}
	items := make([]BlobItem, 0, len(out.Files))
	for _, f := range out.Files {
		items = append(items, BlobItem{Name: f.Name, ContentType: f.ContentType, Size: f.Size})
	}
	return items, nil
}

// Delete removes the blob via the entity.
func (s *Service) Delete(ctx context.Context, blobName string) error {
	u := s.entityBaseURL + "/files?path=" + url.QueryEscape(blobName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("file: delete request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("file: delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("file: delete: entity returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
