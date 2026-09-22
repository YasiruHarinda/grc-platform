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
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

func intPtr(v int) *int { return &v }

// populationRow builds one row shaped like scanPopulationRow's 12-column Scan
// call (see GetPopulationByID's select list), with every optional column left
// NULL.
func populationRow(id, controlID int) *sqlmock.Rows {
	cols := []string{
		"id", "control_id",
		"owner_id", "team_id", "reference_number", "description",
		"status", "due_date", "comments", "attestation",
		"created_at", "updated_at",
	}
	now := time.Now()
	return sqlmock.NewRows(cols).AddRow(
		id, controlID,
		nil, nil, nil, nil,
		"PENDING", nil, nil, nil,
		now, now,
	)
}

// TestCreatePopulation_Replacement_Success covers the common resubmission
// case: previousRoundID is still the latest locked round, so the insert goes
// ahead within the transaction.
func TestCreatePopulation_Replacement_Success(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT 1 FROM audit_control WHERE id = ? AND audit_id = ?")).
		WithArgs(7, 2).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectQuery(re("SELECT id FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectExec(re("INSERT INTO audit_population")).
		WillReturnResult(sqlmock.NewResult(4, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(re("SELECT id, control_id, owner_id, team_id, reference_number, description")).
		WithArgs(4).
		WillReturnRows(populationRow(4, 7))

	req := domain.CreatePopulationRequest{CreatedBy: "actor", PreviousRoundID: intPtr(3)}
	got, err := (&populationRepo{db: repo.db}).CreatePopulation(context.Background(), 2, 7, req)
	if err != nil {
		t.Fatalf("CreatePopulation: %v", err)
	}
	if got.ID != 4 {
		t.Errorf("ID = %d, want 4", got.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestCreatePopulation_Replacement_ConcurrentResubmission covers the race the
// lock exists for: another request already inserted round 4 for the same
// control by the time this one's FOR UPDATE query runs, so the locked latest
// id (4) no longer matches previousRoundID (3) and the insert must not run.
func TestCreatePopulation_Replacement_ConcurrentResubmission(t *testing.T) {
	repo, mock := newControlRepoMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT 1 FROM audit_control WHERE id = ? AND audit_id = ?")).
		WithArgs(7, 2).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectQuery(re("SELECT id FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3).AddRow(4))
	mock.ExpectRollback()

	req := domain.CreatePopulationRequest{CreatedBy: "actor", PreviousRoundID: intPtr(3)}
	_, err := (&populationRepo{db: repo.db}).CreatePopulation(context.Background(), 2, 7, req)
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
