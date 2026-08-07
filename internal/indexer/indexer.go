package indexer

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

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
	db *store.DB

	mu     sync.Mutex
	status Status
}

func New(db *store.DB) *Indexer {
	return &Indexer{db: db, status: Status{State: StateIdle}}
}

func (idx *Indexer) Status() Status {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.status
}

// StartIndex walks the given folder in the background, recording every supported image as a
// sample. Re-running against the same folder is a no-op for files already indexed (path is
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
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		format, ok := supportedExt[strings.ToLower(filepath.Ext(path))]
		if !ok {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable file
		}
		cfg, _, err := image.DecodeConfig(f)
		info, statErr := f.Stat()
		f.Close()
		if err != nil || statErr != nil {
			return nil // skip undecodable file
		}

		if err := idx.db.InsertSample(path, filepath.Base(path), cfg.Width, cfg.Height, info.Size(), format); err != nil {
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
