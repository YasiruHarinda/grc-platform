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

// Package scim is a client for the internal SCIM Operations Service
// (digiops-infra/operations/scim-operations-service), used to resolve
// identity between a person's email and their Asgardeo user id (uuid) — the
// only identifier the `user` table stores (see shared.sql). This client never
// modifies the SCIM Operations Service or Asgardeo itself; it only calls the
// service's existing Users-search endpoint.
package scim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client talks to the SCIM Operations Service's "internal" organization
// endpoints. The service itself sits behind Choreo API Management with
// OAuth2 client-credentials auth, mirroring the hr_entity client's pattern.
type Client struct {
	baseURL      string
	tokenURL     string
	clientID     string
	clientSecret string
	// scopes is a space-separated OAuth2 scope string requested on every token
	// exchange — every method on this client needs org_internal:users:read.
	// Asgardeo silently omits any scope the application isn't authorized for
	// from the issued token rather than erroring, so an empty/wrong value here
	// surfaces later as a 403 "Scope validation failed" from the SCIM
	// Operations Service's gateway, not as a token-request failure.
	scopes     string
	httpClient *http.Client

	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// tokenExpiryBuffer is subtracted from the token's reported lifetime so a
// near-expiry token is never handed to an in-flight request.
const tokenExpiryBuffer = 30 * time.Second

// NewClient creates a Client for the SCIM Operations Service at baseURL,
// authenticating via OAuth2 client-credentials at tokenURL. scopes is a
// space-separated list requested on every token exchange (see Client.scopes).
func NewClient(baseURL, tokenURL, clientID, clientSecret, scopes string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		scopes:       scopes,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

// SetHTTPTimeout replaces the per-request timeout. The 5s default suits the
// live request path, where a user is waiting and a slow directory should fail
// fast rather than hold the request open.
//
// Offline tools want the opposite: this service is Choreo-hosted and cold
// starts, so a first call can take far longer than any interactive budget,
// and a batch job that has no user waiting would rather wait than abort a
// whole migration run.
//
// Call it during setup, before any request — it is not safe to call
// concurrently with in-flight calls.
func (c *Client) SetHTTPTimeout(d time.Duration) {
	c.httpClient.Timeout = d
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// accessToken returns a valid bearer token for the SCIM Operations Service,
// reusing the cached one until it's close to expiry.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if c.scopes != "" {
		form.Set("scope", c.scopes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build scim token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call scim token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scim token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode scim token response: %w", err)
	}

	c.cachedToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenExpiryBuffer)
	return c.cachedToken, nil
}

// stripGroupDomain removes the userstore domain prefix SCIM entries can come
// back with (e.g. "DEFAULT/jane@wso2.com" -> "jane@wso2.com"). Falls back to
// the raw value if there's no "/", rather than dropping it, so an unexpected
// format degrades to "probably wrong" instead of "silently gone." Named for
// its original use against group member entries; searchUsersPage applies it
// to userName too, on the same belt-and-suspenders basis.
func stripGroupDomain(display string) string {
	if i := strings.LastIndex(display, "/"); i != -1 {
		return display[i+1:]
	}
	return display
}

// DirectoryUser is a person as the identity directory knows them.
//
// This is the only place a display name can come from once the platform stops
// storing one, which is why it is a distinct type from anything in the `user`
// table: it is not persisted, it is fetched.
type DirectoryUser struct {
	UUID        string
	Email       string
	DisplayName string
}

type userSearchInput struct {
	Filter     string   `json:"filter"`
	Attributes []string `json:"attributes,omitempty"`
	// StartIndex/ItemsPerPage are 1-indexed SCIM pagination — zero means
	// "unset" and is omitted, letting the server apply its own default
	// (observed to be 100, empirically, regardless of what's requested below
	// it — this platform requests it explicitly instead of relying on that).
	StartIndex   int `json:"startIndex,omitempty"`
	ItemsPerPage int `json:"itemsPerPage,omitempty"`
}

// scimUser is one entry in a Users-search result. The service's own type is an
// open record, so Asgardeo's extra attributes (name, emails, …) pass through
// and can be read here even though its Ballerina definition names only two.
type scimUser struct {
	ID       string `json:"id"`
	UserName string `json:"userName"`
	Name     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
}

type userSearchResult struct {
	TotalResults int        `json:"totalResults"`
	Resources    []scimUser `json:"Resources"`
}

// LookupByEmail resolves an email to that person's directory record — their
// Asgardeo id and display name.
//
// Requires the org_internal:users:read scope, which is narrower than the groups
// scope this client also uses. Asgardeo silently omits a scope the application
// is not authorised for rather than failing the token request, so a missing
// grant surfaces here as a 403 from the gateway at call time, not as a
// configuration error at startup.
//
// Returns (nil, nil) when the directory knows no such user. That is an ordinary
// answer, not a failure: an employee may be assignable in HR long before they
// have an Asgardeo account, and the caller decides what to do about it.
// A nil Client means the directory is not configured — local development
// without credentials for this internal, VPN-only service. It answers "no such
// user" rather than panicking, so callers written to tolerate an unknown person
// behave the same way whether the directory is absent or merely unaware of
// them. Callers that must not proceed without a real answer have to check for
// a nil client themselves; none do today.
func (c *Client) LookupByEmail(ctx context.Context, email string) (*DirectoryUser, error) {
	if c == nil {
		return nil, nil
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, nil
	}

	// userName is the SCIM attribute holding the login identifier, which in this
	// org is the person's email — the same value group membership reports.
	users, err := c.searchUsers(ctx, fmt.Sprintf(`userName eq %q`, email))
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

// LookupByUUID resolves an Asgardeo id to that person's directory record.
//
// This is the lookup the platform depends on once it no longer stores names:
// the `user` table keeps the uuid, and the name and email come from here.
//
// Same contract as LookupByEmail — (nil, nil) for an id the directory does not
// know, which happens for a deleted account, and the same org_internal:users:read
// scope requirement.
func (c *Client) LookupByUUID(ctx context.Context, uuid string) (*DirectoryUser, error) {
	if c == nil {
		return nil, nil
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, nil
	}

	users, err := c.searchUsers(ctx, fmt.Sprintf(`id eq %q`, uuid))
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

// searchUsers runs a caller-built SCIM filter against the Users-search
// endpoint, single-page. The filter string is forwarded to Asgardeo verbatim
// by the SCIM Operations Service, so its syntax — not this platform's — is
// what governs. Fine for LookupByEmail/LookupByUUID, an exact-match filter
// expected to return 0 or 1 result; ListUsersByDomain uses searchUsersPage
// directly instead, since a domain filter can span many pages.
func (c *Client) searchUsers(ctx context.Context, filter string) ([]DirectoryUser, error) {
	users, _, err := c.searchUsersPage(ctx, filter, 0, 0)
	return users, err
}

// searchUsersPage is one page of a Users-search call. startIndex/itemsPerPage
// are 1-indexed SCIM pagination; either left at 0 lets the server apply its
// own default (observed as 100). Returns the page's users and the search's
// totalResults, so a caller can decide whether to fetch another page.
func (c *Client) searchUsersPage(ctx context.Context, filter string, startIndex, itemsPerPage int) ([]DirectoryUser, int, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("get scim access token: %w", err)
	}

	body, err := json.Marshal(userSearchInput{
		Filter: filter,
		// Asking for only what is used keeps the response small and avoids
		// pulling attributes this platform has no business reading.
		Attributes:   []string{"id", "userName", "name"},
		StartIndex:   startIndex,
		ItemsPerPage: itemsPerPage,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal scim user search request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/organizations/internal/users/search", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build scim user search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("call scim user search: %w", err)
	}
	defer resp.Body.Close()

	// 201 as well as 200 — same non-standard success status the group search
	// returns, and for the same reason.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, 0, fmt.Errorf("scim user search returned status %d", resp.StatusCode)
	}

	var result userSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decode scim user search response: %w", err)
	}

	out := make([]DirectoryUser, 0, len(result.Resources))
	for _, u := range result.Resources {
		// stripGroupDomain on userName too: group membership reports it
		// userstore-prefixed, and whether a user resource does the same is not
		// worth depending on either way.
		out = append(out, DirectoryUser{
			UUID:        u.ID,
			Email:       stripGroupDomain(u.UserName),
			DisplayName: fullName(u.Name.GivenName, u.Name.FamilyName),
		})
	}
	return out, result.TotalResults, nil
}

// usersPageSize is the page size ListUsersByDomain requests. The service's
// own default happens to be 100 too (confirmed empirically — it ignored a
// smaller requested value), but this is requested explicitly rather than
// relying on that continuing to be true.
const usersPageSize = 100

// ListUsersByDomain returns every directory user whose userName ends with
// domain (pass e.g. "wso2.com", not "@wso2.com" — the filter already anchors
// on the suffix), walking every page.
//
// This is the bulk-fetch this platform's identity cache refreshes itself
// from — see internal/directory. It exists because the alternative, an
// unfiltered Users-search, returns everyone in this org: 300,000+ records the
// last time this was checked, the overwhelming majority load-test/synthetic
// accounts (user100035@external.com and so on) with a handful of real
// employees mixed in. There is no server-side "active" filter to narrow that
// with — `active eq true` was tried and returns zero results, so it is
// either unsupported or unpopulated in this deployment — domain-restricting
// to real employees' email suffix is what actually narrows it.
func (c *Client) ListUsersByDomain(ctx context.Context, domain string) ([]DirectoryUser, error) {
	if c == nil {
		return nil, nil
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, nil
	}
	filter := fmt.Sprintf(`userName ew %q`, domain)

	var all []DirectoryUser
	for startIndex := 1; ; startIndex += usersPageSize {
		page, total, err := c.searchUsersPage(ctx, filter, startIndex, usersPageSize)
		if err != nil {
			return nil, fmt.Errorf("list users by domain %q (from index %d): %w", domain, startIndex, err)
		}
		all = append(all, page...)
		if len(page) == 0 || startIndex+len(page) > total {
			break
		}
	}
	return all, nil
}

// fullName joins the SCIM name parts, tolerating either being absent.
func fullName(given, family string) string {
	return strings.TrimSpace(strings.TrimSpace(given) + " " + strings.TrimSpace(family))
}
