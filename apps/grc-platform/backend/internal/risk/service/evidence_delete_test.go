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
	"context"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// The point of this file: handleDeleteRiskEvidence's ownership check
// (`ev.CreatedBy != actor`) can't be exercised end-to-end in local dev — the
// admin-override branch (canOverrideAssigneeIn) is AllowAll-bypassed there, so
// every caller succeeds regardless of the comparison. isAdmin is an explicit
// parameter on evidenceService.Delete, so a unit test can hold it at false and
// actually exercise the comparison — which is the only way to verify it at all
// without a full staging login.

// stubEvidenceRepo returns a fixed evidence row and records whether Delete was
// called on it.
type stubEvidenceRepo struct {
	repository.RiskEvidenceRepository // embedded: panics on any unstubbed method
	ev                                *model.RiskEvidence
	deleted                           bool
}

func (s *stubEvidenceRepo) GetByID(_ context.Context, _ int) (*model.RiskEvidence, error) {
	return s.ev, nil
}
func (s *stubEvidenceRepo) Delete(_ context.Context, _, _ int) error {
	s.deleted = true
	return nil
}

// stubRiskRepoForEvidence returns a risk that has never been approved, so
// ActionPlanAttachment's owner-approval lock never engages — isolating the
// ownership comparison from the other two lock rules in Delete.
type stubRiskRepoForEvidence struct {
	repository.RiskRepository
}

func (s *stubRiskRepoForEvidence) GetByID(_ context.Context, _ int) (*model.RiskDetail, error) {
	return &model.RiskDetail{OwnerFirstApprovedAt: nil}, nil
}

func TestEvidenceDelete_OwnershipCheck(t *testing.T) {
	const uploaderUUID = "885aeeb0-2086-4ca4-83c9-b2a62b299967"
	const otherUUID = "d4cd6ec1-4b52-49a1-9e83-99a55c2b144e"

	ev := &model.RiskEvidence{
		ID: 1, RiskID: 19, EvidenceType: EvidenceTypeActionPlanAttachment, CreatedBy: uploaderUUID,
	}

	t.Run("non-owner, no override, is forbidden", func(t *testing.T) {
		repo := &stubEvidenceRepo{ev: ev}
		s := NewEvidenceService(repo, &stubRiskRepoForEvidence{}, nil, nil)

		err := s.Delete(context.Background(), 19, 1, otherUUID, false)
		if err == nil {
			t.Fatal("SECURITY: a non-owner with no override deleted someone else's evidence")
		}
		if repo.deleted {
			t.Error("Delete was called on the repo despite the ownership check failing")
		}
	})

	t.Run("owner, no override, succeeds", func(t *testing.T) {
		repo := &stubEvidenceRepo{ev: ev}
		s := NewEvidenceService(repo, &stubRiskRepoForEvidence{}, nil, nil)

		if err := s.Delete(context.Background(), 19, 1, uploaderUUID, false); err != nil {
			t.Fatalf("the original uploader must be able to delete their own evidence: %v", err)
		}
		if !repo.deleted {
			t.Error("Delete was not called on the repo")
		}
	})

	t.Run("non-owner WITH override, succeeds", func(t *testing.T) {
		repo := &stubEvidenceRepo{ev: ev}
		s := NewEvidenceService(repo, &stubRiskRepoForEvidence{}, nil, nil)

		if err := s.Delete(context.Background(), 19, 1, otherUUID, true); err != nil {
			t.Fatalf("the admin override must bypass ownership: %v", err)
		}
		if !repo.deleted {
			t.Error("Delete was not called on the repo")
		}
	})

	// The case this whole test file exists for: a row backfilled from an old
	// email created_by to the uploader's uuid must still match that uploader,
	// exactly like a row created natively as a uuid.
	t.Run("backfilled row matches its uploader post-migration", func(t *testing.T) {
		backfilled := &model.RiskEvidence{
			ID: 2, RiskID: 19, EvidenceType: EvidenceTypeActionPlanAttachment, CreatedBy: uploaderUUID,
		}
		repo := &stubEvidenceRepo{ev: backfilled}
		s := NewEvidenceService(repo, &stubRiskRepoForEvidence{}, nil, nil)

		if err := s.Delete(context.Background(), 19, 2, uploaderUUID, false); err != nil {
			t.Fatalf("a backfilled row must still match its original uploader: %v", err)
		}
	})
}
