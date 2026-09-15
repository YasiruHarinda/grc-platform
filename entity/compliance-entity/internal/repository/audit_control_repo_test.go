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
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// re quotes s so it is matched as a literal substring by sqlmock's default
// regexp query matcher, rather than as a regexp pattern.
func re(s string) string { return regexp.QuoteMeta(s) }

func newControlRepoMock(t *testing.T) (*controlRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &controlRepo{db: db}, mock
}

// controlRow builds one row shaped like scanControl's 36-column Scan call
// (see controlSelectCols/controlFromClause), with every optional column left
// NULL except the ones a caller asks about.
func controlRow(id, auditID int, requirementType, status string) *sqlmock.Rows {
	cols := []string{
		"id", "audit_id",
		"control_number", "description", "evidence_requirement",
		"requirement_type", "control_type", "scope",
		"owner_id", "owner_uuid", "owner_user_type",
		"team_id", "team_name",
		"auditor_id", "auditor_uuid", "auditor_user_type",
		"due_date",
		"status", "sample_reference", "comments", "control_source", "is_overdue",
		"created_at", "updated_at",
		"status_overridden", "overridden_by", "overridden_at",
		"population_description", "population_comments", "population_due_date",
		"population_owner_uuid", "population_owner_user_type", "population_team_name",
		"population_id", "population_owner_id", "population_status",
	}
	now := time.Now()
	return sqlmock.NewRows(cols).AddRow(
		id, auditID,
		"CTRL-1", "a control", nil,
		requirementType, "CONFIG", "COMMON",
		nil, nil, nil,
		nil, nil,
		nil, nil, nil,
		nil,
		status, nil, nil, "MANUAL", false,
		now, now,
		false, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
}

func strPtr(s string) *string { return &s }

// requireConflict/requireValidation assert the error is the apierror variant
// the handler maps to 409/422, and that expectations were consumed —
// including the deferred tx.Rollback() every early-return path triggers.
func requireConflict(t *testing.T, err error) {
	t.Helper()
	var target *apierror.ConflictError
	if !errors.As(err, &target) {
		t.Fatalf("want *apierror.ConflictError, got %T (%v)", err, err)
	}
}

func requireValidation(t *testing.T, err error, wantMsg string) {
	t.Helper()
	var target *apierror.ValidationError
	if !errors.As(err, &target) {
		t.Fatalf("want *apierror.ValidationError, got %T (%v)", err, err)
	}
	if target.Msg != wantMsg {
		t.Fatalf("want message %q, got %q", wantMsg, target.Msg)
	}
}

func TestChangeRequirementType_ControlNotFound(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "DESIGN", UpdatedBy: "actor"})

	var notFound *apierror.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("want *apierror.NotFoundError, got %T (%v)", err, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_AuditRemoved_Conflict(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("DESIGN", "EVIDENCE_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("REMOVED"))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "OE", UpdatedBy: "actor",
			Population: &domain.InlinePopulationRequest{Description: "d", DueDate: strPtr("2026-01-01")}})

	requireConflict(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_EvidenceExists_Locked(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("DESIGN", "EVIDENCE_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "OE", UpdatedBy: "actor",
			Population: &domain.InlinePopulationRequest{Description: "d", DueDate: strPtr("2026-01-01")}})

	requireConflict(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_OE_MissingPopulationDescription_Validation(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("DESIGN", "EVIDENCE_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "OE", UpdatedBy: "actor", Population: nil})

	requireValidation(t, err, "population.description is required for OE controls")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_OE_WrongStatus_Locked(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("DESIGN", "SUBMITTED_SAMPLE"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "OE", UpdatedBy: "actor",
			Population: &domain.InlinePopulationRequest{Description: "d", DueDate: strPtr("2026-01-01")}})

	requireConflict(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_Design_WrongPopulationCount_Locked(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("OE", "POPULATION_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	// Two rounds on one control (a resubmission cycle) — DESIGN requires exactly one.
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).
			AddRow(10, "AUDITOR_REJECTED").AddRow(11, "PENDING"))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "DESIGN", UpdatedBy: "actor"})

	requireConflict(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_Design_RoundNotPending_Locked(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("OE", "POPULATION_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	// Exactly one round, but it already moved past PENDING (submitted for review).
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(10, "SUBMITTED"))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "DESIGN", UpdatedBy: "actor"})

	requireConflict(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestChangeRequirementType_Design_FilesAttached_Locked pins the guard the
// reviewer flagged as most load-bearing: a PENDING round that already has an
// uploaded file must block the DESIGN switch (and therefore the destructive
// audit_population delete) even though status and round-count both look safe.
func TestChangeRequirementType_Design_FilesAttached_Locked(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("OE", "POPULATION_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(55, "PENDING"))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence_file WHERE population_id = ? FOR UPDATE")).
		WithArgs(55).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "DESIGN", UpdatedBy: "actor"})

	requireConflict(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
	// The lock held, so the destructive DELETE must never have been issued —
	// ExpectationsWereMet above already proves no unexpected exec ran.
}

func TestChangeRequirementType_UnknownType_Validation(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("DESIGN", "EVIDENCE_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "BOGUS", UpdatedBy: "actor"})

	requireValidation(t, err, "requirementType must be DESIGN or OE")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestChangeRequirementType_DesignToOE_Success(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("DESIGN", "EVIDENCE_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(re("INSERT INTO audit_population")).
		WillReturnResult(sqlmock.NewResult(99, 1))
	mock.ExpectExec(re("SET requirement_type = ?, status = ?, status_overridden = FALSE")).
		WithArgs("OE", "POPULATION_PENDING", "actor", 1, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_control WHERE audit_id = ? AND status <> 'COMPLETE' FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectCommit()
	mock.ExpectQuery(re("WHERE c.audit_id = ? AND c.id = ?")).
		WithArgs(1, 2).
		WillReturnRows(controlRow(2, 1, "OE", "POPULATION_PENDING"))

	got, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "OE", UpdatedBy: "actor",
			Population: &domain.InlinePopulationRequest{Description: "d", DueDate: strPtr("2026-01-01")}})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RequirementType != "OE" || got.Status != "POPULATION_PENDING" {
		t.Fatalf("got requirementType=%q status=%q, want OE/POPULATION_PENDING", got.RequirementType, got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestChangeRequirementType_OEToDesign_Success is the destructive path the
// reviewer most wanted pinned: it must delete exactly the one PENDING
// population round it already validated is empty of files, detach its
// notifications first (so the RESTRICT FK doesn't block the delete), and
// land the control on EVIDENCE_PENDING.
func TestChangeRequirementType_OEToDesign_Success(t *testing.T) {
	repo, mock := newControlRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT requirement_type, status FROM audit_control WHERE audit_id = ? AND id = ? FOR UPDATE")).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"requirement_type", "status"}).AddRow("OE", "POPULATION_PENDING"))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery(re("SELECT id, status FROM audit_population WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(55, "PENDING"))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence WHERE control_id = ? FOR UPDATE")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_evidence_file WHERE population_id = ? FOR UPDATE")).
		WithArgs(55).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(re("reminder_dedup_key IS NULL")).
		WithArgs(55).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(re("reminder_dedup_key IS NOT NULL")).
		WithArgs(55).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(re("DELETE FROM audit_population WHERE id = ?")).
		WithArgs(55).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(re("SET requirement_type = ?, status = ?, status_overridden = FALSE")).
		WithArgs("DESIGN", "EVIDENCE_PENDING", "actor", 1, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(re("SELECT status FROM audit WHERE id = ? FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	// Every other control in the audit is already COMPLETE: recomputeAuditStatus
	// must flip the audit itself to COMPLETED as part of the same transaction.
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_control WHERE audit_id = ? AND status <> 'COMPLETE' FOR UPDATE")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(re("UPDATE audit SET status = ? WHERE id = ?")).
		WithArgs("COMPLETED", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(re("WHERE c.audit_id = ? AND c.id = ?")).
		WithArgs(1, 2).
		WillReturnRows(controlRow(2, 1, "DESIGN", "EVIDENCE_PENDING"))

	got, err := repo.ChangeRequirementType(context.Background(), 1, 2,
		domain.ChangeRequirementTypeRequest{RequirementType: "DESIGN", UpdatedBy: "actor"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RequirementType != "DESIGN" || got.Status != "EVIDENCE_PENDING" {
		t.Fatalf("got requirementType=%q status=%q, want DESIGN/EVIDENCE_PENDING", got.RequirementType, got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
