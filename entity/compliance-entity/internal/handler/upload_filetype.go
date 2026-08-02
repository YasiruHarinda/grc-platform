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
// apps/grc-platform/backend/internal/audit/handler/upload_filetype.go — the two
// services are separate Go modules, so nothing enforces this automatically.

// blockedUploadExtensions rejects file extensions that a browser can execute as
// active content. The GRC Backend already rejects these before forwarding; this
// is the same defensive backstop maxFileUploadBytes provides for size.
var blockedUploadExtensions = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true,
	".svg": true,
	".xml": true, ".xsl": true, ".xslt": true,
	".js": true, ".mjs": true, ".jsx": true,
}

// blockedUploadContentTypePrefixes rejects the sniffed/declared content type in
// case the extension was disguised. Matched by prefix so a charset suffix (e.g.
// "text/html; charset=utf-8") still hits.
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
// browser would execute (HTML, SVG, XML, JS) if ever opened inline instead of
// downloaded.
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
