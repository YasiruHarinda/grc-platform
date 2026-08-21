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
import {
  createComplianceReference,
  deleteComplianceReference,
  fetchComplianceReferences,
  updateComplianceReference,
} from "../api/adminApi";
import SimpleReferenceCrudPage from "../components/SimpleReferenceCrudPage";

export default function ComplianceReferencesPage(): JSX.Element {
  return (
    <SimpleReferenceCrudPage
      addLabel="Add Reference"
      itemLabel="reference"
      emptyLabel="No references found."
      nameHint="e.g. ISO 27001, SOC 2, PCI DSS — shown wherever a risk is linked to a framework."
      descriptionHint="A short explanation of what this framework covers."
      fetchAll={fetchComplianceReferences}
      create={createComplianceReference}
      update={updateComplianceReference}
      del={deleteComplianceReference}
    />
  );
}
