// Package indexer walks a storage.Source root and records every supported image as a sample.
// Indexing runs as a job (docs/api.md#post-apiindex) — this package just does the walk and
// reports progress back through whatever callback the job wrapper hands it.
package indexer

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"data777/internal/storage"
	"data777/internal/store"
)

var supportedExt = map[string]string{
	".jpg":  "jpeg",
	".jpeg": "jpeg",
	".png":  "png",
	".gif":  "gif",
	".webp": "webp",
	".bmp":  "bmp",
}

type Indexer struct {
	db     *store.DB
	source storage.Source
}

func New(db *store.DB, source storage.Source) *Indexer {
	return &Indexer{db: db, source: source}
}

// Run walks root, recording every supported image as a sample. Re-running against the same
// root is a no-op for files already indexed (path is UNIQUE in the samples table). report is
// called with total=0 to mean "not known in advance" (api.md#jobs).
func (idx *Indexer) Run(ctx context.Context, root string, report func(processed, total int64)) (any, error) {
	var processed int64
	err := idx.source.Walk(ctx, root, func(path string, size int64) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		format, ok := supportedExt[strings.ToLower(filepath.Ext(path))]
		if !ok {
			return nil
		}

		rc, err := idx.source.Open(ctx, path)
		if err != nil {
			return nil // skip unreadable file
		}
		cfg, _, err := image.DecodeConfig(rc)
		rc.Close()
		if err != nil {
			return nil // skip undecodable file
		}

		if err := idx.db.InsertSample(path, filepath.Base(path), cfg.Width, cfg.Height, size, format); err != nil {
			return err
		}

		processed++
		report(processed, 0)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", root, err)
	}
	return map[string]any{"processed": processed}, nil
}
