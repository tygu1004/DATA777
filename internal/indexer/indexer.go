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
	"sync"

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

type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
	StateDone    State = "done"
	StateError   State = "error"
)

type Status struct {
	State     State  `json:"status"`
	Processed int    `json:"processed"`
	Error     string `json:"error,omitempty"`
}

type Indexer struct {
	db     *store.DB
	source storage.Source

	mu     sync.Mutex
	status Status
}

func New(db *store.DB, source storage.Source) *Indexer {
	return &Indexer{db: db, source: source, status: Status{State: StateIdle}}
}

func (idx *Indexer) Status() Status {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.status
}

// StartIndex walks the given root in the background, recording every supported image as a
// sample. Re-running against the same root is a no-op for files already indexed (path is
// UNIQUE in the samples table).
func (idx *Indexer) StartIndex(root string) error {
	idx.mu.Lock()
	if idx.status.State == StateRunning {
		idx.mu.Unlock()
		return fmt.Errorf("index already running")
	}
	idx.status = Status{State: StateRunning}
	idx.mu.Unlock()

	go idx.run(root)
	return nil
}

func (idx *Indexer) run(root string) {
	ctx := context.Background()
	err := idx.source.Walk(ctx, root, func(path string, size int64) error {
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

		idx.mu.Lock()
		idx.status.Processed++
		idx.mu.Unlock()
		return nil
	})

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if err != nil {
		idx.status.State = StateError
		idx.status.Error = err.Error()
		return
	}
	idx.status.State = StateDone
}
