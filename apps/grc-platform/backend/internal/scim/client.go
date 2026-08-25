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

// Package scim is a client for Asgardeo's own SCIM2 API, used to resolve
// identity between a person's email and their Asgardeo user id (uuid) — the
// only identifier the `user` table stores (see shared.sql). It calls
// Asgardeo directly (POST /t/{org}/scim2/Users/.search, authenticating
// against /t/{org}/oauth2/token) rather than going through digiops-infra's
// SCIM Operations Service. It never modifies Asgardeo; it only calls the
// Users-search endpoint.
package scim

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// maxErrorBodyBytes caps how much of an error response body gets read into
// an error message — enough for Asgardeo's SCIM/OAuth2 error payloads (a
// short JSON object), not so much that a misbehaving proxy returning an
// enormous body could bloat a log line.
const maxErrorBodyBytes = 2048

// readErrorBody best-effort reads and trims resp.Body for inclusion in an
// error message. Asgardeo's non-2xx responses carry a JSON body naming the
// actual problem (e.g. "invalid_client", "unauthorized_client", a SCIM
// "detail" string) that the plain status code alone doesn't distinguish —
// a 403 for "wrong scope" and a 403 for "wrong org" look identical without
// it. Never itself errors: a body read failure degrades to an empty string
// rather than masking the original status-code error.
func readErrorBody(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	return strings.TrimSpace(string(b))
}

// Client talks to Asgardeo's SCIM2 API for one Asgardeo organization —
// "internal" (org wso2) or "external" (org wso2external), picked at
// construction by NewClient vs NewExternalClient and fixed for the Client's
// lifetime. Internal users and external auditors live in genuinely separate
// Asgardeo organizations, each with its own OAuth2 app registration, so
// there is no single set of credentials or endpoint that covers both — every
// method needs a Client built for the right org.
type Client struct {
	baseURL      string
	tokenURL     string
	clientID     string
	clientSecret string
	// scopes is a space-separated OAuth2 scope string requested on every token
	// exchange — every method on this client needs internal_user_mgt_view and
	// internal_user_mgt_list. Asgardeo silently omits any scope the
	// application isn't authorized for from the issued token rather than
	// erroring, so an empty/wrong value here surfaces later as a 403 from
	// Asgardeo's SCIM2 API at call time, not as a token-request failure.
	scopes string
	// org is the Asgardeo tenant path segment this client's requests target
	// (e.g. "wso2" or "wso2external"), used to build both the SCIM2 base
	// (/t/{org}/scim2) and the token endpoint (/t/{org}/oauth2/token).
	org string
	// tokenBindingID is a random identifier generated once per Client and
	// sent as an optional param on every token request. Asgardeo revokes a
	// client's previously issued token when it issues a new one for the same
	// binding, so without a stable-per-instance binding id, concurrent
	// server processes/replicas sharing one Asgardeo app would keep
	// invalidating each other's cached tokens. Mirrors
	// scim-operations-service/modules/scim/client.bal, which generates one
	// via uuid:createRandomUuid() per http:Client it builds.
	tokenBindingID string
	httpClient     *http.Client

	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// tokenExpiryBuffer is subtracted from the token's reported lifetime so a
// near-expiry token is never handed to an in-flight request.
const tokenExpiryBuffer = 30 * time.Second

// NewClient creates a Client for Asgardeo's internal-org SCIM2 API at
// baseURL (the Asgardeo API root, e.g. https://api.asgardeo.io), targeting
// tenant org (e.g. "wso2") and authenticating via OAuth2 client-credentials
// at tokenURL. scopes is a space-separated list requested on every token
// exchange (see Client.scopes).
func NewClient(baseURL, tokenURL, clientID, clientSecret, scopes, org string) *Client {
	return newClient(baseURL, tokenURL, clientID, clientSecret, scopes, org)
}

// NewExternalClient is NewClient for external auditors' identities — pass
// the external org's own tokenURL/clientID/clientSecret/scopes/org (a
// genuinely different Asgardeo app registration, not just a different scope
// string). ListUsersByDomain is still exposed (the type has no per-org
// method set) but has no real use here: the bulk-fetch shape it implements
// has no external-org equivalent to call against, so callers resolving an
// external uuid should use LookupByUUID.
func NewExternalClient(baseURL, tokenURL, clientID, clientSecret, scopes, org string) *Client {
	return newClient(baseURL, tokenURL, clientID, clientSecret, scopes, org)
}

func newClient(baseURL, tokenURL, clientID, clientSecret, scopes, org string) *Client {
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		tokenURL:       tokenURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		scopes:         scopes,
		org:            org,
		tokenBindingID: newTokenBindingID(),
		httpClient:     &http.Client{Timeout: 5 * time.Second},
	}
}

// newTokenBindingID generates a random RFC4122-ish v4 identifier, matching
// internal/middleware.newCorrelationID's approach — no external uuid
// dependency needed for a value that only has to be unique per process, not
// globally unique or spec-compliant.
func newTokenBindingID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("scim: failed to read random bytes for token binding id: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// SetHTTPTimeout replaces the per-request timeout. The 5s default suits the
// live request path, where a user is waiting and a slow directory should fail
// fast rather than hold the request open.
//
// Offline tools want the opposite: a first call can cold-start slower than
// any interactive budget, and a batch job that has no user waiting would
// rather wait than abort a whole migration run.
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

// accessToken returns a valid bearer token for Asgardeo's SCIM2 API, reusing
// the cached one until it's close to expiry.
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
	// See Client.tokenBindingID's doc comment: keeps this token request from
	// revoking a token another process/replica issued for the same Asgardeo
	// app.
	form.Set("tokenBindingId", c.tokenBindingID)

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
		return "", fmt.Errorf("scim token endpoint returned status %d: %s", resp.StatusCode, readErrorBody(resp))
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
	Schemas []string `json:"schemas"`
	Filter  string   `json:"filter"`
	// Attributes to return
	Attributes []string `json:"attributes,omitempty"`
	// StartIndex/ItemsPerPage are 1-indexed SCIM pagination — zero means
	// "unset" and is omitted, letting the server apply its own default
	// (observed to be 100, empirically, regardless of what's requested below
	// it — this platform requests it explicitly instead of relying on that).
	StartIndex   int `json:"startIndex,omitempty"`
	ItemsPerPage int `json:"itemsPerPage,omitempty"`
}

// scimSearchRequestSchema is the SCIM2 SearchRequest schema Asgardeo expects
// on the "schemas" field of a POST /Users/.search body.
const scimSearchRequestSchema = "urn:ietf:params:scim:api:messages:2.0:SearchRequest"

// scimUser is one entry in a Users-search result. Asgardeo's response
// carries far more attributes than these; only the two read here are
// declared, and the rest are ignored by encoding/json.
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

// SearchByQuery finds people whose userName (email), given name, or family
// name contains query — three live SCIM calls run concurrently, merged and
// de-duplicated by uuid (a person can match more than one attribute), capped
// at searchPageSize results overall.
//
// This is three calls, not one with a compound filter, because Asgardeo's
// SCIM2 API in this deployment rejects an "or" filter outright (observed:
// {"detail":"Unsupported Operation: or", ...} — a 500, not a 4xx, so there's
// no way to detect this ahead of trying) even though RFC 7644 requires
// logical operator support. Splitting into per-attribute queries and merging
// here is what stands in for the "or" Asgardeo won't do itself.
//
// This is the external-org counterpart to directory.Service.SearchDomain,
// which searches a bulk snapshot instead: the external org has no
// ListUsersByDomain equivalent to build one from (NewExternalClient's doc
// comment), so every external search hits Asgardeo directly. Fine for an
// admin-only, low-frequency lookup (the Add User dialog's External search),
// same reasoning as LookupByEmail/LookupByUUID — not worth a caching layer
// for how rarely this is called. Three concurrent calls instead of one still
// costs one round trip's worth of latency, not three's.
//
// %q quotes and escapes query the same way LookupByEmail's filter does,
// which is what keeps a query containing a literal `"` from breaking out of
// the SCIM filter string.
func (c *Client) SearchByQuery(ctx context.Context, query string) ([]DirectoryUser, error) {
	if c == nil {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	filters := []string{
		fmt.Sprintf(`userName co %q`, query),
		fmt.Sprintf(`name.givenName co %q`, query),
		fmt.Sprintf(`name.familyName co %q`, query),
	}
	type pageResult struct {
		users []DirectoryUser
		err   error
	}
	results := make([]pageResult, len(filters))
	var wg sync.WaitGroup
	for i, filter := range filters {
		wg.Add(1)
		go func(i int, filter string) {
			defer wg.Done()
			users, _, err := c.searchUsersPage(ctx, filter, 0, searchPageSize)
			results[i] = pageResult{users: users, err: err}
		}(i, filter)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("scim search: %w", r.err)
		}
	}

	// Round-robin across the three filters' result lists rather than
	// appending one fully before the next and truncating at the end: each
	// filter can independently return up to searchPageSize matches, so a
	// straight concatenation lets whichever filter is processed first (here,
	// userName) crowd out every match from the other two before the cap is
	// even reached — someone findable only by surname would never appear.
	seen := make(map[string]bool, searchPageSize)
	out := make([]DirectoryUser, 0, searchPageSize)
	for j := 0; len(out) < searchPageSize; j++ {
		any := false
		for _, r := range results {
			if j >= len(r.users) {
				continue
			}
			any = true
			u := r.users[j]
			if seen[u.UUID] {
				continue
			}
			seen[u.UUID] = true
			out = append(out, u)
			if len(out) == searchPageSize {
				break
			}
		}
		if !any {
			break // every filter's result list is exhausted
		}
	}
	return out, nil
}

// searchPageSize caps SearchByQuery's single page — an admin typing a query
// into the Add User dialog needs a short, scannable list, not every match.
const searchPageSize = 20

// LookupByEmail resolves an email to that person's directory record — their
// Asgardeo id and display name.
//
// Requires the internal_user_mgt_view and internal_user_mgt_list scopes.
// Asgardeo silently omits a scope the application is not authorised for
// rather than failing the token request, so a missing grant surfaces here as
// a 403 from Asgardeo's SCIM2 API at call time, not as a configuration error
// at startup.
//
// Returns (nil, nil) when the directory knows no such user. That is an ordinary
// answer, not a failure: an employee may be assignable in HR long before they
// have an Asgardeo account, and the caller decides what to do about it.
// A nil Client means the directory is not configured — local development
// without Asgardeo credentials. It answers "no such user" rather than
// panicking, so callers written to tolerate an unknown person behave the
// same way whether the directory is absent or merely unaware of them.
// Callers that must not proceed without a real answer have to check for a
// nil client themselves; none do today.
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
// know, which happens for a deleted account, and the same
// internal_user_mgt_view/internal_user_mgt_list scope requirement.
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

// searchUsers runs a caller-built SCIM filter against Asgardeo's
// Users-search endpoint, single-page. The filter string is forwarded to
// Asgardeo verbatim, so its syntax is what governs. Fine for
// LookupByEmail/LookupByUUID, an exact-match filter expected to return 0 or
// 1 result; ListUsersByDomain uses searchUsersPage directly instead, since a
// domain filter can span many pages.
func (c *Client) searchUsers(ctx context.Context, filter string) ([]DirectoryUser, error) {
	users, _, err := c.searchUsersPage(ctx, filter, 0, 0)
	return users, err
}

// searchUsersPage is one page of a Users-search call against
// /t/{org}/scim2/Users/.search. startIndex/itemsPerPage are 1-indexed SCIM
// pagination; either left at 0 lets the server apply its own default
// (observed as 100). Returns the page's users and the search's totalResults,
// so a caller can decide whether to fetch another page.
func (c *Client) searchUsersPage(ctx context.Context, filter string, startIndex, itemsPerPage int) ([]DirectoryUser, int, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("get scim access token: %w", err)
	}

	body, err := json.Marshal(userSearchInput{
		Schemas: []string{scimSearchRequestSchema},
		Filter:  filter,
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
		c.baseURL+"/t/"+c.org+"/scim2/Users/.search", bytes.NewReader(body))
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

	// 201 as well as 200: not documented as a possible SCIM2 search response
	// from Asgardeo, but accepted defensively since it costs nothing and the
	// service this replaced (scim-operations-service) was observed to return
	// it.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, 0, fmt.Errorf("scim user search returned status %d: %s", resp.StatusCode, readErrorBody(resp))
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
// @domain (pass e.g. "wso2.com", not "@wso2.com" — the "@" is added here so
// the filter anchors on an actual domain boundary; without it, "ew" is a
// plain string suffix match and "wso2.com" would also match a userName like
// person@notwso2.com), walking every page.
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
	filter := fmt.Sprintf(`userName ew %q`, "@"+domain)

	var all []DirectoryUser
	// Advances by len(page), not usersPageSize: the service is only observed
	// to ignore a smaller requested size (see usersPageSize's comment), not
	// guaranteed to always return exactly what was asked for. Advancing by
	// the fixed request size on a page that came back short would skip every
	// record between the short page's end and the next requested startIndex.
	for startIndex := 1; ; {
		page, total, err := c.searchUsersPage(ctx, filter, startIndex, usersPageSize)
		if err != nil {
			return nil, fmt.Errorf("list users by domain %q (from index %d): %w", domain, startIndex, err)
		}
		all = append(all, page...)
		if len(page) == 0 || startIndex+len(page) > total {
			break
		}
		startIndex += len(page)
	}
	return all, nil
}

// fullName joins the SCIM name parts, tolerating either being absent.
func fullName(given, family string) string {
	return strings.TrimSpace(strings.TrimSpace(given) + " " + strings.TrimSpace(family))
}
