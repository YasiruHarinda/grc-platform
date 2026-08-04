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

package handler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// notifySem is shared package-wide (it bounds every notifyRiskEvent call
// site, not just this test), so this exercises the primitive directly rather
// than draining/refilling it through a real notification, which would need a
// full fake Deps for no added coverage of the thing actually being tested.
func TestNotifySemBoundsConcurrency(t *testing.T) {
	const attempts = notifyConcurrency * 4

	var current, peak int64
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			notifySem <- struct{}{}
			defer func() { <-notifySem }()

			n := atomic.AddInt64(&current, 1)
			for {
				p := atomic.LoadInt64(&peak)
				if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&current, -1)
		}()
	}
	wg.Wait()

	if peak > notifyConcurrency {
		t.Errorf("peak concurrent holders = %d, want <= %d (notifyConcurrency)", peak, notifyConcurrency)
	}
	if peak < notifyConcurrency {
		// Not a correctness failure, but if this ever fires it means the test
		// isn't actually exercising the bound (e.g. attempts too low relative
		// to the sleep), which would silently defeat its own purpose.
		t.Errorf("peak concurrent holders = %d, never reached notifyConcurrency (%d) — test isn't exercising the limit", peak, notifyConcurrency)
	}
}
