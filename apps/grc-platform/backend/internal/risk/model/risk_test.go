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

package model

import (
	"testing"
	"time"
)

func TestAssigneeCorrectionDeadline(t *testing.T) {
	created := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	wantDeadline := time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		createdBy string
		now       time.Time
		wantOpen  bool
	}{
		{"migrated, just created", MigrationMarker, created, true},
		{"migrated, one second before the deadline", MigrationMarker, wantDeadline.Add(-time.Second), true},
		{"migrated, exactly at the deadline", MigrationMarker, wantDeadline, false},
		{"migrated, after the deadline", MigrationMarker, wantDeadline.Add(time.Hour), false},
		{"not migrated", "3f1c9a2e-user-uuid", created, false},
		{"empty created_by", "", created, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AssigneeCorrectionDeadline(tc.createdBy, created, tc.now)
			if !tc.wantOpen {
				if got != nil {
					t.Fatalf("got deadline %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want an open window")
			}
			if !got.Equal(wantDeadline) {
				t.Errorf("deadline = %v, want %v", *got, wantDeadline)
			}
		})
	}
}
