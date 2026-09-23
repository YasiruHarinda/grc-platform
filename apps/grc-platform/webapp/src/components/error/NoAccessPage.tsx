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
import { useIdTokenClaims } from "@hooks/useIdTokenClaims";
import illustration from "@assets/error/error-403.svg";

// Domain that marks a signed-in user as internal, for COPY ONLY — it picks one
// sentence below and never gates access. The backend is the authority on who
// is internal (callerGuard in middleware/caller.go, configured by
// AUTH_INTERNAL_EMAIL_DOMAINS) and trusts nothing from here. If the two ever
// disagree, the cost is one surplus or missing line of text.
const INTERNAL_EMAIL_DOMAIN = "wso2.com";

// Shown by LandingRedirect when the signed-in user can see no module at all —
// no Audit Hub privilege. Two very different people land here:
//
//   - an internal user, who may be looking for Risk Hub or the Admin Console.
//     Both live in One WSO2 now, so say so.
//   - an external auditor, mid-onboarding or with a grant revoked once their
//     audit closed. They cannot sign into One WSO2 at all, so sending them
//     there would be a dead end.
//
// So the One WSO2 line is internal-only, while the actionable "contact an
// administrator" line comes first and everyone sees it. The line stays hidden
// while the claims load, which fails safe: an external auditor never sees it,
// and an internal user sees it appear a moment later.
export default function NoAccessPage(): JSX.Element {
  const claims = useIdTokenClaims();
  const email = typeof claims?.email === "string" ? claims.email : "";
  const isInternal = email.toLowerCase().endsWith(`@${INTERNAL_EMAIL_DOMAIN}`);

  const description =
    "Your account doesn't have access to the Audit Hub yet.\n" +
    "Contact a platform administrator to get a role assigned." +
    (isInternal ? "\n\nRisk Hub and the Admin Console are in One WSO2." : "");

  return (
    <ErrorPage
      illustration={illustration}
      illustrationAlt="no module access illustration"
      description={description}
    />
  );
}
