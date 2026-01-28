package storage

import (
	"context"
	"io"
)

// Storage defines the interface for file storage operations.
type Storage interface {
	// Save saves a file to the storage.
	// path is the relative path where the file should be stored.
	// content is the file content.
	// Returns the error if any.
	Save(ctx context.Context, path string, content io.Reader) error

	// Get retrieves a file from the storage.
	// path is the relative path of the file.
	// Returns a ReadCloser for the file content, or error.
	Get(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes a file from the storage.
	// path is the relative path of the file to delete.
	// Returns error if any.
	Delete(ctx context.Context, path string) error
}
