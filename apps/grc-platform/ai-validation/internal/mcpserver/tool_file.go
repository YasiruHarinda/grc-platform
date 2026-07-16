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

package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/mcpserver/extract"
)

// maxPDFPages mirrors the Anthropic document page limit.
const maxPDFPages = 100

// getEvidenceFileArgs is the input of get_evidence_file.
type getEvidenceFileArgs struct {
	FileID int `json:"fileId"`
}

// getEvidenceFile resolves a fileId, re-checks it belongs to the session's
// evidence chain (defense in depth — no wildcard reads), streams the bytes
// from the entity (fully proxied; no Azure credential anywhere here), and
// converts them into MCP content the agent bridge can hand to the model.
func (s *Server) getEvidenceFile(ctx context.Context, sess *Session, raw json.RawMessage) (*mcp.CallToolResult, error) {
	var args getEvidenceFileArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.FileID <= 0 {
		return toolError("fileId must be a positive integer"), nil
	}

	// File record → owning evidence.
	var f entEvidenceFile
	if err := s.entity.Get(ctx, fmt.Sprintf("/evidence-files/%d", args.FileID), &f); err != nil {
		return toolError(fmt.Sprintf("file %d not found", args.FileID)), nil
	}
	if f.EvidenceID == nil {
		return toolError(fmt.Sprintf("file %d is not attached to an evidence submission", args.FileID)), nil
	}

	// Scope check: the file's evidence must be the scoped submission or a
	// prior submission of the same control.
	if *f.EvidenceID != sess.Scope.EvidenceID {
		var owner entEvidence
		if err := s.entity.Get(ctx, fmt.Sprintf("/evidence/%d", *f.EvidenceID), &owner); err != nil {
			return nil, fmt.Errorf("could not verify file ownership: %w", err)
		}
		if owner.ControlID != sess.Scope.ControlID {
			s.log.Warn("mcp scope violation", "tool", "get_evidence_file",
				"fileId", args.FileID, "fileEvidenceId", *f.EvidenceID,
				"scopeControlId", sess.Scope.ControlID, "fileControlId", owner.ControlID)
			return toolError(fmt.Sprintf("file %d is outside this validation's scope", args.FileID)), nil
		}
	}

	// Size guard before fetching bytes.
	if f.FileSize != nil && *f.FileSize > s.cfg.MaxFileBytesToLLM {
		return toolError(fmt.Sprintf("file %q is too large for AI review (%d bytes, limit %d)",
			f.FileName, *f.FileSize, s.cfg.MaxFileBytesToLLM)), nil
	}

	data, contentType, _, err := s.entity.GetBytes(ctx,
		fmt.Sprintf("/evidence-files/%d/content", args.FileID), s.cfg.MaxFileBytesToLLM)
	if err != nil {
		return nil, fmt.Errorf("could not fetch file content: %w", err)
	}
	if f.FileType != nil && *f.FileType != "" {
		contentType = *f.FileType // DB record wins over transport header
	}

	header := fmt.Sprintf("file %d: %s (%s, %d bytes)", f.ID, f.FileName, contentType, len(data))

	switch kind(contentType, f.FileName) {
	case "pdf":
		if n := countPDFPages(data); n > maxPDFPages {
			return toolError(fmt.Sprintf("file %q has ~%d pages — over the %d page limit for AI review", f.FileName, n, maxPDFPages)), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: header},
			&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{
				URI:      fmt.Sprintf("evidence://file/%d", f.ID),
				MIMEType: "application/pdf",
				Blob:     data,
			}},
		}}, nil
	case "image":
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: header},
			&mcp.ImageContent{Data: data, MIMEType: normalizedImageMime(contentType, f.FileName)},
		}}, nil
	case "text":
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: header + "\n" + string(data)},
		}}, nil
	case "xlsx":
		text, err := extract.XLSXToCSV(data)
		if err != nil {
			return toolError(fmt.Sprintf("could not read spreadsheet %q: unsupported or corrupt file", f.FileName)), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: header + "\n" + text},
		}}, nil
	default:
		return toolError(fmt.Sprintf("unsupported file type for AI review: %s", f.FileName)), nil
	}
}

// kind classifies a file by MIME type with a filename-extension fallback.
func kind(contentType, fileName string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	ext := strings.ToLower(filepath.Ext(fileName))
	switch {
	case ct == "application/pdf" || ext == ".pdf":
		return "pdf"
	case strings.HasPrefix(ct, "image/png") || strings.HasPrefix(ct, "image/jpeg") ||
		strings.HasPrefix(ct, "image/gif") || strings.HasPrefix(ct, "image/webp"):
		return "image"
	case ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp":
		return "image"
	case ct == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		ext == ".xlsx" || ext == ".xlsm":
		return "xlsx"
	case strings.HasPrefix(ct, "text/") || ct == "application/json" ||
		ext == ".txt" || ext == ".csv" || ext == ".json" || ext == ".md" || ext == ".log":
		return "text"
	default:
		return "unsupported"
	}
}

// normalizedImageMime maps to one of the image MIME types Anthropic accepts.
func normalizedImageMime(contentType, fileName string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return ct
	}
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

var pdfPagePattern = regexp.MustCompile(`/Type\s*/Page[^s]`)

// countPDFPages approximates the page count by counting page objects. Good
// enough for a guard rail; the Anthropic API enforces the hard limit anyway.
func countPDFPages(data []byte) int {
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return 0
	}
	return len(pdfPagePattern.FindAll(data, -1))
}
