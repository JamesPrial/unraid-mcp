// Package ups provides UPS (Uninterruptible Power Supply) monitoring for
// Unraid systems via GraphQL.
package ups

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
)

// Compile-time interface check.
var _ UPSMonitor = (*GraphQLUPSMonitor)(nil)

// GraphQLUPSMonitor implements UPSMonitor by querying the Unraid GraphQL API.
type GraphQLUPSMonitor struct {
	client graphql.Client
}

// NewGraphQLUPSMonitor returns a new GraphQLUPSMonitor that uses the provided
// GraphQL client to fetch UPS device data.
func NewGraphQLUPSMonitor(client graphql.Client) *GraphQLUPSMonitor {
	if client == nil {
		panic("graphql client must not be nil")
	}
	return &GraphQLUPSMonitor{client: client}
}

// upsResponse is the JSON envelope returned by the GraphQL UPS query.
// Execute() already strips the outer "data" envelope, so this struct maps
// directly to the contents of the data object. The field name matches the
// GraphQL query field "ups".
type upsResponse struct {
	UPS []UPSDevice `json:"ups"`
}

const upsQuery = `query { ups { id name model status battery { charge runtime } power { inputVoltage outputVoltage load } } }`

// GetDevices queries the Unraid GraphQL API for all UPS devices and returns
// them as a slice of UPSDevice. An empty (non-nil) slice is returned when no
// devices are configured.
func (m *GraphQLUPSMonitor) GetDevices(ctx context.Context) ([]UPSDevice, error) {
	data, err := m.client.Execute(ctx, upsQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("get ups devices: %w", err)
	}

	var resp upsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("get ups devices: parse response: %w", err)
	}

	// Return a non-nil empty slice when the API returns no devices so that
	// callers can distinguish "empty" from "error".
	if resp.UPS == nil {
		return []UPSDevice{}, nil
	}
	return resp.UPS, nil
}
