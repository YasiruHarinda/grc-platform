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

// The active theme (AcrylicOrangeTheme) gives every Dialog a blurred,
// translucent Paper background by default (glassmorphism) — reads as
// unreadably transparent for a dense form dialog. Spread this into a
// Dialog's PaperProps.sx to opt out.
//
// Deliberately duplicated from modules/risk/pages/cardStyles.ts's identical
// `dialogPaperSx`, rather than imported across the module boundary — same
// call made for useAdminPrivileges vs. useRiskPrivileges (see that hook's
// doc comment): modules/admin and modules/risk are meant to stay
// independent, and six lines of shared sx is a cheap enough duplication to
// keep it that way.
export const dialogPaperSx = {
  backdropFilter: "none",
  backgroundImage: "none",
  backgroundColor: "#ffffff",
  "[data-color-scheme='dark'] &": { backgroundColor: "#1a1a24" },
};
