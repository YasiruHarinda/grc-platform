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

package config

import (
	"slices"
	"testing"
)

func TestLoadInternalEmailDomains(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		fallback string
		want     []string
		wantErr  bool
	}{
		{name: "defaults to the SCIM user domain", fallback: "wso2.com", want: []string{"wso2.com"}},
		{name: "overrides the fallback", env: "example.com", fallback: "wso2.com", want: []string{"example.com"}},
		{name: "splits and trims a list", env: "wso2.com, Acquired.Example ", fallback: "wso2.com",
			want: []string{"wso2.com", "acquired.example"}},

		// Would 403 the whole company, or match any empty domain part.
		{name: "rejects an empty set", fallback: "", wantErr: true},
		{name: "rejects a blank set", env: "   ", fallback: "wso2.com", wantErr: true},
		{name: "rejects a trailing comma", env: "wso2.com,", fallback: "wso2.com", wantErr: true},
		{name: "rejects an interior blank", env: "wso2.com, ,example.com", fallback: "wso2.com", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear first: an inherited value would leak into the unset cases.
			t.Setenv("AUTH_INTERNAL_EMAIL_DOMAINS", "")
			if tc.env != "" {
				t.Setenv("AUTH_INTERNAL_EMAIL_DOMAINS", tc.env)
			}
			got, err := loadInternalEmailDomains(tc.fallback)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got %v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
