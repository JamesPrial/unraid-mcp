// Package notifications provides notification management for Unraid systems
// via the Unraid GraphQL API.
package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
)

// validFilterTypes is the allowlist of accepted filterType values for List.
var validFilterTypes = map[string]bool{
	"UNREAD":  true,
	"ALL":     true,
	"ARCHIVE": true,
}

// Compile-time interface check.
var _ NotificationManager = (*GraphQLNotificationManager)(nil)

// GraphQLNotificationManager implements NotificationManager using a GraphQL client.
type GraphQLNotificationManager struct {
	client graphql.Client
}

// NewGraphQLNotificationManager returns a new GraphQLNotificationManager backed
// by the provided GraphQL client.
func NewGraphQLNotificationManager(client graphql.Client) *GraphQLNotificationManager {
	if client == nil {
		panic("graphql client must not be nil")
	}
	return &GraphQLNotificationManager{client: client}
}

// listResponse is the JSON wrapper for a notifications list query response.
type listResponse struct {
	Notifications struct {
		List []Notification `json:"list"`
	} `json:"notifications"`
}

// List executes a GraphQL query to retrieve notifications matching the given
// filter type and limit. filterType must be one of "UNREAD", "ARCHIVE", or
// "ALL"; any other value returns an error to prevent query injection.
func (m *GraphQLNotificationManager) List(ctx context.Context, filterType string, limit int) ([]Notification, error) {
	if !validFilterTypes[filterType] {
		return nil, fmt.Errorf("invalid filter type %q: must be UNREAD, ALL, or ARCHIVE", filterType)
	}

	query := fmt.Sprintf(
		`{ notifications { list(filter: { type: %s, limit: %d }) { id title subject description importance timestamp } } }`,
		filterType, limit,
	)

	data, err := m.client.Execute(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("notifications list: %w", err)
	}

	var resp listResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("notifications list: parse response: %w", err)
	}

	return resp.Notifications.List, nil
}

// validateID rejects notification ids that are empty or contain quote or
// backslash characters to prevent GraphQL query injection.
func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("invalid notification id: empty string")
	}
	if strings.ContainsAny(id, `"'\`) {
		return fmt.Errorf("invalid notification id: %q", id)
	}
	return nil
}

// Archive executes a GraphQL mutation to archive the notification with the
// given id.
func (m *GraphQLNotificationManager) Archive(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return fmt.Errorf("notifications archive: %w", err)
	}
	mutation := fmt.Sprintf(`mutation { notifications { archive(id: "%s") } }`, id)
	if _, err := m.client.Execute(ctx, mutation, nil); err != nil {
		return fmt.Errorf("notifications archive: %w", err)
	}
	return nil
}

// Unarchive executes a GraphQL mutation to unarchive the notification with the
// given id.
func (m *GraphQLNotificationManager) Unarchive(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return fmt.Errorf("notifications unarchive: %w", err)
	}
	mutation := fmt.Sprintf(`mutation { notifications { unarchive(id: "%s") } }`, id)
	if _, err := m.client.Execute(ctx, mutation, nil); err != nil {
		return fmt.Errorf("notifications unarchive: %w", err)
	}
	return nil
}

// Delete executes a GraphQL mutation to permanently delete the notification
// with the given id.
func (m *GraphQLNotificationManager) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return fmt.Errorf("notifications delete: %w", err)
	}
	mutation := fmt.Sprintf(`mutation { notifications { delete(id: "%s") } }`, id)
	if _, err := m.client.Execute(ctx, mutation, nil); err != nil {
		return fmt.Errorf("notifications delete: %w", err)
	}
	return nil
}

// ArchiveAll executes a GraphQL mutation to archive all notifications.
func (m *GraphQLNotificationManager) ArchiveAll(ctx context.Context) error {
	mutation := `mutation { notifications { archiveAll } }`
	if _, err := m.client.Execute(ctx, mutation, nil); err != nil {
		return fmt.Errorf("notifications archiveAll: %w", err)
	}
	return nil
}

// DeleteAll executes a GraphQL mutation to permanently delete all notifications.
func (m *GraphQLNotificationManager) DeleteAll(ctx context.Context) error {
	mutation := `mutation { notifications { deleteAll } }`
	if _, err := m.client.Execute(ctx, mutation, nil); err != nil {
		return fmt.Errorf("notifications deleteAll: %w", err)
	}
	return nil
}
