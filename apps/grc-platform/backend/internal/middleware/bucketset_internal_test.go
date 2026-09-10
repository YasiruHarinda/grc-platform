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

package middleware

import (
	"strconv"
	"testing"
)

// A limiter that forgets everything when its key set fills up is a limiter an
// attacker can clear on demand: flood it with fresh keys and the key actually
// being throttled gets its burst back.
func TestBucketSetFloodDoesNotRestoreALimitedKey(t *testing.T) {
	s := newBucketSet(1, 2) // burst 2, refilling 1/s — nothing refills mid-test

	for i := range 2 {
		if !s.allow("victim") {
			t.Fatalf("victim should still have budget on call %d", i)
		}
	}
	if s.allow("victim") {
		t.Fatal("victim should be limited once its burst is spent")
	}

	for i := range bucketSetMaxKeys + 10 {
		s.allow("flood-" + strconv.Itoa(i))
	}

	if s.allow("victim") {
		t.Fatal("flooding the key set handed a limited key its budget back")
	}
	if len(s.m) > bucketSetMaxKeys {
		t.Fatalf("key set grew past its cap: %d keys", len(s.m))
	}
}
