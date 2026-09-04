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
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// channelEvidencePortal tags audit-trail entries submitted through the
// Evidence Portal M2M ingress.
const channelEvidencePortal = "evidence-portal-api"

// PortalEvidenceFile is one file in a portal evidence submission.
type PortalEvidenceFile struct {
	FileName    string
	ContentType string
	Data        []byte
}

// SubmitPortalEvidence uploads each portal file to the control's evidence
// folder, then hands the resulting refs to the same finalizeEvidenceSubmission
// path the web-app submit route uses — so the round is recorded, the status
// advanced, and notify/trail/AI fired exactly once and identically to a
// web-app submission. All files land in ONE evidence round. actorUUID is the
// resolved submitter; clientID is recorded as the trail issuer.
func (d *Deps) SubmitPortalEvidence(ctx context.Context, auditID, controlID int, files []PortalEvidenceFile, actorUUID, clientID string) (*model.AuditEvidence, error) {
	eh := newEvidenceHandler(d)

	link, err := eh.svc.GetUploadLink(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	refs := make([]model.EvidenceFileRef, 0, len(files))
	for _, f := range files {
		blobName, err := eh.svc.UploadFile(ctx, link.FolderPath, f.FileName, f.ContentType, f.Data)
		if err != nil {
			return nil, err
		}
		refs = append(refs, model.EvidenceFileRef{BlobName: blobName, FileName: f.FileName})
	}
	return eh.finalizeEvidenceSubmission(ctx, auditID, controlID, refs, "", false, actorUUID, channelEvidencePortal, clientID)
}
