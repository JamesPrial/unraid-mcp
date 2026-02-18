package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// dummyHandler returns a simple 200 OK handler used as the "next" handler in middleware tests.
func dummyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func Test_NewAuthMiddleware_Cases(t *testing.T) {
	const correctToken = "correct-token"

	tests := []struct {
		name           string
		configToken    string
		authHeader     string
		setAuthHeader  bool
		wantStatusCode int
	}{
		{
			name:           "valid token passes through",
			configToken:    correctToken,
			authHeader:     "Bearer correct-token",
			setAuthHeader:  true,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing header returns 401",
			configToken:    correctToken,
			authHeader:     "",
			setAuthHeader:  false,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "wrong token returns 401",
			configToken:    correctToken,
			authHeader:     "Bearer wrong-token",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "malformed header returns 401",
			configToken:    correctToken,
			authHeader:     "NotBearer token",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "empty token config disables auth - no header",
			configToken:    "",
			authHeader:     "",
			setAuthHeader:  false,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "empty token config disables auth - any header",
			configToken:    "",
			authHeader:     "Bearer anything",
			setAuthHeader:  true,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "Bearer with extra spaces returns 401",
			configToken:    correctToken,
			authHeader:     "Bearer  correct-token",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "empty Authorization header returns 401",
			configToken:    correctToken,
			authHeader:     "",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "Bearer prefix with no token returns 401",
			configToken:    correctToken,
			authHeader:     "Bearer ",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "only Bearer word returns 401",
			configToken:    correctToken,
			authHeader:     "Bearer",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "case sensitive Bearer prefix",
			configToken:    correctToken,
			authHeader:     "bearer correct-token",
			setAuthHeader:  true,
			wantStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewAuthMiddleware(tt.configToken)
			handler := middleware(dummyHandler())

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.setAuthHeader {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatusCode)
			}
		})
	}
}

func Test_NewAuthMiddleware_PassesRequestToNext(t *testing.T) {
	// Verify that when auth succeeds, the request reaches the next handler intact.
	const token = "my-token"
	var called bool

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewAuthMiddleware(token)
	handler := middleware(next)

	req := httptest.NewRequest(http.MethodPost, "/test-path", nil)
	req.Header.Set("Authorization", "Bearer my-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called when auth succeeds")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
}

func Test_NewAuthMiddleware_BlocksRequestFromNext(t *testing.T) {
	// Verify that when auth fails, the next handler is never called.
	const token = "my-token"
	var called bool

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewAuthMiddleware(token)
	handler := middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Error("expected next handler NOT to be called when auth fails")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
