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
	// AuditorEmail is the email of the auditor assigned to this file's owning
	// control (nil if none). Only populated by EvidenceService.GetFileByID's
	// underlying repo call, for the assigned-auditor download gate — omitted
	// from JSON since it's not evidence metadata callers need.
	AuditorEmail *string `json:"-"`
}

// AuditEvidence represents one submission round for a control.
// Each resubmission creates a new row; Files holds all blobs in that round.
type AuditEvidence struct {
	ID         int                  `json:"id"`
	ControlID  int                  `json:"controlId"`
	Status     string               `json:"status"`
	FolderPath string               `json:"folderPath"`
	Files      []*AuditEvidenceFile `json:"files"`
	CreatedBy  string               `json:"createdBy"`
	CreatedAt  time.Time            `json:"createdAt"`
}

// AssignedControlForEvidence is the flat, enriched control record the Compliance
// Entity returns for the authenticated user's team. It is the transport type
// unmarshalled from the entity; the evidence-app handler reshapes it into the
// nested EvidenceAppControl contract (§3.1) with a phase-aware base folder path.
type AssignedControlForEvidence struct {
	AuditID             int     `json:"auditId"`
	AuditName           string  `json:"auditName"`
	Product             string  `json:"product"`
	Framework           string  `json:"framework"`
	PeriodStart         string  `json:"periodStart"`
	PeriodEnd           string  `json:"periodEnd"`
	ControlID           int     `json:"controlId"`
	ControlNumber       string  `json:"controlNumber"`
	Description         string  `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     string  `json:"requirementType"`
	Status              string  `json:"status"`
	DueDate             *string `json:"dueDate"`
}

// EvidenceAppControl is one item in the GET /api/v1/evidence-app/controls response
// (design §3.1). The audit and control are nested, and the backend computes two
// derived fields the portal needs: the control's phase and the phase-aware base
// folder path — both server-side, never trusted from a client.
type EvidenceAppControl struct {
	Audit          EvidenceAppAudit       `json:"audit"`
	Control        EvidenceAppControlInfo `json:"control"`
	BaseFolderPath string                 `json:"baseFolderPath"`
}

// EvidenceAppAudit is the audit context nested in EvidenceAppControl.
type EvidenceAppAudit struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Product     string `json:"product"`
	Framework   string `json:"framework"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
}

// EvidenceAppControlInfo is the control detail nested in EvidenceAppControl.
type EvidenceAppControlInfo struct {
	ID                  int     `json:"id"`
	Number              string  `json:"number"`
	Description         string  `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     string  `json:"requirementType"` // DESIGN | OE
	Status              string  `json:"status"`
	Phase               string  `json:"phase"` // POPULATION | EVIDENCE
	DueDate             *string `json:"dueDate"`
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
// folder re-listing in the flat evidence layout (see design doc §3.3): the
// client accumulates the blobName returned by each upload call and submits the
// exact list of files that make up this round.
type SubmitEvidenceRequest struct {
	Files []EvidenceFileRef `json:"files"`
}

// PopulationSubmitRequest is the body for POST .../population/submit and
// .../population/{controlId}/submit. Unlike evidence, population/sample keep
// the folder-listing contract (their subfolders already fence their files —
// see design doc §3.3), so the client only echoes back the folder path handed
// out by the upload-link endpoint.
type PopulationSubmitRequest struct {
	FolderPath string `json:"folderPath"`
}
