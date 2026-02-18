// Package graphql provides a GraphQL HTTP client for communicating with the
// Unraid GraphQL API.
package graphql

import "context"

// GraphQLError represents a single error returned in a GraphQL response.
type GraphQLError struct {
	Message string `json:"message"`
}

// Client defines the interface for executing GraphQL queries.
type Client interface {
	Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}
