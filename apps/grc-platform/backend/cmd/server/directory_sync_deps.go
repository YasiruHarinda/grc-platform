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

package main

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin"
	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditjob "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/job"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directorysync"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	riskjob "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/job"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// buildDirectorySyncJob wires the Directory Status Sync. Every dependency is a
// plain function or narrow interface, as the audit reminder job's are. The
// per-hub work — gathering assignments, resolving recipients, sending the
// digest — lives in each module's job package beside its reminder/escalation
// sweep, not in its handler package.
func buildDirectorySyncJob(
	adminRepo admin.Repository,
	users userentity.Repository,
	dirSvc *directory.Service,
	auditDeps *audithandler.Deps,
	riskDeps *riskhandler.Deps,
	activityLog *adminactivity.Client,
	emailEnabled bool,
) *directorysync.Job {
	auditDep := auditjob.NewDepartureHub(auditjob.DepartureDeps{
		Audits:          auditDeps.Audit,
		Controls:        auditDeps.Control,
		Users:           auditDeps.Users,
		Grants:          auditDeps.Grants,
		Directory:       auditDeps.Directory,
		Email:           auditDeps.Email,
		FrontendBaseURL: auditDeps.FrontendBaseURL,
	})
	riskDep := riskjob.NewDepartureHub(riskjob.DepartureDeps{
		Risks:           riskDeps.Risk,
		Users:           riskDeps.Users,
		Grants:          riskDeps.Grants,
		Directory:       riskDeps.Directory,
		Email:           riskDeps.Email,
		FrontendBaseURL: riskDeps.FrontendBaseURL,
	})

	return directorysync.New(directorysync.Deps{
		// SearchUsers is the one read carrying uuid, user type and status together.
		Users: func(ctx context.Context) ([]directorysync.User, error) {
			rows, err := adminRepo.SearchUsers(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]directorysync.User, 0, len(rows))
			for _, u := range rows {
				out = append(out, directorysync.User{
					ID: u.ID, UUID: u.UUID, UserType: u.UserType, Status: u.Status,
				})
			}
			return out, nil
		},
		Directory:   dirSvc,
		ResolveName: dirSvc.DescribeTyped,
		Disable: func(ctx context.Context, u directorysync.User, name string) error {
			return directorysync.Disable(ctx, users, activityLog, u, name)
		},
		Hubs: []directorysync.Hub{
			{
				Name:        "audit",
				Assignments: auditDep.Assignments,
				Recipients:  auditDep.Recipients,
				Notify:      auditDep.Notify,
			},
			{
				Name:        "risk",
				Assignments: riskDep.Assignments,
				Recipients:  riskDep.Recipients,
				Notify:      riskDep.Notify,
			},
		},
		EmailEnabled: emailEnabled,
	})
}
