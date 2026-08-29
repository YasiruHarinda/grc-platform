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
 * User returned by GET /api/v1/audits/users.
 * Currently sourced from the MySQL `user` table.
 * When Asgardeo SCIM2 is integrated, `asgardeoId: string` will be added alongside.
 */
export interface AuditUser {
  id: number;
  displayName: string;
  email: string | null;
  userType: "INTERNAL" | "EXTERNAL";
  /** Thumbnail photo URL, or null if not set. */
  profileUrl: string | null;
}
