// Package graphql provides a GraphQL HTTP client for communicating with the
// Unraid GraphQL API.
package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/config"
)

const defaultTimeout = 30 * time.Second

// HTTPClient is a concrete implementation of the Client interface that sends
// GraphQL requests over HTTP using the standard library net/http package.
type HTTPClient struct {
	httpClient *http.Client
	graphqlURL string
	apiKey     string
}

// NewHTTPClient constructs an HTTPClient from the provided GraphQLConfig.
// It returns an error if cfg.URL is empty. When cfg.Timeout is zero or
// negative, a default timeout of 30 seconds is used. An empty API key is
// accepted at construction time but will cause Execute to return an error.
func NewHTTPClient(cfg config.GraphQLConfig) (*HTTPClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("graphql: URL is required")
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if cfg.Timeout <= 0 {
		timeout = defaultTimeout
	}

	graphqlURL := normalizeURL(cfg.URL)

	return &HTTPClient{
		httpClient: &http.Client{Timeout: timeout},
		graphqlURL: graphqlURL,
		apiKey:     cfg.APIKey,
	}, nil
}

// normalizeURL trims any trailing slash from rawURL and appends /graphql if
// the path does not already end with that suffix.
func normalizeURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if !strings.HasSuffix(u, "/graphql") {
		u += "/graphql"
	}
	return u
}

// graphqlRequest is the JSON body shape for a GraphQL HTTP request.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlResponse is the JSON body shape for a GraphQL HTTP response.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors"`
}

// Execute sends a GraphQL query to the configured endpoint and returns the
// raw JSON bytes of the "data" field on success. Variables may be nil, in
// which case the "variables" key is omitted from the request body.
//
// Execute returns an error if:
//   - the client was constructed without an API key
//   - the HTTP request cannot be created or sent
//   - the server responds with a non-2xx status code
//   - the response body cannot be decoded as JSON
//   - the GraphQL response contains one or more errors
func (c *HTTPClient) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("graphql: API key is not configured")
	}

	reqBody := graphqlRequest{
		Query:     query,
		Variables: variables,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("graphql: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("graphql: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("graphql: authentication failed (HTTP 401)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("graphql: unexpected HTTP status %d", resp.StatusCode)
	}

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("graphql: decode response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("graphql: %s", strings.Join(msgs, "; "))
	}

	return []byte(gqlResp.Data), nil
}
