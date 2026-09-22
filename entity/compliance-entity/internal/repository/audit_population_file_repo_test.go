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

// TestDeletePopulationFile_Success covers the common case: the file's round is
// still the latest for its control, so the NOT EXISTS guard finds nothing and
// the row is removed.
func TestDeletePopulationFile_Success(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE aef FROM audit_evidence_file aef")).
		WithArgs(5).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := (&populationRepo{db: repo.db}).DeletePopulationFile(context.Background(), 5); err != nil {
		t.Fatalf("DeletePopulationFile: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDeletePopulationFile_SupersededRound covers the race the NOT EXISTS
// guard exists for: a resubmission created a newer round for the file's
// control between the handler's latest-round check and this delete, so no row
// matches and the query must report not-found rather than delete the file.
func TestDeletePopulationFile_SupersededRound(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectExec(re("DELETE aef FROM audit_evidence_file aef")).
		WithArgs(5).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := (&populationRepo{db: repo.db}).DeletePopulationFile(context.Background(), 5)
	var notFound *apierror.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
