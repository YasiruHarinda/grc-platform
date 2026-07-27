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

// Oxygen UI's dark theme gives Paper/Card a near-black background
// (background.paper: "#000000b8"), which is indistinguishable from the
// page background. This override lightens the card so it reads as a
// distinct surface in dark mode, matched across every card/table/chart
// container in the Risk module (Add Risk, Dashboard, Analytics, Registers).
export const darkCardSx = {
  "[data-color-scheme='dark'] &": {
    backgroundColor: "rgba(36, 36, 36, 0.6)",
    borderColor: "rgba(255, 255, 255, 0.16)",
  },
};

// The active theme (AcrylicOrangeTheme) gives every Dialog/Drawer a blurred,
// translucent Paper background by default (glassmorphism) — readable for
// some surfaces, but washes out form dialogs. Spread this into a Dialog's
// PaperProps.sx (or a Drawer's, alongside its own layout sx) to opt out.
// Distinct from the datepicker/Autocomplete popover fix (#fff / #1e1e1e,
// no backgroundImage) used elsewhere — that's a different surface with
// deliberately different values, not the same duplication.
export const dialogPaperSx = {
  backdropFilter: "none",
  backgroundImage: "none",
  backgroundColor: "#ffffff",
  "[data-color-scheme='dark'] &": { backgroundColor: "#1a1a24" },
};
