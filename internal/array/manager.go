// Package array provides Unraid array management via the GraphQL API.
// It exposes Start, Stop, and ParityCheck operations that map directly to
// the corresponding Unraid GraphQL mutations.
package array

import (
	"context"
	"fmt"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
)

// Compile-time interface check.
var _ ArrayManager = (*GraphQLArrayManager)(nil)

// GraphQLArrayManager implements ArrayManager by issuing GraphQL mutations
// against the Unraid API.
type GraphQLArrayManager struct {
	client graphql.Client
}

// NewGraphQLArrayManager returns a new GraphQLArrayManager that uses the
// provided graphql.Client for all API calls.
func NewGraphQLArrayManager(client graphql.Client) *GraphQLArrayManager {
	if client == nil {
		panic("graphql client must not be nil")
	}
	return &GraphQLArrayManager{client: client}
}

// Start issues a mutation to start the Unraid array.
func (m *GraphQLArrayManager) Start(ctx context.Context) error {
	const query = `mutation { array { start } }`
	if _, err := m.client.Execute(ctx, query, nil); err != nil {
		return fmt.Errorf("array start: %w", err)
	}
	return nil
}

// Stop issues a mutation to stop the Unraid array.
func (m *GraphQLArrayManager) Stop(ctx context.Context) error {
	const query = `mutation { array { stop } }`
	if _, err := m.client.Execute(ctx, query, nil); err != nil {
		return fmt.Errorf("array stop: %w", err)
	}
	return nil
}

// validParityActions is the set of accepted parity-check action strings.
var validParityActions = map[string]struct{}{
	"start":         {},
	"start_correct": {},
	"pause":         {},
	"resume":        {},
	"cancel":        {},
}

// IsValidParityAction reports whether action is a recognized parity check action.
func IsValidParityAction(action string) bool {
	_, ok := validParityActions[action]
	return ok
}

// ParityCheck issues a parity-check mutation for the given action.
// Valid actions: start, start_correct, pause, resume, cancel.
// An unrecognised action returns an error immediately, before the client is
// called.
func (m *GraphQLArrayManager) ParityCheck(ctx context.Context, action string) (string, error) {
	if _, ok := validParityActions[action]; !ok {
		return "", fmt.Errorf("invalid parity check action: %q (valid: start, start_correct, pause, resume, cancel)", action)
	}

	var query string
	switch action {
	case "start":
		query = `mutation { array { parityCheck { start(correct: false) } } }`
	case "start_correct":
		query = `mutation { array { parityCheck { start(correct: true) } } }`
	case "pause":
		query = `mutation { array { parityCheck { pause } } }`
	case "resume":
		query = `mutation { array { parityCheck { resume } } }`
	case "cancel":
		query = `mutation { array { parityCheck { cancel } } }`
	}

	if _, err := m.client.Execute(ctx, query, nil); err != nil {
		return "", fmt.Errorf("parity check %s: %w", action, err)
	}

	return fmt.Sprintf("Parity check %s command issued.", action), nil
}
