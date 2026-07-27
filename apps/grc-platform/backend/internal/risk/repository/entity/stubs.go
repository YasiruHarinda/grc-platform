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

// Unimplemented repositories, kept so the risk module can stop depending on a
// *sql.DB before these features exist.
//
// Risk evidence and notifications are still scaffolding: every method below
// returns errNotImplemented, neither is routed, and their tables are empty.
// Their MySQL counterparts were identical — stubs holding a database handle
// they never used. Holding an entity client they never use instead changes
// nothing at runtime.
//
// Escalation (escalation.go) and action plans (action_plan.go) have moved out
// of this file — they're real now, backed by the Compliance Entity. Notes for
// whoever picks up what's left:
//
//   - Evidence: the entity's delete is `DELETE FROM risk_evidence WHERE id = ?`
//     with no risk_id scoping, so any file id can be deleted regardless of which
//     risk owns it. The MySQL TODO called for `AND risk_id = ?`. Fix that first.
//   - Notifications: the entity now has full CRUD (POST /notifications,
//     GET /notifications?recipientId=, PATCH /notifications/{id}/read) —
//     written for the escalation feature's own use (internal/job and the
//     action-plan-completion cascade write to it directly, server-side). This
//     backend-facing repository is still unimplemented since no route here
//     calls it yet; wiring it up is what's left, not building the feature.

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// ── Risk evidence ────────────────────────────────────────────────────────────

type riskEvidenceRepository struct{ c *entityclient.Client }

// NewRiskEvidenceRepository returns an unimplemented risk evidence repository.
func NewRiskEvidenceRepository(c *entityclient.Client) repository.RiskEvidenceRepository {
	return &riskEvidenceRepository{c: c}
}

func (r *riskEvidenceRepository) List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error) {
	return nil, errNotImplemented
}

func (r *riskEvidenceRepository) Create(ctx context.Context, riskID int, fileName, filePath, note, evidenceType, createdBy string) (*model.RiskEvidence, error) {
	return nil, errNotImplemented
}

func (r *riskEvidenceRepository) Delete(ctx context.Context, riskID, evidenceID int, byUserID string) error {
	return errNotImplemented
}

// ── Notifications ────────────────────────────────────────────────────────────

type notificationRepository struct{ c *entityclient.Client }

// NewNotificationRepository returns an unimplemented notification repository.
func NewNotificationRepository(c *entityclient.Client) repository.NotificationRepository {
	return &notificationRepository{c: c}
}

func (r *notificationRepository) List(ctx context.Context, recipientID int) ([]*model.Notification, error) {
	return nil, errNotImplemented
}

func (r *notificationRepository) MarkRead(ctx context.Context, id, recipientID int) error {
	return errNotImplemented
}
