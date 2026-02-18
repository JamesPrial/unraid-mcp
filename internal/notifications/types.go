// Package notifications provides notification management for Unraid systems
// via the Unraid GraphQL API.
package notifications

import "context"

// Notification represents a single Unraid notification.
type Notification struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Subject     string  `json:"subject"`
	Description string  `json:"description"`
	Importance  string  `json:"importance"`
	Timestamp   *string `json:"timestamp"`
}

// NotificationManager defines the interface for notification operations.
type NotificationManager interface {
	List(ctx context.Context, filterType string, limit int) ([]Notification, error)
	Archive(ctx context.Context, id string) error
	Unarchive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	ArchiveAll(ctx context.Context) error
	DeleteAll(ctx context.Context) error
}
