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

// Package integration contains end-to-end tests that run against a real MySQL
// database. Set DB_DSN in the environment before running.
package integration

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		os.Exit(0) // skip when no database is configured
	}
	var err error
	testDB, err = sql.Open("mysql", dsn)
	if err != nil {
		panic("integration: open: " + err.Error())
	}
	if err := testDB.Ping(); err != nil {
		panic("integration: ping: " + err.Error())
	}
	exitCode := m.Run()
	_ = testDB.Close()
	os.Exit(exitCode)
}
