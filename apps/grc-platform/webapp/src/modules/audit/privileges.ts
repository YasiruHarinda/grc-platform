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

// Audit Hub privilege name constants.
// Values must match privilege_name in the privilege table and the constants in
// backend/internal/shared/privilege/privilege.go exactly.
export const AuditPrivilege = {
  ViewAudits:           "AUDIT_VIEW_AUDITS",
  ViewAllAudits:        "AUDIT_VIEW_ALL_AUDITS",
  CreateAudit:          "AUDIT_CREATE_AUDIT",
  UpdateAudit:          "AUDIT_UPDATE_AUDIT",
  ManageControls:       "AUDIT_MANAGE_CONTROLS",
  ManageFrameworks:     "AUDIT_MANAGE_FRAMEWORKS",
  SubmitEvidence:       "AUDIT_SUBMIT_EVIDENCE",
  ReviewEvidence:       "AUDIT_REVIEW_EVIDENCE",
  ValidateEvidence:     "AUDIT_VALIDATE_EVIDENCE",
  SelectSample:         "AUDIT_SELECT_SAMPLE",
  AddComment:           "AUDIT_ADD_COMMENT",
  ViewInternalComments: "AUDIT_VIEW_INTERNAL_COMMENTS",
} as const;
