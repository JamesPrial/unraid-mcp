package graphql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/config"
)

// ---------------------------------------------------------------------------
// Compile-time interface satisfaction check
// ---------------------------------------------------------------------------

// Verify that HTTPClient satisfies the Client interface at compile time.
var _ Client = (*HTTPClient)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestConfig returns a GraphQLConfig pointing at the given URL with
// reasonable defaults for testing.
func newTestConfig(t *testing.T, url, apiKey string) config.GraphQLConfig {
	t.Helper()
	return config.GraphQLConfig{
		URL:     url,
		APIKey:  apiKey,
		Timeout: 5,
	}
}

// graphqlRequestBody is the expected shape of a GraphQL HTTP request body.
type graphqlRequestBody struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// ---------------------------------------------------------------------------
// normalizeURL tests
// ---------------------------------------------------------------------------

func Test_normalizeURL_Cases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bare host without trailing slash",
			input: "http://tower.local",
			want:  "http://tower.local/graphql",
		},
		{
			name:  "bare host with single trailing slash",
			input: "http://tower.local/",
			want:  "http://tower.local/graphql",
		},
		{
			name:  "already has graphql suffix",
			input: "http://tower.local/graphql",
			want:  "http://tower.local/graphql",
		},
		{
			name:  "graphql suffix with trailing slash",
			input: "http://tower.local/graphql/",
			want:  "http://tower.local/graphql",
		},
		{
			name:  "multiple trailing slashes",
			input: "http://tower.local///",
			want:  "http://tower.local/graphql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewHTTPClient tests
// ---------------------------------------------------------------------------

func Test_NewHTTPClient_Cases(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.GraphQLConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with URL and key",
			cfg: config.GraphQLConfig{
				URL:     "http://tower.local",
				APIKey:  "abc",
				Timeout: 30,
			},
			wantErr: false,
		},
		{
			name: "URL without graphql suffix",
			cfg: config.GraphQLConfig{
				URL:     "http://tower.local",
				APIKey:  "key123",
				Timeout: 10,
			},
			wantErr: false,
		},
		{
			name: "empty URL returns error",
			cfg: config.GraphQLConfig{
				URL:     "",
				APIKey:  "abc",
				Timeout: 30,
			},
			wantErr: true,
			errMsg:  "URL is required",
		},
		{
			name: "zero timeout uses default",
			cfg: config.GraphQLConfig{
				URL:     "http://tower.local",
				APIKey:  "abc",
				Timeout: 0,
			},
			wantErr: false,
		},
		{
			name: "negative timeout uses default",
			cfg: config.GraphQLConfig{
				URL:     "http://tower.local",
				APIKey:  "abc",
				Timeout: -5,
			},
			wantErr: false,
		},
		{
			name: "empty API key succeeds at construction time",
			cfg: config.GraphQLConfig{
				URL:     "http://tower.local",
				APIKey:  "",
				Timeout: 30,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewHTTPClient(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
				if client != nil {
					t.Error("expected nil client on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("expected non-nil client")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — happy path
// ---------------------------------------------------------------------------

func Test_Execute_HappyPath(t *testing.T) {
	responseData := `{"data":{"info":{"hostname":"tower"}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseData))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	query := `query { info { hostname } }`
	result, err := client.Execute(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// The result should contain the data portion as JSON bytes.
	resultStr := string(result)
	if !strings.Contains(resultStr, "hostname") {
		t.Errorf("result = %q, expected it to contain 'hostname'", resultStr)
	}
	if !strings.Contains(resultStr, "tower") {
		t.Errorf("result = %q, expected it to contain 'tower'", resultStr)
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — request body verification
// ---------------------------------------------------------------------------

func Test_Execute_QueryWithVariables(t *testing.T) {
	var receivedBody graphqlRequestBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &receivedBody); err != nil {
			http.Error(w, "failed to parse body", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"result":"ok"}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	query := `query GetInfo($id: ID!) { info(id: $id) { name } }`
	variables := map[string]any{
		"id": "abc-123",
	}

	_, err = client.Execute(context.Background(), query, variables)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the query was sent correctly.
	if receivedBody.Query != query {
		t.Errorf("request query = %q, want %q", receivedBody.Query, query)
	}

	// Verify variables were sent.
	if receivedBody.Variables == nil {
		t.Fatal("expected variables in request body, got nil")
	}
	if id, ok := receivedBody.Variables["id"]; !ok {
		t.Error("expected 'id' in variables")
	} else if id != "abc-123" {
		t.Errorf("variables['id'] = %v, want %q", id, "abc-123")
	}
}

func Test_Execute_NilVariables(t *testing.T) {
	var receivedRawBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedRawBody, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"result":"ok"}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	query := `query { info { hostname } }`
	_, err = client.Execute(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify the request body is valid JSON and contains the query.
	var bodyMap map[string]any
	if err := json.Unmarshal(receivedRawBody, &bodyMap); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if q, ok := bodyMap["query"]; !ok {
		t.Error("expected 'query' in request body")
	} else if q != query {
		t.Errorf("request query = %v, want %q", q, query)
	}

	// Variables should be omitted or null.
	if vars, ok := bodyMap["variables"]; ok && vars != nil {
		t.Errorf("expected variables to be omitted or null, got %v", vars)
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — API key header
// ---------------------------------------------------------------------------

func Test_Execute_APIKeyHeader(t *testing.T) {
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"result":"ok"}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify x-api-key header is set.
	apiKey := receivedHeaders.Get("x-api-key")
	if apiKey != "test-key" {
		t.Errorf("x-api-key header = %q, want %q", apiKey, "test-key")
	}

	// Verify Content-Type header is set to application/json.
	contentType := receivedHeaders.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want it to contain 'application/json'", contentType)
	}
}

func Test_Execute_EmptyAPIKey_ReturnsError(t *testing.T) {
	// The server should NOT be contacted when the API key is empty.
	serverCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	cfg := config.GraphQLConfig{
		URL:     srv.URL,
		APIKey:  "",
		Timeout: 5,
	}
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "api key is not configured") {
		t.Errorf("error = %q, want it to contain 'api key is not configured'", err.Error())
	}

	if serverCalled {
		t.Error("server should not have been called when API key is empty")
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — HTTP error responses
// ---------------------------------------------------------------------------

func Test_Execute_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "bad-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "authentication failed") {
		t.Errorf("error = %q, want it to contain 'authentication failed'", err.Error())
	}
}

func Test_Execute_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal server error`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected HTTP status 500") {
		t.Errorf("error = %q, want it to contain 'unexpected HTTP status 500'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — GraphQL errors
// ---------------------------------------------------------------------------

func Test_Execute_GraphQLSingleError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"field not found"}]}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { nonexistent { field } }`, nil)
	if err == nil {
		t.Fatal("expected error for GraphQL error response, got nil")
	}
	if !strings.Contains(err.Error(), "field not found") {
		t.Errorf("error = %q, want it to contain 'field not found'", err.Error())
	}
}

func Test_Execute_GraphQLMultipleErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{"data":null,"errors":[{"message":"first error"},{"message":"second error"}]}`
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { bad }`, nil)
	if err == nil {
		t.Fatal("expected error for GraphQL multiple errors, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "first error") {
		t.Errorf("error = %q, want it to contain 'first error'", errStr)
	}
	if !strings.Contains(errStr, "second error") {
		t.Errorf("error = %q, want it to contain 'second error'", errStr)
	}
	// Errors should be joined with "; ".
	if !strings.Contains(errStr, "; ") {
		t.Errorf("error = %q, expected errors joined by '; '", errStr)
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — context cancellation and deadline
// ---------------------------------------------------------------------------

func Test_Execute_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response that should be interrupted.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err = client.Execute(ctx, `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("error = %q, want it to reference context cancellation", err.Error())
	}
}

func Test_Execute_ContextDeadlineExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Allow the timeout to expire.
	time.Sleep(5 * time.Millisecond)

	_, err = client.Execute(ctx, `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error with deadline exceeded, got nil")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "deadline") && !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "canceled") {
		t.Errorf("error = %q, want it to reference deadline/timeout", err.Error())
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — malformed response
// ---------------------------------------------------------------------------

func Test_Execute_MalformedJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "decode response") {
		t.Errorf("error = %q, want it to contain 'decode response'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — connection refused
// ---------------------------------------------------------------------------

func Test_Execute_ConnectionRefused(t *testing.T) {
	// Use a URL pointing to a port that is not listening.
	// We start a server, get its URL, then close it to guarantee the port is free.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	cfg := newTestConfig(t, closedURL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "request failed") {
		t.Errorf("error = %q, want it to contain 'request failed'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — concurrent requests
// ---------------------------------------------------------------------------

func Test_Execute_ConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"info":{"hostname":"tower"}}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make([]error, goroutines)
	results := make([][]byte, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			result, err := client.Execute(context.Background(), `query { info { hostname } }`, nil)
			errors[idx] = err
			results[idx] = result
		}(i)
	}

	wg.Wait()

	for i := 0; i < goroutines; i++ {
		if errors[i] != nil {
			t.Errorf("goroutine %d error: %v", i, errors[i])
		}
		if results[i] == nil {
			t.Errorf("goroutine %d returned nil result", i)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if requestCount != goroutines {
		t.Errorf("server received %d requests, want %d", requestCount, goroutines)
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — request method and content type
// ---------------------------------------------------------------------------

func Test_Execute_RequestMethod(t *testing.T) {
	var receivedMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"result":"ok"}}`))
	}))
	defer srv.Close()

	cfg := newTestConfig(t, srv.URL, "test-key")
	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	_, err = client.Execute(context.Background(), `query { info { hostname } }`, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("request method = %q, want %q", receivedMethod, http.MethodPost)
	}
}

// ---------------------------------------------------------------------------
// HTTPClient.Execute tests — HTTP status code table-driven
// ---------------------------------------------------------------------------

func Test_Execute_HTTPStatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		wantErr     bool
		errContains string
	}{
		{
			name:       "200 OK with valid data succeeds",
			statusCode: http.StatusOK,
			body:       `{"data":{"info":{"hostname":"tower"}}}`,
			wantErr:    false,
		},
		{
			name:        "401 Unauthorized returns auth error",
			statusCode:  http.StatusUnauthorized,
			body:        `{"error":"unauthorized"}`,
			wantErr:     true,
			errContains: "authentication failed",
		},
		{
			name:        "403 Forbidden returns error",
			statusCode:  http.StatusForbidden,
			body:        `{"error":"forbidden"}`,
			wantErr:     true,
			errContains: "403",
		},
		{
			name:        "500 Internal Server Error",
			statusCode:  http.StatusInternalServerError,
			body:        `internal server error`,
			wantErr:     true,
			errContains: "unexpected HTTP status 500",
		},
		{
			name:        "502 Bad Gateway",
			statusCode:  http.StatusBadGateway,
			body:        `bad gateway`,
			wantErr:     true,
			errContains: "502",
		},
		{
			name:        "503 Service Unavailable",
			statusCode:  http.StatusServiceUnavailable,
			body:        `service unavailable`,
			wantErr:     true,
			errContains: "503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			cfg := newTestConfig(t, srv.URL, "test-key")
			client, err := NewHTTPClient(cfg)
			if err != nil {
				t.Fatalf("NewHTTPClient: %v", err)
			}

			result, err := client.Execute(context.Background(), `query { info }`, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GraphQLError type tests
// ---------------------------------------------------------------------------

func Test_GraphQLError_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{
			name:    "standard error message",
			input:   `{"message":"field not found"}`,
			wantMsg: "field not found",
		},
		{
			name:    "empty message",
			input:   `{"message":""}`,
			wantMsg: "",
		},
		{
			name:    "missing message field",
			input:   `{}`,
			wantMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gqlErr GraphQLError
			if err := json.Unmarshal([]byte(tt.input), &gqlErr); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if gqlErr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", gqlErr.Message, tt.wantMsg)
			}
		})
	}
}

func Test_GraphQLError_JSONMarshal(t *testing.T) {
	gqlErr := GraphQLError{Message: "test error"}
	data, err := json.Marshal(gqlErr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var roundTrip GraphQLError
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal roundtrip: %v", err)
	}
	if roundTrip.Message != gqlErr.Message {
		t.Errorf("roundtrip Message = %q, want %q", roundTrip.Message, gqlErr.Message)
	}
}

func Test_GraphQLError_ZeroValue(t *testing.T) {
	var gqlErr GraphQLError
	if gqlErr.Message != "" {
		t.Errorf("zero value Message = %q, want empty", gqlErr.Message)
	}
}

// ---------------------------------------------------------------------------
// Client interface type tests
// ---------------------------------------------------------------------------

func Test_Client_InterfaceHasExecuteMethod(t *testing.T) {
	// This test verifies the Client interface contract at a type level.
	// The compile-time check (var _ Client = (*HTTPClient)(nil)) above
	// is the primary guard; this test documents the expectation.
	var c Client
	if c != nil {
		t.Error("nil interface value should be nil")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func Benchmark_Execute_HappyPath(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"info":{"hostname":"tower"}}}`))
	}))
	defer srv.Close()

	cfg := config.GraphQLConfig{
		URL:     srv.URL,
		APIKey:  "bench-key",
		Timeout: 5,
	}
	client, err := NewHTTPClient(cfg)
	if err != nil {
		b.Fatalf("NewHTTPClient: %v", err)
	}

	ctx := context.Background()
	query := `query { info { hostname } }`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.Execute(ctx, query, nil)
	}
}

func Benchmark_Execute_WithVariables(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"result":"ok"}}`))
	}))
	defer srv.Close()

	cfg := config.GraphQLConfig{
		URL:     srv.URL,
		APIKey:  "bench-key",
		Timeout: 5,
	}
	client, err := NewHTTPClient(cfg)
	if err != nil {
		b.Fatalf("NewHTTPClient: %v", err)
	}

	ctx := context.Background()
	query := `query GetInfo($id: ID!) { info(id: $id) { name } }`
	variables := map[string]any{"id": "abc-123"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.Execute(ctx, query, variables)
	}
}

func Benchmark_NewHTTPClient(b *testing.B) {
	cfg := config.GraphQLConfig{
		URL:     "http://tower.local",
		APIKey:  "bench-key",
		Timeout: 30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewHTTPClient(cfg)
	}
}
