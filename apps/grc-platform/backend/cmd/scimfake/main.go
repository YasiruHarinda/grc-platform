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

// Command scimfake is a local stand-in for the SCIM Operations Service, for
// developing against the identity directory without reaching the real one.
//
// # WHY THIS EXISTS
//
// The real service is Choreo-hosted on WSO2's internal network — it needs VPN,
// it cold starts, and its Users-search endpoint needs a scope
// (org_internal:users:read) that this platform does not hold yet. None of that
// is a good dependency for a unit test or for offline work.
//
// It serves both endpoints this platform calls, plus the OAuth2 token endpoint,
// so pointing SCIM_BASE_URL and SCIM_TOKEN_URL at it exercises the real
// internal/scim client code end to end — request building, the filter string,
// the 201-not-200 status, JSON decoding — with no client changes and no
// interface to stub. Both env vars have to move together: one base URL covers
// both endpoints, so real and fake cannot be mixed.
//
// # IT HOLDS NO REAL PEOPLE
//
// The built-in seed is synthetic. Real WSO2 uuids are stable identifiers for
// real employees, so they are deliberately not committed here — pass -seed with
// a file outside the repo if you need to reproduce a live case locally.
//
// Usage:
//
//	scimfake [-addr :9099] [-seed users.json]
//
//	SCIM_BASE_URL=http://localhost:9099
//	SCIM_TOKEN_URL=http://localhost:9099/oauth2/token
//
// The seed file is a JSON array:
//
//	[{"email":"nimali.re@wso2.com","uuid":"...","firstName":"Nimali","lastName":"Rajapaksha",
//	  "groups":["wso2-everyone"]}]
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// seedUser is one person in the fake directory.
type seedUser struct {
	Email     string   `json:"email"`
	UUID      string   `json:"uuid"`
	FirstName string   `json:"firstName"`
	LastName  string   `json:"lastName"`
	Groups    []string `json:"groups"`
}

// defaultSeed is deliberately synthetic — see the package comment. The uuids
// are well-formed but invented, and the emails follow the project's
// obviously-fake convention rather than looking like real colleagues.
var defaultSeed = []seedUser{
	{Email: "nimali.re@wso2.com", UUID: "11111111-1111-4111-8111-111111111111",
		FirstName: "Nimali", LastName: "Rajapaksha", Groups: []string{"wso2-everyone"}},
	{Email: "suneth.ga@wso2.com", UUID: "22222222-2222-4222-8222-222222222222",
		FirstName: "Suneth", LastName: "Gamage", Groups: []string{"wso2-everyone"}},
	{Email: "tharindu.we@wso2.com", UUID: "33333333-3333-4333-8333-333333333333",
		FirstName: "Tharindu", LastName: "Weerasinghe", Groups: []string{"wso2-everyone"}},
}

func main() {
	addr := ""
	seedPath := ""
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-addr":
			i++
			if i < len(os.Args) {
				addr = os.Args[i]
			}
		case "-seed":
			i++
			if i < len(os.Args) {
				seedPath = os.Args[i]
			}
		}
	}
	if addr == "" {
		addr = ":9099"
	}

	users := defaultSeed
	if seedPath != "" {
		raw, err := os.ReadFile(seedPath)
		if err != nil {
			log.Fatalf("scimfake: read seed %s: %v", seedPath, err)
		}
		users = nil
		if err := json.Unmarshal(raw, &users); err != nil {
			log.Fatalf("scimfake: parse seed %s: %v", seedPath, err)
		}
		log.Printf("scimfake: loaded %d user(s) from %s", len(users), seedPath)
	} else {
		log.Printf("scimfake: using the built-in synthetic seed (%d users); pass -seed for your own",
			len(users))
	}

	mux := http.NewServeMux()

	// The token endpoint. The real one is Asgardeo, and internal/scim only reads
	// access_token and expires_in off the response, so nothing here needs to be
	// a real JWT — this client never inspects it.
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "scimfake-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})

	mux.HandleFunc("POST /organizations/internal/groups/search",
		func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Filter string `json:"filter"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			group := filterValue(body.Filter)

			var members []map[string]any
			for _, u := range users {
				if !hasGroup(u, group) {
					continue
				}
				// "display" comes back userstore-domain-prefixed from the real
				// service, and internal/scim strips that prefix — so the fake
				// has to add it, or it would not exercise that code at all.
				members = append(members, map[string]any{
					"display": "DEFAULT/" + u.Email,
					"value":   u.UUID,
				})
			}

			resources := []map[string]any{}
			if group != "" && len(members) > 0 {
				resources = append(resources, map[string]any{
					"id":          "fake-group-" + group,
					"displayName": "DEFAULT/" + group,
					"members":     members,
				})
			}
			log.Printf("scimfake: groups/search filter=%q → %d member(s)", body.Filter, len(members))

			// 201, not 200: the real service answers a successful search POST
			// with 201, and internal/scim accepts both. Returning 200 here would
			// hide a client that only tolerated one of them.
			writeJSON(w, http.StatusCreated, map[string]any{
				"totalResults": len(resources),
				"startIndex":   1,
				"itemsPerPage": len(resources),
				"Resources":    resources,
				"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
			})
		})

	// Users-search. Shape taken from the service's own Ballerina types
	// (modules/scim/types.bal): a UserSearchResult of open User records, each
	// carrying id and userName, with Asgardeo's further attributes — name among
	// them — passing through.
	//
	// This is the endpoint whose scope (org_internal:users:read) has not been
	// granted yet, so this fake is currently the only way to exercise the code
	// that calls it. Re-verify against the real service once the scope lands:
	// a fake agreeing with itself proves nothing about the contract.
	mux.HandleFunc("POST /organizations/internal/users/search",
		func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Filter string `json:"filter"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			want := filterValue(body.Filter)

			resources := []map[string]any{}
			for _, u := range users {
				// The client filters on `userName eq <email>`; matching the uuid
				// too keeps this useful for the by-id lookups enrichment needs.
				if !strings.EqualFold(u.Email, want) && u.UUID != want {
					continue
				}
				resources = append(resources, map[string]any{
					"id":       u.UUID,
					"userName": u.Email,
					"name": map[string]any{
						"givenName":  u.FirstName,
						"familyName": u.LastName,
					},
				})
			}
			log.Printf("scimfake: users/search filter=%q → %d user(s)", body.Filter, len(resources))

			writeJSON(w, http.StatusCreated, map[string]any{
				"totalResults": len(resources),
				"startIndex":   1,
				"itemsPerPage": len(resources),
				"Resources":    resources,
				"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
			})
		})

	log.Printf("scimfake: listening on %s", addr)
	log.Printf("scimfake:   SCIM_BASE_URL=http://localhost%s", addr)
	log.Printf("scimfake:   SCIM_TOKEN_URL=http://localhost%s/oauth2/token", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("scimfake: %v", err)
	}
}

// filterValue pulls the compared value out of a SCIM filter like
// `displayName eq "wso2-everyone"`.
//
// Only `eq` is handled, and only the single-term form — that is all this
// platform's client sends. Quoting is accepted either way because the real
// service tolerates both, and the client happens to send the quoted form.
func filterValue(filter string) string {
	parts := strings.SplitN(filter, " eq ", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
}

func hasGroup(u seedUser, group string) bool {
	if group == "" {
		return false
	}
	for _, g := range u.Groups {
		if strings.EqualFold(g, group) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "scimfake: encode response: %v\n", err)
	}
}
