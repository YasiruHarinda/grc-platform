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

import { useIdTokenClaims } from "@hooks/useIdTokenClaims";

/**
 * The signed-in user's Asgardeo uuid, from the `sub` ID-token claim the
 * backend reads as auth.FromContext(ctx).Subject. Used purely for UI gating
 * (e.g. "is this my comment") — the backend re-derives and enforces
 * ownership independently from the same claim, so a mismatch here only
 * hides a button, it never grants access.
 */
export function useCurrentUserId(): string | null {
  const claims = useIdTokenClaims();
  const sub = claims?.sub;
  return typeof sub === "string" ? sub : null;
}
