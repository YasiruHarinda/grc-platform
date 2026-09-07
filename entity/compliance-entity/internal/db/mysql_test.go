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

package db

import (
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

const baseDSN = "user:pass@tcp(host:3306)/dbname"

// reparse runs the DSN back through the driver's own parser so assertions are
// against the resolved Config rather than a brittle formatted string.
func reparse(t *testing.T, dsn string) *mysql.Config {
	t.Helper()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q): %v", dsn, err)
	}
	return cfg
}

// normalizeDSN always pins the session to UTC regardless of the input DSN.
func TestNormalizeDSNForcesUTC(t *testing.T) {
	cfg := reparse(t, normalizeDSN(baseDSN))

	if !cfg.ParseTime {
		t.Error("ParseTime = false, want true")
	}
	if cfg.Loc != time.UTC {
		t.Errorf("Loc = %v, want UTC", cfg.Loc)
	}
	if got := cfg.Params["time_zone"]; got != "'+00:00'" {
		t.Errorf("time_zone = %q, want %q", got, "'+00:00'")
	}
}

func TestNormalizeDSNTLSHandling(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string // expected resolved cfg.TLSConfig
	}{
		{"omitted tls is defaulted", baseDSN, "preferred"},
		{"bare tls= is defaulted", baseDSN + "?tls=", "preferred"},
		{"tls=false is preserved", baseDSN + "?tls=false", "false"},
		{"tls=true is preserved", baseDSN + "?tls=true", "true"},
		{"tls=skip-verify is preserved", baseDSN + "?tls=skip-verify", "skip-verify"},
		{"tls=preferred is preserved", baseDSN + "?tls=preferred", "preferred"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := reparse(t, normalizeDSN(tc.dsn))
			if cfg.TLSConfig != tc.want {
				t.Errorf("TLSConfig = %q, want %q", cfg.TLSConfig, tc.want)
			}
		})
	}
}

// A DSN the driver cannot parse is returned untouched so sql.Open surfaces the
// error rather than normalizeDSN masking it.
func TestNormalizeDSNMalformedPassThrough(t *testing.T) {
	const bad = "::::not a dsn"
	if got := normalizeDSN(bad); got != bad {
		t.Errorf("normalizeDSN(%q) = %q, want it returned unchanged", bad, got)
	}
}
