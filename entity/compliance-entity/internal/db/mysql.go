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

// Package db manages the MySQL connection pool for the compliance entity service.
package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/config"
)

const (
	maxOpenConns    = 20
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

// normalizeDSN pins the connection to UTC so timestamps are consistent end to end.
//
// The DATETIME columns (created_at etc.) default to CURRENT_TIMESTAMP, which is
// evaluated in the session time zone. The go-sql-driver reads DATETIME back in
// its `loc` (UTC by default). If the two disagree — e.g. a server running in a
// local zone — every timestamp is written in local wall-clock but read (and then
// JSON-serialized) as if it were UTC, shifting it by the server's offset. That is
// what makes the UI show wrong times and "just now" for everything.
//
// Forcing session time_zone='+00:00' and loc=UTC makes writes and reads agree on
// UTC regardless of the server's zone. Applied in code so it holds no matter what
// DB_DSN the deployment provides. On a malformed DSN we fall back to the raw value
// and let sql.Open surface the error.
func normalizeDSN(dsn string) string {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return dsn
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	return cfg.FormatDSN()
}

// New opens a MySQL connection pool, pings the database to confirm connectivity,
// and returns the pool ready for use. The caller is responsible for calling db.Close.
func New(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", normalizeDSN(cfg.DBDSN))
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}
