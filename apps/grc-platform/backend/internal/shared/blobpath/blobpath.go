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

// Package blobpath builds and sanitizes Azure Blob path segments and file
// names shared by every module that stores evidence (Audit Hub, Risk). Kept
// separate from internal/shared/file (the Azure HTTP client) since this is
// pure string logic with no network dependency.
package blobpath

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// SanitizeSegment collapses s to the path-safe charset [A-Za-z0-9 _-], trimming
// leading/trailing whitespace. Used for any human-readable name that becomes a
// literal Azure Blob path segment (audit names, control numbers, risk codes) —
// every other character (including "/" and "..") collapses to "-" rather than
// being rejected outright.
func SanitizeSegment(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isPathSafeRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.TrimSpace(b.String())
}

func isPathSafeRune(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return true
	case r == ' ', r == '_', r == '-':
		return true
	}
	return false
}

// SanitizeFileName reduces name to its basename (killing any directory
// component, "..", "/", or "\\" — this is what closes path traversal and folder
// forking for uploaded file names), then splits it into a sanitized stem and its
// original extension.
func SanitizeFileName(name string) (stem, ext string) {
	base := filepath.Base(name)
	ext = filepath.Ext(base)
	stem = strings.TrimSuffix(base, ext)
	stem = SanitizeSegment(stem)
	if stem == "" {
		stem = "file"
	}
	ext = SanitizeSegment(strings.TrimPrefix(ext, "."))
	return stem, ext
}

// shortUUID returns 32 random hex characters (128 bits), used as a
// collision-proof suffix on every uploaded blob name so two files sharing a
// human name never overwrite each other.
func shortUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never errors on the standard reader
	return hex.EncodeToString(b)
}

// BuildBlobName joins a sanitized stem, a short UUID, and the original
// extension into the final blob file name: "stem-<uuid>.ext" (or "stem-<uuid>"
// when the file had no extension).
func BuildBlobName(stem, ext string) string {
	name := stem + "-" + shortUUID()
	if ext != "" {
		name += "." + ext
	}
	return name
}
