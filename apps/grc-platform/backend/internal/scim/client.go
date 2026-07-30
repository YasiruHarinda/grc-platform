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
// (digiops-infra/operations/scim-operations-service), used to answer "which
// users belong to Asgardeo group X" — a question this platform's own data
// cannot answer, since role assignment lives only in Asgardeo, never in the
// `user` table (see shared.sql). This client never modifies the SCIM
// Operations Service or Asgardeo itself; it only calls the service's existing
// generic group-search endpoint, reading each matched group's embedded
// member list.
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
	// scopes is a space-separated OAuth2 scope string requested on every
	// token exchange — only "org_internal:groups:read" is needed, since
	// ListGroupMemberEmails reads group membership via the groups-search
	// resource, not a users-search one. Asgardeo silently omits any scope the
	// application isn't authorized for from the issued token rather than
	// erroring, so an empty/wrong value here surfaces later as a 403
	// "Scope validation failed" from the SCIM Operations Service's gateway,
	// not as a token-request failure.
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

type groupSearchInput struct {
	Filter string `json:"filter"`
}

// scimGroupMember is one entry in a Group's embedded "members" list. Display
// comes back domain-prefixed (e.g. "DEFAULT/jane@wso2.com" for this org's
// default userstore) — stripGroupDomain removes that prefix to get a plain
// email.
type scimGroupMember struct {
	Display string `json:"display"`
	Value   string `json:"value"`
}

type scimGroup struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"displayName"`
	Members     []scimGroupMember `json:"members"`
}

type groupSearchResult struct {
	Resources []scimGroup `json:"Resources"`
}

// ListGroupMemberEmails returns the email of every member of the given
// Asgardeo group, via the SCIM Operations Service's generic internal-org
// group search endpoint (POST /organizations/internal/groups/search),
// reading the matched group's embedded "members" list directly — this
// service's Users-search endpoint requires a separate, more narrowly-granted
// scope (org_internal:users:read) that Groups-search does not.
func (c *Client) ListGroupMemberEmails(ctx context.Context, groupName string) ([]string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get scim access token: %w", err)
	}

	body, err := json.Marshal(groupSearchInput{
		Filter: fmt.Sprintf(`displayName eq %q`, groupName),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal scim group search request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/organizations/internal/groups/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build scim group search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call scim group search: %w", err)
	}
	defer resp.Body.Close()

	// This SCIM Operations Service returns 201 (not 200) for a successful
	// search POST, confirmed empirically against the live service — accept
	// both rather than assume standard REST conventions apply here.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("scim group search returned status %d", resp.StatusCode)
	}

	var result groupSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode scim group search response: %w", err)
	}

	var emails []string
	for _, g := range result.Resources {
		for _, m := range g.Members {
			if email := stripGroupDomain(m.Display); email != "" {
				emails = append(emails, email)
			}
		}
	}
	return emails, nil
}

// stripGroupDomain removes the userstore domain prefix SCIM group members
// come back with (e.g. "DEFAULT/jane@wso2.com" -> "jane@wso2.com"). Falls
// back to the raw value if there's no "/", rather than dropping it, so an
// unexpected format degrades to "probably wrong" instead of "silently gone."
func stripGroupDomain(display string) string {
	if i := strings.LastIndex(display, "/"); i != -1 {
		return display[i+1:]
	}
	return display
}
