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

import "net/http"

// CORS adds the Access-Control-* headers required for browser cross-origin requests.
// It must wrap the outermost handler so that OPTIONS preflight requests are answered
// before they reach the auth middleware (which would reject them — no token is sent).
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	if allowedOrigin == "*" {
		panic("cors: wildcard '*' is not a safe CORS origin; set CORS_ALLOWED_ORIGIN to a specific origin")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, x-user-id-token, X-Correlation-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// Preflight — respond immediately without forwarding to the auth middleware.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
