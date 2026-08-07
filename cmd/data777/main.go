package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"data777/internal/indexer"
	"data777/internal/server"
	"data777/internal/store"
	"data777/internal/thumbnail"
)

func main() {
	dataDir := flag.String("data-dir", "./devdata", "directory for the sqlite db and thumbnail cache")
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	db, err := store.Open(filepath.Join(*dataDir, "data777.db"))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	thumbs, err := thumbnail.New(filepath.Join(*dataDir, "thumbs"))
	if err != nil {
		log.Fatalf("init thumbnail cache: %v", err)
	}
	previews, err := thumbnail.NewPreview(filepath.Join(*dataDir, "previews"))
	if err != nil {
		log.Fatalf("init preview cache: %v", err)
	}

	idx := indexer.New(db)
	srv := server.New(db, idx, thumbs, previews)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("data777 listening on %s (data dir: %s)", addr, *dataDir)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}
