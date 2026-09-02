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
// The level is fixed: every call site here is logger.error, so the old
// GRC_PLATFORM_LOG_LEVEL changed no output at all. Reintroduce it alongside the
// first real debug/info/warn calls, and with the raw console.* sites routed
// through the logger — those print in production, ungated by any level.
export const loggerConfig = {
  level: "ERROR",
  prefix: "GRCPlatform",
};
