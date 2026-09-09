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
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

// Resolver turns a work email into a compliance-entity user.id, via:
//
//	email --(SCIM snapshot)--> uuid --(GET /users/by-uuid, else POST /users)--> user.id
//
// The snapshot is the one preflight already fetched. In a dry run the resolver
// stops at the uuid: it confirms the email resolves to exactly one uuid but
// never calls the entity, so a dry run writes nothing (plan §9). On a real run
// it resolves the uuid to a user.id, provisioning a user row if the person has
// none — never mutating an existing row (plan §6).
type Resolver struct {
	entity   *EntityClient
	dryRun   bool
	snapshot []DirectoryUser

	uuidByEmail map[string]string // lower(email) -> uuid
	ambiguous   map[string]bool   // lower(email) seen with >1 distinct uuid — never resolvable
	idByUUID    map[string]int    // uuid -> user.id (real-run cache)
}

// loadSnapshotFile reads an email,uuid CSV for local testing in place of the
// live Asgardeo directory (see Config.SCIMSnapshotFile). The first two columns
// are email and uuid; a leading "email,uuid" header row is tolerated, as are
// blank lines and trailing columns. Emails are lowercased and trimmed to match
// how mapRow stores them.
func loadSnapshotFile(path string) ([]DirectoryUser, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied local path, offline testing only
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	var out []DirectoryUser
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 2 {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(rec[0]))
		uuid := strings.TrimSpace(rec[1])
		if email == "" || uuid == "" || email == "email" {
			continue // blank line or the optional header
		}
		out = append(out, DirectoryUser{Email: email, UUID: uuid})
	}
	return out, nil
}

func NewResolver(entity *EntityClient, snapshot []DirectoryUser, dryRun bool) *Resolver {
	return &Resolver{
		entity:      entity,
		dryRun:      dryRun,
		snapshot:    snapshot,
		uuidByEmail: map[string]string{},
		ambiguous:   map[string]bool{},
		idByUUID:    map[string]int{},
	}
}

// Build indexes the snapshot by lowercased email. An email that appears with
// two different uuids is marked ambiguous and will never resolve — the same
// "never pick a winner" rule cmd/backfill-uuids follows.
func (r *Resolver) Build() {
	for _, u := range r.snapshot {
		key := strings.ToLower(strings.TrimSpace(u.Email))
		if key == "" || u.UUID == "" {
			continue
		}
		if existing, seen := r.uuidByEmail[key]; seen {
			if existing != u.UUID {
				r.ambiguous[key] = true
			}
			continue
		}
		r.uuidByEmail[key] = u.UUID
	}
}

// Apply fills the people id fields on every row (the team / category / ref ids
// are already set by mapRow, T4). It returns a Finding per person that could
// not be resolved and, on a real run, a non-nil error if provisioning hit a
// transport/entity failure — the caller then aborts and re-runs to resume.
//
// Rejection vs. warning by column (plan §6, D4 / D7):
//
//   - Risk Owner / Risk Assigned To / Management Approver — always REJECT
//   - Action Owner — REJECT for IN_REMEDIATION rows, WARN + null for CLOSED
func (r *Resolver) Apply(ctx context.Context, rows []Row) ([]Finding, error) {
	var fs []Finding
	for i := range rows {
		row := &rows[i]
		reject := func(field, value, detail string) {
			fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
				Severity: SevReject, Failure: field, Detail: detail, Value: value})
		}
		warn := func(field, value, detail string) {
			fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
				Severity: SevWarn, Failure: field, Detail: detail, Value: value})
		}

		// The three NOT NULL people columns: unresolvable → REJECT (D4).
		for _, p := range []struct {
			field string
			email string
			dst   *int
		}{
			{"Risk Assigned To", row.AssignerEmail, &row.AssignerID},
			{"Risk Owner", row.OwnerEmail, &row.OwnerID},
			{"Management Approver", row.ManagementApproverEmail, &row.ManagementApproverID},
		} {
			id, ok, err := r.userID(ctx, p.email)
			if err != nil {
				return nil, fmt.Errorf("%s %q: %w", p.field, p.email, err)
			}
			if !ok {
				reject(p.field, p.email, r.unresolvedReason(p.email))
				continue
			}
			if !r.dryRun {
				*p.dst = id
			}
		}

		// Action Owner: mandatory for IN_REMEDIATION, nullable for CLOSED (D7).
		inRemediation := row.WorkflowStatus == "IN_REMEDIATION"
		switch {
		case row.ActionOwnerEmail == "":
			if inRemediation {
				reject("Action Owner", "", "mandatory for an IN_REMEDIATION row")
			}
		default:
			id, ok, err := r.userID(ctx, row.ActionOwnerEmail)
			if err != nil {
				return nil, fmt.Errorf("Action Owner %q: %w", row.ActionOwnerEmail, err)
			}
			switch {
			case ok:
				if !r.dryRun {
					actionOwnerID := id
					row.ActionOwnerID = &actionOwnerID
				}
			case inRemediation:
				reject("Action Owner", row.ActionOwnerEmail, r.unresolvedReason(row.ActionOwnerEmail))
			default:
				warn("Action Owner", row.ActionOwnerEmail, r.unresolvedReason(row.ActionOwnerEmail)+" — left null on this CLOSED row")
			}
		}
	}
	return fs, nil
}

// unresolvedReason explains why an email did not resolve, for the finding.
func (r *Resolver) unresolvedReason(email string) string {
	key := strings.ToLower(strings.TrimSpace(email))
	switch {
	case key == "":
		return "blank"
	case r.ambiguous[key]:
		return fmt.Sprintf("%q is ambiguous in the directory — one email, multiple Asgardeo ids", email)
	default:
		return fmt.Sprintf("%q is not in the SCIM directory for this domain", email)
	}
}

// userID resolves one email to a user.id. ok is true when the email maps to
// exactly one uuid (resolvable); in a dry run it returns (0, true, nil) at that
// point without calling the entity. On a real run it resolves the uuid to a
// user.id, provisioning a user row on a 404 (an upsert on uuid — never a
// mutation of an existing row).
func (r *Resolver) userID(ctx context.Context, email string) (id int, ok bool, err error) {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" || r.ambiguous[key] {
		return 0, false, nil
	}
	uuid, found := r.uuidByEmail[key]
	if !found {
		return 0, false, nil
	}
	if r.dryRun {
		return 0, true, nil
	}
	if cached, hit := r.idByUUID[uuid]; hit {
		return cached, true, nil
	}

	id, exists, err := r.entity.GetUserByUUID(ctx, uuid)
	if err != nil {
		return 0, false, err
	}
	if !exists {
		id, err = r.entity.CreateUser(ctx, CreateUserRequest{
			UUID: uuid, UserType: "INTERNAL", Status: "ACTIVE", CreatedBy: marker,
		})
		if err != nil {
			return 0, false, err
		}
	}
	r.idByUUID[uuid] = id
	return id, true, nil
}
