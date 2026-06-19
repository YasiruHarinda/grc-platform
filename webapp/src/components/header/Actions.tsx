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

import {
  Box,
  ColorSchemeToggle,
  Divider,
  Header as HeaderUI,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import UserProfile from "./UserProfile";

interface ActionsProps {
  showUserProfile?: boolean;
}

export default function Actions({
  showUserProfile = true,
}: ActionsProps): JSX.Element {
  return (
    <HeaderUI.Actions
      sx={{
        flexShrink: 0,
        minWidth: 0,
        gap: { xs: 0.5, sm: 0.75, lg: 0.5, xl: 1 },
      }}
    >
      <ColorSchemeToggle />
      <Divider
        orientation="vertical"
        flexItem
        sx={{
          mx: { sm: 0.75, xl: 1 },
          display: { xs: "none", sm: "block" },
          visibility: showUserProfile ? "visible" : "hidden",
        }}
      />
      {showUserProfile ? (
        <UserProfile />
      ) : (
        <Box sx={{ width: 40, height: 40 }} />
      )}
    </HeaderUI.Actions>
  );
}
