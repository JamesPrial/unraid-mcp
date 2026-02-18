// Package shares provides types and interfaces for Unraid share operations.
package shares

import "context"

// Share represents a single Unraid share.
type Share struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Used    int64  `json:"used"`
	Free    int64  `json:"free"`
	Comment string `json:"comment"`
}

// ShareManager defines the interface for share operations.
type ShareManager interface {
	List(ctx context.Context) ([]Share, error)
}
