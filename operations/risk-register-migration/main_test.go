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

package main

import (
	"testing"
	"time"
)

func TestParseBoolEnv_WriteGateDefaultsToDryRun(t *testing.T) {
	// DRY_RUN defaults true — the tool must be told explicitly to write. An
	// unrecognized value is an error, not a silent flip to write mode: a typo
	// must fail loud, never fail toward the destructive branch.
	cases := map[string]struct {
		set     bool
		val     string
		def     bool
		want    bool
		wantErr bool
	}{
		"unset keeps default true":  {set: false, def: true, want: true},
		"unset keeps default false": {set: false, def: false, want: false},
		"true":                      {set: true, val: "true", def: true, want: true},
		"TRUE mixed case":           {set: true, val: "TRUE", def: true, want: true},
		"1":                         {set: true, val: "1", def: true, want: true},
		"yes":                       {set: true, val: "yes", def: true, want: true},
		"false":                     {set: true, val: "false", def: true, want: false},
		"0":                         {set: true, val: "0", def: true, want: false},
		"no":                        {set: true, val: "no", def: true, want: false},
		"empty string is default":   {set: true, val: "", def: true, want: true},
		"typo errors, not false":    {set: true, val: "flase", def: true, wantErr: true},
		"garbage errors, not false": {set: true, val: "maybe", def: true, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.set {
				t.Setenv("DRY_RUN_TEST_KEY", tc.val)
			}
			got, err := parseBoolEnv("DRY_RUN_TEST_KEY", tc.def)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseBoolEnv(%q) = %v, <nil>, want an error", tc.val, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBoolEnv(%q) unexpected error: %v", tc.val, err)
			}
			if got != tc.want {
				t.Errorf("parseBoolEnv(%q, %v) = %v, want %v", tc.val, tc.def, got, tc.want)
			}
		})
	}
}

func TestEnvDuration(t *testing.T) {
	if got := envDuration("MISSING_DURATION_KEY", 90*time.Second); got != 90*time.Second {
		t.Errorf("missing key should return the default, got %v", got)
	}
	t.Setenv("DURATION_KEY", "2m30s")
	if got := envDuration("DURATION_KEY", time.Second); got != 2*time.Minute+30*time.Second {
		t.Errorf("envDuration = %v", got)
	}
	t.Setenv("DURATION_KEY", "not-a-duration")
	if got := envDuration("DURATION_KEY", 5*time.Second); got != 5*time.Second {
		t.Errorf("an unparseable value should fall back to the default, got %v", got)
	}
}

func TestEnvOr(t *testing.T) {
	if got := envOr("UNSET_KEY_XYZ", "fallback"); got != "fallback" {
		t.Errorf("envOr unset = %q", got)
	}
	t.Setenv("SET_KEY_XYZ", "value")
	if got := envOr("SET_KEY_XYZ", "fallback"); got != "value" {
		t.Errorf("envOr set = %q", got)
	}
}
