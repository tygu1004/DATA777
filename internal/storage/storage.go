// Package storage abstracts read access to source image files so the indexer and thumbnail
// generator don't care whether files live on local disk or in an S3-compatible bucket.
package storage

import (
	"context"
	"io"
)

// Source is implemented by Local and S3.
type Source interface {
	// Walk enumerates every regular file under root, calling fn with each file's path and size.
	Walk(ctx context.Context, root string, fn func(path string, size int64) error) error
	// Open returns a reader for the file at path. Callers must Close it.
	Open(ctx context.Context, path string) (io.ReadCloser, error)
}
