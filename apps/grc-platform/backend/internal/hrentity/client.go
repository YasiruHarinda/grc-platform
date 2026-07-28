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

// Package hrentity is a read-only client for the WSO2 HR entity GraphQL
// service (hr_entity). It is used to look up employees for the Risk module's
// "Risk Identified By: Employee" field — employee data is never stored in
// the GRC platform's own database, only fetched live per search.
package hrentity

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

// Employee is the subset of hr_entity's Employee fields the Risk module needs.
type Employee struct {
	FirstName string
	LastName  string
	WorkEmail string
	Thumbnail string
}

// Client talks to the hr_entity GraphQL service. Pointed at a local mock
// server during development and the real Choreo-hosted service in
// production — only the URL (and OAuth2 credentials) change, the
// request/response contract is identical either way.
//
// The real service sits behind Choreo API Management with OAuth2
// client-credentials auth, so the client fetches and caches its own bearer
// token, refreshing it once it's within tokenExpiryBuffer of expiring.
type Client struct {
	graphqlURL   string
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// tokenExpiryBuffer is subtracted from the token's reported lifetime so a
// near-expiry token is never handed to an in-flight request.
const tokenExpiryBuffer = 30 * time.Second

// NewClient creates a Client for the hr_entity service at graphqlURL,
// authenticating via OAuth2 client-credentials at tokenURL using clientID
// and clientSecret.
func NewClient(graphqlURL, tokenURL, clientID, clientSecret string) *Client {
	return &Client{
		graphqlURL:   graphqlURL,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// accessToken returns a valid bearer token for the hr_entity service,
// reusing the cached one until it's close to expiry.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build hr entity token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call hr entity token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hr entity token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode hr entity token response: %w", err)
	}

	c.cachedToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenExpiryBuffer)
	return c.cachedToken, nil
}

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// graphqlEnvelope is the outer shape of every hr_entity GraphQL response.
// Data is left raw since its inner shape differs per query.
type graphqlEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// doQuery executes a GraphQL query against hr_entity, handling token
// attachment, the HTTP round trip, and envelope-level error checking. The
// caller unmarshals the returned raw Data into whatever shape their query's
// response has.
func (c *Client) doQuery(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("marshal hr entity request: %w", err)
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get hr entity access token: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build hr entity request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call hr entity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hr entity returned status %d", resp.StatusCode)
	}

	var envelope graphqlEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode hr entity response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return nil, fmt.Errorf("hr entity returned errors: %s", envelope.Errors[0].Message)
	}
	return envelope.Data, nil
}

const employeesQuery = `
query SearchEmployees($filter: EmployeeFilter, $limit: Int) {
	employees(filter: $filter, limit: $limit) {
		firstName
		lastName
		workEmail
		employeeThumbnail
	}
}`

// SearchActiveEmployees returns active WSO2 employees whose work email
// contains emailSearchString, capped at limit results. hr_entity's
// EmployeeFilter has no name-search field, so matching is by email only.
func (c *Client) SearchActiveEmployees(ctx context.Context, emailSearchString string, limit int) ([]Employee, error) {
	data, err := c.doQuery(ctx, employeesQuery, map[string]any{
		"filter": map[string]any{
			"emailSearchString": emailSearchString,
			"employeeStatus":    []string{"Active"},
		},
		"limit": limit,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Employees []struct {
			FirstName *string `json:"firstName"`
			LastName  *string `json:"lastName"`
			WorkEmail *string `json:"workEmail"`
		} `json:"employees"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode hr entity employees: %w", err)
	}

	employees := make([]Employee, 0, len(result.Employees))
	for _, e := range result.Employees {
		var emp Employee
		if e.FirstName != nil {
			emp.FirstName = *e.FirstName
		}
		if e.LastName != nil {
			emp.LastName = *e.LastName
		}
		if e.WorkEmail != nil {
			emp.WorkEmail = *e.WorkEmail
		}
		employees = append(employees, emp)
	}
	return employees, nil
}

// employeeByEmailSearchLimit is small: emailSearchString is an exact work
// email, so more than a couple of substring matches would be unexpected.
const employeeByEmailSearchLimit = 5

// GetEmployeeByEmail looks up a single active employee's name and profile
// photo by their exact work email. Used to show the signed-in user's own
// name/avatar in the account menu (Asgardeo's ID token/userinfo don't carry
// those claims for this org's application) and, via the risk module, to
// verify an "Identified By: Employee" or Action Owner email actually
// belongs to a current employee before attributing anything to it.
//
// Reuses the same employees(filter:) query and Active-only employeeStatus
// filter as SearchActiveEmployees, rather than hr_entity's employee(email:)
// resolver, which has no status filter of its own — an email belonging to
// a former employee would otherwise still resolve. Returns (nil, nil) if no
// active employee matches.
func (c *Client) GetEmployeeByEmail(ctx context.Context, email string) (*Employee, error) {
	data, err := c.doQuery(ctx, employeesQuery, map[string]any{
		"filter": map[string]any{
			"emailSearchString": email,
			"employeeStatus":    []string{"Active"},
		},
		"limit": employeeByEmailSearchLimit,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Employees []struct {
			FirstName         *string `json:"firstName"`
			LastName          *string `json:"lastName"`
			WorkEmail         *string `json:"workEmail"`
			EmployeeThumbnail *string `json:"employeeThumbnail"`
		} `json:"employees"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode hr entity employees: %w", err)
	}

	for _, e := range result.Employees {
		if e.WorkEmail == nil || !strings.EqualFold(*e.WorkEmail, email) {
			continue
		}
		emp := &Employee{WorkEmail: *e.WorkEmail}
		if e.FirstName != nil {
			emp.FirstName = *e.FirstName
		}
		if e.LastName != nil {
			emp.LastName = *e.LastName
		}
		if e.EmployeeThumbnail != nil {
			emp.Thumbnail = *e.EmployeeThumbnail
		}
		return emp, nil
	}
	return nil, nil
}
