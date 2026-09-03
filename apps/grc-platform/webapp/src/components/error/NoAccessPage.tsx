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

import { type JSX } from "react";
import ErrorPage from "./ErrorPage";
import illustration from "@assets/error/error-403.svg";

// Shown by LandingRedirect when the signed-in user can see no module at all —
// no Risk/Audit/Admin privilege and not an Action Owner on any risk. This is a
// legitimate early state for a brand-new employee, not a failure, so the copy
// is informational rather than an error.
export default function NoAccessPage(): JSX.Element {
  return (
    <ErrorPage
      illustration={illustration}
      illustrationAlt="no module access illustration"
      description={
        "Your account doesn't have access to any module yet.\n" +
        "Contact a platform administrator to get a role assigned."
      }
    />
  );
}
