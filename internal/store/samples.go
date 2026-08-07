package store

import "fmt"

type Sample struct {
	ID       int64    `json:"id"`
	Path     string   `json:"path"`
	Filename string   `json:"filename"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Filesize int64    `json:"filesize"`
	Format   string   `json:"format"`
	Tags     []string `json:"tags"`
}

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

func (db *DB) CountSamples() (int, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count samples: %w", err)
	}
	return count, nil
}

// ListSamples returns a window of samples ordered by id, each with its current tags attached.
func (db *DB) ListSamples(offset, limit int) ([]Sample, error) {
	rows, err := db.Query(
		`SELECT id, path, filename, width, height, filesize, format
		 FROM samples ORDER BY id LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list samples: %w", err)
	}
	defer rows.Close()

	samples := []Sample{} // never nil: encoding/json renders a nil slice as `null`, not `[]`
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var s Sample
		if err := rows.Scan(&s.ID, &s.Path, &s.Filename, &s.Width, &s.Height, &s.Filesize, &s.Format); err != nil {
			return nil, fmt.Errorf("scan sample: %w", err)
		}
		s.Tags = []string{}
		samples = append(samples, s)
		ids = append(ids, s.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return samples, nil
	}

	tagsBySample, err := db.tagsForSamples(ids)
	if err != nil {
		return nil, err
	}
	for i := range samples {
		if tags, ok := tagsBySample[samples[i].ID]; ok {
			samples[i].Tags = tags
		}
	}
	return samples, nil
}

func (db *DB) tagsForSamples(ids []int64) (map[int64][]string, error) {
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT sample_id, tag FROM sample_tags WHERE sample_id IN (%s) ORDER BY sample_id, tag`, placeholders)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]string)
	for rows.Next() {
		var sampleID int64
		var tag string
		if err := rows.Scan(&sampleID, &tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		result[sampleID] = append(result[sampleID], tag)
	}
	return result, rows.Err()
}

func (db *DB) GetSamplePath(id int64) (string, error) {
	var path string
	if err := db.QueryRow(`SELECT path FROM samples WHERE id = ?`, id).Scan(&path); err != nil {
		return "", fmt.Errorf("get sample path: %w", err)
	}
	return path, nil
}
