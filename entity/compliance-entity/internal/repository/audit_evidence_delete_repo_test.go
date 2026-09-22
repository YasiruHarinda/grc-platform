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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
)

// TestDeleteEvidence_Success covers the common case: evidenceID is still the
// latest round for its control, so the self-join guard finds no newer round
// and the row is removed.
func TestDeleteEvidence_Success(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE e1 FROM audit_evidence e1")).
		WithArgs(5).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := (&evidenceRepo{db: repo.db}).DeleteEvidence(context.Background(), 5); err != nil {
		t.Fatalf("DeleteEvidence: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDeleteEvidence_SupersededRound covers the race the join guard exists
// for: a resubmission created a newer round for the same control between the
// caller's latest-round check and this delete, so no row matches, the round
// still exists, and the error must be a 409 conflict, not a 404.
func TestDeleteEvidence_SupersededRound(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE e1 FROM audit_evidence e1")).
		WithArgs(5).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(re("SELECT 1 FROM audit_evidence WHERE id = ?")).
		WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	err := (&evidenceRepo{db: repo.db}).DeleteEvidence(context.Background(), 5)
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDeleteEvidence_NotFound covers a genuinely absent round: the delete
// matches no rows and the round doesn't exist at all, so the error stays 404.
func TestDeleteEvidence_NotFound(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE e1 FROM audit_evidence e1")).
		WithArgs(5).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(re("SELECT 1 FROM audit_evidence WHERE id = ?")).
		WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"1"}))

	err := (&evidenceRepo{db: repo.db}).DeleteEvidence(context.Background(), 5)
	var notFound *apierror.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDeleteEvidenceFile_Success mirrors TestDeleteEvidence_Success for the
// file-level delete.
func TestDeleteEvidenceFile_Success(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE aef FROM audit_evidence_file aef")).
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := (&evidenceRepo{db: repo.db}).DeleteEvidenceFile(context.Background(), 9); err != nil {
		t.Fatalf("DeleteEvidenceFile: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDeleteEvidenceFile_SupersededRound mirrors
// TestDeleteEvidence_SupersededRound for the file-level delete.
func TestDeleteEvidenceFile_SupersededRound(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE aef FROM audit_evidence_file aef")).
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(re("SELECT 1 FROM audit_evidence_file WHERE id = ? AND evidence_id IS NOT NULL")).
		WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	err := (&evidenceRepo{db: repo.db}).DeleteEvidenceFile(context.Background(), 9)
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDeleteEvidenceFile_NotFound mirrors TestDeleteEvidence_NotFound for the
// file-level delete.
func TestDeleteEvidenceFile_NotFound(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE aef FROM audit_evidence_file aef")).
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(re("SELECT 1 FROM audit_evidence_file WHERE id = ? AND evidence_id IS NOT NULL")).
		WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"1"}))

	err := (&evidenceRepo{db: repo.db}).DeleteEvidenceFile(context.Background(), 9)
	var notFound *apierror.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
