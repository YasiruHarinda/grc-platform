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

// Configuration for the Logger service.
//
// The level is fixed rather than read from window.config. Every call site in
// this app is logger.error — there is not one logger.debug/info/warn — so no
// value of the old GRC_PLATFORM_LOG_LEVEL changed a single line of console
// output. It read as a control on production console noise and controlled
// nothing, which is worse than having no knob at all.
//
// The Logger class still supports the full DEBUG..ERROR range. Reintroduce the
// config key together with the first real debug/info/warn calls — and with the
// raw console.* sites (RiskRegisters, RiskDashboard, RiskAnalytics,
// useAuthApiClient, authConfig) routed through the logger, since those print in
// production today and no level setting has ever gated them.
export const loggerConfig = {
  level: "ERROR",
  prefix: "GRCPlatform",
};
