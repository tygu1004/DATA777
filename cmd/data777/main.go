package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"data777/internal/auth"
	"data777/internal/catalog"
	"data777/internal/indexer"
	"data777/internal/jobs"
	"data777/internal/plugins"
	"data777/internal/server"
	"data777/internal/storage"
	"data777/internal/store"
	"data777/internal/thumbnail"
	"data777/internal/vectorindex"
)

func main() {
	dataDir := flag.String("data-dir", "./devdata", "directory for the sqlite db and thumbnail cache")
	port := flag.Int("port", 8080, "port to listen on")

	storageBackend := flag.String("storage", "local", "where source images live: local or s3")
	s3Bucket := flag.String("s3-bucket", "", "s3 bucket name (storage=s3)")
	s3Region := flag.String("s3-region", "us-east-1", "s3 region (storage=s3)")
	s3Endpoint := flag.String("s3-endpoint", "", "custom s3 endpoint, e.g. http://localhost:9000 for RustFS/MinIO (storage=s3)")
	s3PathStyle := flag.Bool("s3-path-style", true, "use path-style bucket addressing, required by most self-hosted s3-compatible servers (storage=s3)")
	pluginsConfig := flag.String("plugins-config", "plugins.yaml", "plugin registration file (plugins.md#registration); missing file means no plugins")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	db, err := store.Open(filepath.Join(*dataDir, "data777.db"))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	source, err := newSource(*storageBackend, *s3Bucket, *s3Region, *s3Endpoint, *s3PathStyle)
	if err != nil {
		log.Fatalf("init storage backend: %v", err)
	}

	thumbs, err := thumbnail.New(filepath.Join(*dataDir, "thumbs"), source)
	if err != nil {
		log.Fatalf("init thumbnail cache: %v", err)
	}
	previews, err := thumbnail.NewPreview(filepath.Join(*dataDir, "previews"), source)
	if err != nil {
		log.Fatalf("init preview cache: %v", err)
	}

	vector := vectorindex.NewBruteForce(db.DB)
	cat := catalog.NewSQLite(db.DB, vector)
	idx := indexer.New(db, source)
	tokens := auth.NewStore(db.DB)

	jobsMgr := jobs.New(db.DB)
	ctx := context.Background()
	if err := jobsMgr.MarkInterruptedOnStartup(ctx); err != nil {
		log.Fatalf("mark interrupted jobs: %v", err)
	}

	pluginsReg, err := plugins.Load(*pluginsConfig)
	if err != nil {
		log.Fatalf("load plugins config: %v", err)
	}
	pluginsReg.Reload(ctx)

	srv := server.New(cat, vector, idx, thumbs, previews, jobsMgr, tokens, pluginsReg)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("data777 listening on %s (data dir: %s, storage: %s)", addr, *dataDir, *storageBackend)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}

func newSource(backend, bucket, region, endpoint string, pathStyle bool) (storage.Source, error) {
	switch backend {
	case "local":
		return storage.NewLocal(), nil
	case "s3":
		return storage.NewS3(context.Background(), storage.S3Config{
			Bucket:    bucket,
			Region:    region,
			Endpoint:  endpoint,
			PathStyle: pathStyle,
		})
	default:
		return nil, fmt.Errorf("unknown storage backend %q (want local or s3)", backend)
	}
}
