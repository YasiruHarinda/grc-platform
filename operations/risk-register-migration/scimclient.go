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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SCIMClient talks to Asgardeo's SCIM2 API for one org via an OAuth2
// client-credentials grant — the same endpoints and grant that
// apps/grc-platform/backend/internal/scim.Client and cmd/backfill-uuids use
// (token at {baseURL}/t/{org}/oauth2/token, search at
// {baseURL}/t/{org}/scim2/Users/.search). It is only ever used to pull the
// internal-org snapshot (ListUsersByDomain); this tool never writes to SCIM.
type SCIMClient struct {
	baseURL      string // Asgardeo API root, e.g. https://api.asgardeo.io (no trailing slash, no /t/{org})
	tokenURL     string // {baseURL}/t/{org}/oauth2/token
	clientID     string
	clientSecret string
	scopes       string // space-separated; Asgardeo silently drops unauthorised scopes → 403 at call time, not here
	org          string // tenant path segment, e.g. "wso2"
	http         *http.Client

	// tokenBindingID is generated once per client and sent on every token
	// request. Asgardeo revokes a client's previous token when it issues a new
	// one for the same binding, so without a stable id this tool's token
	// request would revoke the running server's token when they share one
	// Asgardeo app. Mirrors internal/scim.Client.tokenBindingID.
	tokenBindingID string

	tok    string
	tokExp time.Time
}

func NewSCIMClient(baseURL, tokenURL, clientID, clientSecret, scopes, org string, timeout time.Duration) *SCIMClient {
	return &SCIMClient{
		baseURL:        strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		tokenURL:       strings.TrimSpace(tokenURL),
		clientID:       clientID,
		clientSecret:   clientSecret,
		scopes:         scopes,
		org:            strings.TrimSpace(org),
		http:           &http.Client{Timeout: timeout},
		tokenBindingID: newTokenBindingID(),
	}
}

// newTokenBindingID is a random v4-ish identifier — unique per process is all
// it needs to be. Mirrors internal/scim.newTokenBindingID.
func newTokenBindingID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("risk-register-import: read random bytes for token binding id: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// tokenExpiryBuffer is subtracted from the reported lifetime so a near-expiry
// token is never handed to an in-flight request.
const tokenExpiryBuffer = 30 * time.Second

type scimTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// token returns a valid bearer token, reusing the cached one until it nears
// expiry.
func (c *SCIMClient) token(ctx context.Context) (string, error) {
	if c.tok != "" && time.Now().Before(c.tokExp) {
		return c.tok, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if c.scopes != "" {
		form.Set("scope", c.scopes)
	}
	form.Set("tokenBindingId", c.tokenBindingID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build scim token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("call scim token endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scim token endpoint returned status %d: %s", resp.StatusCode, readBodyForError(resp.Body))
	}

	var tr scimTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode scim token response: %w", err)
	}

	c.tok = tr.AccessToken
	c.tokExp = time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - tokenExpiryBuffer)
	return c.tok, nil
}

// DirectoryUser is one entry of the org snapshot.
type DirectoryUser struct {
	UUID        string
	Email       string
	DisplayName string
}

const scimSearchRequestSchema = "urn:ietf:params:scim:api:messages:2.0:SearchRequest"

type scimUserSearchInput struct {
	Schemas      []string `json:"schemas"`
	Filter       string   `json:"filter"`
	Domain       string   `json:"domain"`
	Attributes   []string `json:"attributes,omitempty"`
	StartIndex   int      `json:"startIndex,omitempty"`
	ItemsPerPage int      `json:"itemsPerPage,omitempty"`
}

type scimUser struct {
	ID       string `json:"id"`
	UserName string `json:"userName"`
	Name     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
}

type scimUserSearchResult struct {
	TotalResults int        `json:"totalResults"`
	Resources    []scimUser `json:"Resources"`
}

// scimUsersPageSize is what each page requests. internal/scim notes the service
// caps at ~100 regardless, but requests it explicitly rather than relying on
// that.
const scimUsersPageSize = 100

// ListUsersByDomain returns every directory user whose userName (email) ends
// with @domain — pass "wso2.com", not "@wso2.com" (the "@" is added here so the
// suffix anchors on a domain boundary). It walks every page. This is the sole
// identity source for the importer (plan §6).
func (c *SCIMClient) ListUsersByDomain(ctx context.Context, domain string) ([]DirectoryUser, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("scim: empty domain")
	}
	filter := fmt.Sprintf(`userName ew %q`, "@"+domain)

	var all []DirectoryUser
	// Advance by len(page), not the requested size: a short page must not make
	// the next startIndex skip the records between its end and that index.
	for startIndex := 1; ; {
		page, total, err := c.searchUsersPage(ctx, filter, startIndex, scimUsersPageSize)
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

// searchUsersPage runs one page of POST {baseURL}/t/{org}/scim2/Users/.search.
// startIndex is 1-indexed SCIM pagination. Returns the page plus the search's
// totalResults so the caller can decide whether to fetch another.
func (c *SCIMClient) searchUsersPage(ctx context.Context, filter string, startIndex, itemsPerPage int) ([]DirectoryUser, int, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("get scim access token: %w", err)
	}

	body, err := json.Marshal(scimUserSearchInput{
		Schemas:      []string{scimSearchRequestSchema},
		Filter:       filter,
		Domain:       "DEFAULT",
		Attributes:   []string{"id", "userName", "name"},
		StartIndex:   startIndex,
		ItemsPerPage: itemsPerPage,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal scim user search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/t/"+c.org+"/scim2/Users/.search", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build scim user search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("call scim user search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 201 accepted defensively alongside 200 — the service internal/scim
	// replaced was observed to return it.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, 0, fmt.Errorf("scim user search returned status %d: %s", resp.StatusCode, readBodyForError(resp.Body))
	}

	var result scimUserSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decode scim user search response: %w", err)
	}

	out := make([]DirectoryUser, 0, len(result.Resources))
	for _, u := range result.Resources {
		out = append(out, DirectoryUser{
			UUID:        u.ID,
			Email:       stripUserstoreDomain(u.UserName),
			DisplayName: joinName(u.Name.GivenName, u.Name.FamilyName),
		})
	}
	return out, result.TotalResults, nil
}

// stripUserstoreDomain removes a "DEFAULT/" style userstore prefix SCIM entries
// can carry (e.g. "DEFAULT/user1@wso2.com" -> "user1@wso2.com"); falls back to
// the raw value when there is no "/".
func stripUserstoreDomain(s string) string {
	if i := strings.LastIndex(s, "/"); i != -1 {
		return s[i+1:]
	}
	return s
}

func joinName(given, family string) string {
	return strings.TrimSpace(strings.TrimSpace(given) + " " + strings.TrimSpace(family))
}

// readBodyForError best-effort reads a capped chunk of an error response body
// for the error message; never itself errors.
func readBodyForError(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 2048))
	return strings.TrimSpace(string(b))
}
