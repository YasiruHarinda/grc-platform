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

package model

import "time"

// AuditEvidenceFile represents a single uploaded file attached to an evidence submission.
type AuditEvidenceFile struct {
	ID         int       `json:"id"`
	EvidenceID int       `json:"evidenceId"`
	FileName   string    `json:"fileName"`
	FilePath   string    `json:"filePath"`
	FileType   *string   `json:"fileType"`
	FileSize   *int64    `json:"fileSize"`
	CreatedBy  string    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
	// ReadURL is the backend proxy download URL (GET /api/v1/evidence/files/{id}/download).
	// Computed at list time (not persisted); nil if the file has no DB id.
	ReadURL *string `json:"readUrl"`
	// AuditorID is the user.id of the auditor assigned to this file's owning
	// control (nil if none). Only populated by EvidenceService.GetFileByID's
	// underlying repo call, for the assigned-auditor download gate — omitted
	// from JSON since it's not evidence metadata callers need.
	AuditorID *int `json:"-"`
	// TeamID is this file's owning control's team_id (nil if none). Only
	// populated by the same repo call as AuditorID, for the team-scoped
	// download gate — omitted from JSON since it's not evidence metadata
	// callers need.
	TeamID *int `json:"-"`
}

// AuditEvidence represents one submission round for a control.
// Each resubmission creates a new row; Files holds all blobs in that round.
type AuditEvidence struct {
	ID         int                  `json:"id"`
	ControlID  int                  `json:"controlId"`
	Status     string               `json:"status"`
	FolderPath string               `json:"folderPath"`
	Files      []*AuditEvidenceFile `json:"files"`
	// Attestation is a written justification for a round with no files. Empty
	// for ordinary rounds.
	Attestation string    `json:"attestation,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// UploadLinkResponse is returned by GET .../evidence/upload-link.
// It gives the agent the folder path to use as a prefix when requesting
// per-file upload URLs and when calling the submit endpoint.
type UploadLinkResponse struct {
	// FolderPath is the Azure Blob prefix for this upload session — a
	// human-readable, deterministic path built from the audit name and control
	// number (e.g. "soc2 asgardeo 2026/CA-01/evidence/"), not a per-session folder.
	FolderPath string    `json:"folderPath"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// FileUploadURLRequest is the body for POST .../evidence/file-url.
// The agent calls this once per file to get a blob-scoped upload URL.
type FileUploadURLRequest struct {
	FileName   string `json:"fileName"`
	FolderPath string `json:"folderPath"`
}

// FileUploadURLResponse is returned by POST .../evidence/file-url.
// UploadURL is a pre-signed PUT URL scoped to exactly one blob.
// Agent: PUT {UploadURL} with body=file bytes and header x-ms-blob-type: BlockBlob.
type FileUploadURLResponse struct {
	UploadURL string    `json:"uploadUrl"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// EvidenceFileRef identifies one already-uploaded evidence blob by its stored
// (sanitized) blob name plus the original, human-readable file name — the
// upload endpoint returns one of these per file (see FileUploadResponse), and
// the client accumulates them to submit an explicit batch.
type EvidenceFileRef struct {
	BlobName string `json:"blobName"`
	FileName string `json:"fileName"`
}

// SubmitEvidenceRequest is the body for POST .../evidence/submit. There is no
// folder re-listing in the flat evidence layout: the client accumulates the
// blobName returned by each upload call and submits the exact list of files
// that make up this round.
//
// Attestation is only honored when Files is empty AND the caller holds
// ManageControls — see EvidenceService.Submit. Anyone else submitting zero
// files still gets the ordinary "no files provided" rejection regardless of
// what they put here.
type SubmitEvidenceRequest struct {
	Files       []EvidenceFileRef `json:"files"`
	Attestation string            `json:"attestation,omitempty"`
}

// PopulationSubmitRequest is the body for POST .../population/submit and
// .../population/{controlId}/submit. Unlike evidence, population/sample keep
// the folder-listing contract (their subfolders already fence their files),
// so the client only echoes back the folder path handed out by the
// upload-link endpoint.
//
// Attestation is a written note standing in for population files — required
// when the folder has none (mirrors SampleSubmitRequest.Note: files, a note,
// or both, at least one required). Unlike SubmitEvidenceRequest.Attestation,
// there is no privilege gate — anyone who can submit population files can use
// this too, matching sample selection's openness rather than evidence's
// ManageControls-only fileless completion.
type PopulationSubmitRequest struct {
	FolderPath  string `json:"folderPath"`
	Attestation string `json:"attestation,omitempty"`
}
