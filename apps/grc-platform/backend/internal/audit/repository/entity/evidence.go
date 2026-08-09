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

package entity

import (
	"context"
	"fmt"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type evidenceRepo struct{ c *entityclient.Client }

// NewEvidenceRepository returns an entity-backed EvidenceRepository.
func NewEvidenceRepository(c *entityclient.Client) repository.EvidenceRepository {
	return &evidenceRepo{c: c}
}

// entEvidence mirrors the entity's AuditEvidence JSON (createdOn / *createdBy)
// which differs from the backend model (createdAt / createdBy string).
type entEvidence struct {
	ID         int       `json:"id"`
	ControlID  int       `json:"controlId"`
	Status     string    `json:"status"`
	FolderPath *string   `json:"folderPath"`
	CreatedBy  *string   `json:"createdBy"`
	CreatedOn  time.Time `json:"createdOn"`
}

// entFile mirrors the entity's AuditEvidenceFile JSON.
type entFile struct {
	ID           int       `json:"id"`
	EvidenceID   *int      `json:"evidenceId"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	FileType     *string   `json:"fileType"`
	FileSize     *int64    `json:"fileSize"`
	CreatedBy    *string   `json:"createdBy"`
	CreatedOn    time.Time `json:"createdOn"`
	AuditorEmail *string   `json:"auditorEmail"`
}

func (f entFile) toModel() *model.AuditEvidenceFile {
	m := &model.AuditEvidenceFile{
		ID:           f.ID,
		FileName:     f.FileName,
		FilePath:     f.FilePath,
		FileType:     f.FileType,
		FileSize:     f.FileSize,
		CreatedAt:    f.CreatedOn,
		AuditorEmail: f.AuditorEmail,
	}
	if f.EvidenceID != nil {
		m.EvidenceID = *f.EvidenceID
	}
	if f.CreatedBy != nil {
		m.CreatedBy = *f.CreatedBy
	}
	return m
}

func (r *evidenceRepo) Create(ctx context.Context, auditID, controlID int, folderPath, createdBy string) (int, error) {
	body := map[string]any{"folderPath": folderPath, "createdBy": createdBy}
	var ev entEvidence
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/%d/evidence", auditID, controlID), body, &ev); err != nil {
		return 0, err
	}
	return ev.ID, nil
}

func (r *evidenceRepo) AddFile(ctx context.Context, evidenceID int, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error {
	body := map[string]any{
		"fileName":  fileName,
		"filePath":  filePath,
		"fileType":  fileType,
		"fileSize":  fileSize,
		"createdBy": createdBy,
	}
	return r.c.Post(ctx, fmt.Sprintf("/evidence/%d/files", evidenceID), body, nil)
}

func (r *evidenceRepo) ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error) {
	var resp struct {
		Evidence []entEvidence `json:"evidence"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d/evidence", auditID, controlID), &resp); err != nil {
		return nil, err
	}

	out := make([]*model.AuditEvidence, 0, len(resp.Evidence))
	for _, ev := range resp.Evidence {
		e := &model.AuditEvidence{
			ID:        ev.ID,
			ControlID: ev.ControlID,
			Status:    ev.Status,
			CreatedAt: ev.CreatedOn,
		}
		if ev.FolderPath != nil {
			e.FolderPath = *ev.FolderPath
		}
		if ev.CreatedBy != nil {
			e.CreatedBy = *ev.CreatedBy
		}
		// The entity's evidence list does not embed files — fetch them per submission.
		var files struct {
			Files []entFile `json:"files"`
		}
		if err := r.c.Get(ctx, fmt.Sprintf("/evidence/%d/files", ev.ID), &files); err != nil {
			return nil, err
		}
		e.Files = make([]*model.AuditEvidenceFile, 0, len(files.Files))
		for _, f := range files.Files {
			e.Files = append(e.Files, f.toModel())
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *evidenceRepo) DeleteEvidence(ctx context.Context, evidenceID int) error {
	return r.c.Delete(ctx, fmt.Sprintf("/evidence/%d", evidenceID))
}

func (r *evidenceRepo) GetFileByID(ctx context.Context, fileID int) (*model.AuditEvidenceFile, error) {
	var f entFile
	if err := r.c.Get(ctx, fmt.Sprintf("/evidence-files/%d", fileID), &f); err != nil {
		return nil, err
	}
	return f.toModel(), nil
}

func (r *evidenceRepo) DeleteFile(ctx context.Context, fileID int) error {
	return r.c.Delete(ctx, fmt.Sprintf("/evidence-files/%d", fileID))
}

func (r *evidenceRepo) UpdateStatus(ctx context.Context, evidenceID int, status, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy}
	return r.c.Patch(ctx, fmt.Sprintf("/evidence/%d", evidenceID), body, nil)
}
