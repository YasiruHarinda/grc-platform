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

package service

import (
	"context"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// employeeSearchLimit caps how many HR entity results are fetched per
// keystroke search — this is a live typeahead, not a full directory dump.
const employeeSearchLimit = 20

// EmployeeSearchService looks up active WSO2 employees from the HR entity
// service for the Risk module's "Risk Identified By: Employee" field.
type EmployeeSearchService interface {
	Search(ctx context.Context, query string) ([]model.EmployeeOption, error)
	// Resolve verifies a single employee by work email and returns their
	// canonical name, or nil if no active employee has that email. Unlike
	// Search, this is the server-side trust boundary: it is used to derive
	// identified_by_name from hr_entity rather than accept a client-supplied
	// string, so a risk cannot be attributed to a fabricated employee name.
	Resolve(ctx context.Context, email string) (*model.EmployeeOption, error)
}

type employeeSearchService struct {
	hrClient *hrentity.Client
}

// NewEmployeeSearchService creates an EmployeeSearchService backed by the
// given HR entity GraphQL client.
func NewEmployeeSearchService(hrClient *hrentity.Client) EmployeeSearchService {
	return &employeeSearchService{hrClient: hrClient}
}

func (s *employeeSearchService) Search(ctx context.Context, query string) ([]model.EmployeeOption, error) {
	employees, err := s.hrClient.SearchActiveEmployees(ctx, query, employeeSearchLimit)
	if err != nil {
		return nil, err
	}

	options := make([]model.EmployeeOption, 0, len(employees))
	for _, e := range employees {
		name := strings.TrimSpace(strings.TrimSpace(e.FirstName) + " " + strings.TrimSpace(e.LastName))
		if name == "" || e.WorkEmail == "" {
			continue
		}
		options = append(options, model.EmployeeOption{Name: name, Email: e.WorkEmail})
	}
	return options, nil
}

func (s *employeeSearchService) Resolve(ctx context.Context, email string) (*model.EmployeeOption, error) {
	emp, err := s.hrClient.GetEmployeeByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if emp == nil {
		return nil, nil
	}
	name := strings.TrimSpace(strings.TrimSpace(emp.FirstName) + " " + strings.TrimSpace(emp.LastName))
	if name == "" {
		return nil, nil
	}
	return &model.EmployeeOption{Name: name, Email: emp.WorkEmail}, nil
}
