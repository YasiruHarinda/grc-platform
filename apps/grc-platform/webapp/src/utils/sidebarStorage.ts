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

const SIDEBAR_COLLAPSED_KEY = "sidebar_collapsed";
const PENDING_SUCCESS_MESSAGE_KEY = "pending_success_message";

export function getSidebarCollapsed(): boolean {
  try {
    const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
    if (stored === null) return false;
    return stored === "true";
  } catch {
    return false;
  }
}

export function setSidebarCollapsed(collapsed: boolean): void {
  try {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(collapsed));
  } catch {
    return;
  }
}

export function consumePendingSuccessMessage(): string | null {
  try {
    const msg = sessionStorage.getItem(PENDING_SUCCESS_MESSAGE_KEY);
    if (msg !== null) sessionStorage.removeItem(PENDING_SUCCESS_MESSAGE_KEY);
    return msg;
  } catch {
    return null;
  }
}
