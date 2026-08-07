package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Local reads files from the machine's own filesystem.
type Local struct{}

func NewLocal() Local { return Local{} }

func (Local) Walk(ctx context.Context, root string, fn func(path string, size int64) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // skip unreadable file
		}
		return fn(path, info.Size())
	})
}

func (Local) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}
