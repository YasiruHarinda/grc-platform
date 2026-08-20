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
import { createRiskCategory, deleteRiskCategory, fetchRiskCategories, updateRiskCategory } from "../api/adminApi";
import SimpleReferenceCrudPage from "../components/SimpleReferenceCrudPage";

// No status column — risk_category has no status field in the schema, unlike
// risk_team/user, so there's no soft-delete/deactivate option here: deleting
// a category is a real DELETE, refused server-side while any risk still uses
// it (see deleteRiskCategory's doc comment).
export default function RiskCategoriesPage(): JSX.Element {
  return (
    <SimpleReferenceCrudPage
      addLabel="Add Category"
      itemLabel="category"
      emptyLabel="No categories found."
      nameHint="Shown in the risk-creation form's category picker."
      descriptionHint="What kinds of risk belong in this category — helps whoever's classifying a new risk pick the right one."
      fetchAll={fetchRiskCategories}
      create={createRiskCategory}
      update={updateRiskCategory}
      del={deleteRiskCategory}
    />
  );
}
