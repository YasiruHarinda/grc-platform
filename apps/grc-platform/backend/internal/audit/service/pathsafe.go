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
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// SanitizeSegment is the exported form of sanitizeSegment, for callers outside
// this package that need to display or precompute a path segment the same way
// the evidence service derives it (e.g. the Evidence Portal's base folder path).
func SanitizeSegment(s string) string {
	return sanitizeSegment(s)
}

// sanitizeSegment collapses s to the path-safe charset [A-Za-z0-9 _-], trimming
// leading/trailing whitespace. It is used for audit names and control numbers —
// both become literal Azure Blob path segments, so every other character
// (including "/" and "..") collapses to "-" rather than being rejected outright.
func sanitizeSegment(s string) string {
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

// sanitizeFileName reduces name to its basename (killing any directory
// component, "..", "/", or "\\" — this is what closes path traversal and folder
// forking for uploaded file names), then splits it into a sanitized stem and its
// original extension.
func sanitizeFileName(name string) (stem, ext string) {
	base := filepath.Base(name)
	ext = filepath.Ext(base)
	stem = strings.TrimSuffix(base, ext)
	stem = sanitizeSegment(stem)
	if stem == "" {
		stem = "file"
	}
	ext = sanitizeSegment(strings.TrimPrefix(ext, "."))
	return stem, ext
}

// shortUUID returns 8 random hex characters, used as a collision-proof suffix on
// every uploaded blob name so two files sharing a human name never overwrite
// each other.
func shortUUID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b) // crypto/rand.Read never errors on the standard reader
	return hex.EncodeToString(b)
}

// buildBlobName joins a sanitized stem, a short UUID, and the original
// extension into the final blob file name: "stem-<uuid>.ext" (or "stem-<uuid>"
// when the file had no extension).
func buildBlobName(stem, ext string) string {
	name := stem + "-" + shortUUID()
	if ext != "" {
		name += "." + ext
	}
	return name
}
