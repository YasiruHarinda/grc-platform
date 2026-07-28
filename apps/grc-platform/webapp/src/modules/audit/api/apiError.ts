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

/**
 * Extracts a human-readable message from a failed API response. Backend error
 * responses are a JSON envelope ({"message": "..."}); this unwraps it so
 * callers don't surface the raw JSON text. Falls back to the raw body (for
 * non-JSON error responses, e.g. from a proxy) or the given default.
 */
export async function extractErrorMessage(res: Response, fallback: string): Promise<string> {
  const body = await res.text().catch(() => "");
  if (!body) return fallback;
  try {
    const parsed = JSON.parse(body) as { message?: unknown };
    if (typeof parsed.message === "string" && parsed.message) {
      return parsed.message;
    }
  } catch {
    // Not JSON — fall through to the raw body.
  }
  return body;
}
