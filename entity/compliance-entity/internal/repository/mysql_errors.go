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
	"errors"

	"github.com/go-sql-driver/mysql"
)

// isFKViolation reports whether err is a MySQL foreign-key constraint failure
// in either direction: 1452 (ER_NO_REFERENCED_ROW_2) — the parent row an
// INSERT/UPDATE points at does not exist — or 1451 (ER_ROW_IS_REFERENCED_2) —
// a DELETE/UPDATE of this row is blocked because a child still references it
// through an ON DELETE RESTRICT/NO ACTION constraint. Callers map both to a
// client-facing 4xx rather than a 500.
func isFKViolation(err error) bool {
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr) && (myErr.Number == 1451 || myErr.Number == 1452)
}

// isDuplicateKey reports whether err is a MySQL 1062 duplicate-entry error
// (ER_DUP_ENTRY), which means the row already exists.
func isDuplicateKey(err error) bool {
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr) && myErr.Number == 1062
}
