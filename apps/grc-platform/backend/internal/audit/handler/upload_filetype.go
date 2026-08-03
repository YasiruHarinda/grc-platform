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
	"fmt"
	"path/filepath"
	"strings"
)

// Keep this file's blocklist in sync with its twin:
// entity/compliance-entity/internal/handler/upload_filetype.go — the two
// services are separate Go modules, so nothing enforces this automatically.

// blockedUploadExtensions rejects file extensions that a browser can execute as
// active content. Evidence/population/sample files are business documents
// (office formats, PDFs, images, archives, text) — none of them legitimately
// need these extensions, so they are blocked outright rather than allow-listed.
var blockedUploadExtensions = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true,
	".svg": true,
	".xml": true, ".xsl": true, ".xslt": true,
	".js": true, ".mjs": true, ".jsx": true,
}

// blockedUploadContentTypePrefixes rejects the sniffed/declared content type in
// case the extension was disguised (e.g. an ".pdf" upload whose bytes are
// actually HTML). Matched by prefix so "text/html; charset=utf-8" still hits.
var blockedUploadContentTypePrefixes = []string{
	"text/html",
	"application/xhtml+xml",
	"image/svg+xml",
	"text/xml",
	"application/xml",
	"application/javascript",
	"text/javascript",
	"application/x-javascript",
}

// validateUploadFileType rejects file types capable of carrying script that a
// browser would execute (HTML, SVG, XML, JS). This guards a downstream gap: a
// reviewer's "view" action fetches the file as a blob and opens it in a new
// tab, which renders by content type regardless of the Content-Disposition
// header the download endpoints set. Blocking these types at upload time is
// the actual control — closing the gap at the download layer alone would still
// leave the file rendered inline in the browser's blob-URL viewer.
func validateUploadFileType(fileName, contentType string) error {
	ext := strings.ToLower(filepath.Ext(fileName))
	if blockedUploadExtensions[ext] {
		return fmt.Errorf("file type %q is not allowed", ext)
	}
	base, _, _ := strings.Cut(contentType, ";")
	base = strings.ToLower(strings.TrimSpace(base))
	for _, prefix := range blockedUploadContentTypePrefixes {
		if strings.HasPrefix(base, prefix) {
			return fmt.Errorf("file type %q is not allowed", base)
		}
	}
	return nil
}
