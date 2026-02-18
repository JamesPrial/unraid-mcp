// Package shares provides Unraid share listing via the GraphQL API.
package shares

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
)

// GraphQLShareManager implements ShareManager by querying the Unraid GraphQL API.
type GraphQLShareManager struct {
	client graphql.Client
}

// NewGraphQLShareManager returns a new GraphQLShareManager that uses the given
// GraphQL client to fetch share data.
func NewGraphQLShareManager(client graphql.Client) *GraphQLShareManager {
	if client == nil {
		panic("graphql client must not be nil")
	}
	return &GraphQLShareManager{client: client}
}

// sharesResponse is the JSON envelope returned by the GraphQL shares query.
// Execute() already strips the outer "data" envelope, so this struct maps
// directly to the contents of the data object.
type sharesResponse struct {
	Shares []Share `json:"shares"`
}

// List fetches all shares from the Unraid GraphQL API and returns them as a
// slice. An empty array in the response is returned as a non-nil empty slice.
func (m *GraphQLShareManager) List(ctx context.Context) ([]Share, error) {
	const query = `query { shares { name size used free comment } }`

	raw, err := m.client.Execute(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("shares: execute query: %w", err)
	}

	var resp sharesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("shares: unmarshal response: %w", err)
	}

	shares := resp.Shares
	if shares == nil {
		shares = []Share{}
	}

	return shares, nil
}

// Compile-time interface check.
var _ ShareManager = (*GraphQLShareManager)(nil)
