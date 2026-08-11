package store

import "fmt"

// InsertSample inserts a sample, ignoring rows whose path already exists (re-indexing is a no-op per file).
func (db *DB) InsertSample(path, filename string, width, height int, filesize int64, format string) error {
	_, err := db.Exec(
		`INSERT INTO samples (path, filename, width, height, filesize, format) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO NOTHING`,
		path, filename, width, height, filesize, format,
	)
	if err != nil {
		return fmt.Errorf("insert sample: %w", err)
	}
	return nil
}
