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
import { Box, CircularProgress } from "@wso2/oxygen-ui";
import { Navigate } from "react-router";
import NoAccessPage from "@components/error/NoAccessPage";
import ErrorLayout from "@layouts/ErrorLayout";
import { resolveVisibleNav } from "./resolveVisibleNav";
import { useSectionPrivileges } from "./useSectionPrivileges";
import { SECTIONS } from "./sections";

// Renders at "/". Sends the user to the first nav item they can actually see —
// which is no longer always the Audit dashboard now that whole sections hide
// from users who lack their privileges. When nothing is visible (no module
// privilege and not an Action Owner on any risk), shows NoAccessPage instead of
// bouncing them into a 403.
//
// Blocks on a spinner until every privilege resolver has settled, so the user
// sees one clean transition rather than a redirect that corrects itself.
export default function LandingRedirect(): JSX.Element {
  const sectionPrivs = useSectionPrivileges();
  const { sections, loading } = resolveVisibleNav(SECTIONS, sectionPrivs);

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", height: "100dvh" }}>
        <CircularProgress />
      </Box>
    );
  }

  const firstItem = sections.flatMap((s) => s.items)[0];
  if (!firstItem) {
    return (
      <ErrorLayout>
        <NoAccessPage />
      </ErrorLayout>
    );
  }

  return <Navigate to={firstItem.path} replace />;
}
