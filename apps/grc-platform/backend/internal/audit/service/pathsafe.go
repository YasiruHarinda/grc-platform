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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/blobpath"
)

// The sanitize/build logic itself now lives in internal/shared/blobpath so the
// Risk module's evidence service can reuse the exact same rules (see ADR: one
// proven path-safety implementation, not two). These wrappers keep every
// existing call site in this package unchanged.

// SanitizeSegment is the exported form of sanitizeSegment, for callers outside
// this package that need to display or precompute a path segment the same way
// the evidence service derives it (e.g. the Evidence Portal's base folder path).
func SanitizeSegment(s string) string {
	return blobpath.SanitizeSegment(s)
}

// sanitizeSegment collapses s to the path-safe charset [A-Za-z0-9 _-], trimming
// leading/trailing whitespace. It is used for audit names and control numbers —
// both become literal Azure Blob path segments, so every other character
// (including "/" and "..") collapses to "-" rather than being rejected outright.
func sanitizeSegment(s string) string {
	return blobpath.SanitizeSegment(s)
}

// sanitizeFileName reduces name to its basename (killing any directory
// component, "..", "/", or "\\" — this is what closes path traversal and folder
// forking for uploaded file names), then splits it into a sanitized stem and its
// original extension.
func sanitizeFileName(name string) (stem, ext string) {
	return blobpath.SanitizeFileName(name)
}

// buildBlobName joins a sanitized stem, a short UUID, and the original
// extension into the final blob file name: "stem-<uuid>.ext" (or "stem-<uuid>"
// when the file had no extension).
func buildBlobName(stem, ext string) string {
	return blobpath.BuildBlobName(stem, ext)
}
