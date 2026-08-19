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

type populationRepo struct{ c *entityclient.Client }

// NewPopulationRepository returns an entity-backed PopulationRepository.
func NewPopulationRepository(c *entityclient.Client) repository.PopulationRepository {
	return &populationRepo{c: c}
}

func (r *populationRepo) AddFile(ctx context.Context, populationID int, fileKind, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error {
	body := map[string]any{
		"fileKind":  fileKind,
		"fileName":  fileName,
		"filePath":  filePath,
		"fileType":  fileType,
		"fileSize":  fileSize,
		"createdBy": createdBy,
	}
	return r.c.Post(ctx, fmt.Sprintf("/populations/%d/files", populationID), body, nil)
}

func (r *populationRepo) UpdateStatus(ctx context.Context, populationID int, status, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy}
	return r.c.Patch(ctx, fmt.Sprintf("/populations/%d", populationID), body, nil)
}

func (r *populationRepo) UpdateStatusWithAttestation(ctx context.Context, populationID int, status, attestation, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy}
	if attestation != "" {
		body["attestation"] = attestation
	}
	return r.c.Patch(ctx, fmt.Sprintf("/populations/%d", populationID), body, nil)
}

// ClearAttestation blanks the round's attestation column. The entity's PATCH
// treats a JSON key's absence as "leave untouched" (see UpdateStatusWithAttestation
// above, which omits the key entirely for that reason) — so clearing requires
// sending the key with an empty string, not omitting it.
func (r *populationRepo) ClearAttestation(ctx context.Context, populationID int, updatedBy string) error {
	body := map[string]any{"attestation": "", "updatedBy": updatedBy}
	return r.c.Patch(ctx, fmt.Sprintf("/populations/%d", populationID), body, nil)
}

func (r *populationRepo) UpdateDetails(ctx context.Context, populationID int, details model.PopulationDetails, updatedBy string) error {
	body := map[string]any{
		"description":     details.Description,
		"dueDate":         details.DueDate,
		"comments":        details.Comments,
		"referenceNumber": details.ReferenceNumber,
		"ownerId":         details.OwnerID,
		"teamId":          details.TeamID,
		"updatedBy":       updatedBy,
	}
	return r.c.Patch(ctx, fmt.Sprintf("/populations/%d", populationID), body, nil)
}

// entPopulationRound mirrors the entity's AuditPopulation JSON.
type entPopulationRound struct {
	ID              int       `json:"id"`
	ControlID       int       `json:"controlId"`
	ReferenceNumber *int      `json:"referenceNumber"`
	Description     *string   `json:"description"`
	Status          string    `json:"status"`
	DueDate         *string   `json:"dueDate"`
	Comments        *string   `json:"comments"`
	Attestation     *string   `json:"attestation"`
	CreatedOn       time.Time `json:"createdOn"`
	UpdatedOn       time.Time `json:"updatedOn"`
}

func (p entPopulationRound) toModel() *model.AuditPopulation {
	return &model.AuditPopulation{
		ID:              p.ID,
		ControlID:       p.ControlID,
		Status:          p.Status,
		ReferenceNumber: p.ReferenceNumber,
		Description:     p.Description,
		DueDate:         p.DueDate,
		Attestation:     p.Attestation,
		Comments:        p.Comments,
		CreatedAt:       p.CreatedOn,
		UpdatedAt:       p.UpdatedOn,
	}
}

func (r *populationRepo) ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditPopulation, error) {
	var rounds []entPopulationRound
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d/populations", auditID, controlID), &rounds); err != nil {
		return nil, err
	}
	out := make([]*model.AuditPopulation, 0, len(rounds))
	for _, p := range rounds {
		out = append(out, p.toModel())
	}
	return out, nil
}

// entPopulationFile mirrors the entity's AuditEvidenceFile JSON (shared table
// with evidence files; populationId/fileKind are set only for population rows).
type entPopulationFile struct {
	ID           int       `json:"id"`
	PopulationID *int      `json:"populationId"`
	FileKind     *string   `json:"fileKind"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	FileType     *string   `json:"fileType"`
	FileSize     *int64    `json:"fileSize"`
	CreatedOn    time.Time `json:"createdOn"`
	TeamID       *int      `json:"teamId"`
}

func (f entPopulationFile) toModel() *model.PopulationFile {
	m := &model.PopulationFile{
		ID:        f.ID,
		FileName:  f.FileName,
		FilePath:  f.FilePath,
		FileType:  f.FileType,
		FileSize:  f.FileSize,
		CreatedAt: f.CreatedOn,
		TeamID:    f.TeamID,
	}
	if f.PopulationID != nil {
		m.PopulationID = *f.PopulationID
	}
	if f.FileKind != nil {
		m.FileKind = *f.FileKind
	}
	return m
}

func (r *populationRepo) ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error) {
	var files []entPopulationFile
	if err := r.c.Get(ctx, fmt.Sprintf("/populations/%d/files", populationID), &files); err != nil {
		return nil, err
	}
	out := make([]*model.PopulationFile, 0, len(files))
	for _, f := range files {
		out = append(out, f.toModel())
	}
	return out, nil
}

func (r *populationRepo) GetFileByID(ctx context.Context, fileID int) (*model.PopulationFile, error) {
	var f entPopulationFile
	if err := r.c.Get(ctx, fmt.Sprintf("/population-files/%d", fileID), &f); err != nil {
		return nil, err
	}
	return f.toModel(), nil
}

func (r *populationRepo) DeleteFile(ctx context.Context, fileID int) error {
	return r.c.Delete(ctx, fmt.Sprintf("/population-files/%d", fileID))
}
