// Package array provides types and interfaces for Unraid array operations.
package array

import "context"

// ArrayManager defines the interface for array operations.
type ArrayManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	ParityCheck(ctx context.Context, action string) (string, error)
}
