package catalog

import (
	"context"
	"database/sql"
	"fmt"

	"data777/internal/vectorindex"
)

// SQLiteCatalog is the default Catalog implementation (api.md#internal-interface). It is
// correct at any scale but, unlike the documented ClickHouseCatalog scale-out path, evaluates
// a filter by materializing candidate rows in Go rather than pushing every predicate down into
// SQL — see the ponytail note on evaluate() in query.go for the ceiling this implies.
type SQLiteCatalog struct {
	db     *sql.DB
	vector vectorindex.Index
}

func NewSQLite(db *sql.DB, vector vectorindex.Index) *SQLiteCatalog {
	return &SQLiteCatalog{db: db, vector: vector}
}

func (c *SQLiteCatalog) Schema(ctx context.Context) ([]FieldDef, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT name, kind, COALESCE(type,''), COALESCE(dims,0), COALESCE(metric,'') FROM fields ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("query fields: %w", err)
	}
	defer rows.Close()

	fields := []FieldDef{}
	for rows.Next() {
		var f FieldDef
		if err := rows.Scan(&f.Name, &f.Kind, &f.Type, &f.Dims, &f.Metric); err != nil {
			return nil, fmt.Errorf("scan field: %w", err)
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

var ErrFieldConflict = fmt.Errorf("field already declared with a different definition")

// DefineField declares a new labels or embedding field. Idempotent for an identical
// redeclaration; a conflicting redeclaration is reported via ErrFieldConflict (mapped to 409
// by the HTTP handler), per api.md#post-apischemafields.
func (c *SQLiteCatalog) DefineField(ctx context.Context, f FieldDef) error {
	if f.Kind != KindLabels && f.Kind != KindEmbedding {
		return fmt.Errorf("only labels and embedding fields can be declared, got kind %q", f.Kind)
	}

	existing, err := c.Schema(ctx)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.Name != f.Name {
			continue
		}
		if e.Kind == f.Kind && e.Type == f.Type && e.Dims == f.Dims && e.Metric == f.Metric {
			return nil // identical redeclaration: no-op
		}
		return ErrFieldConflict
	}

	_, err = c.db.ExecContext(ctx,
		`INSERT INTO fields (name, kind, type, dims, metric) VALUES (?, ?, NULLIF(?,''), NULLIF(?,0), NULLIF(?,''))`,
		f.Name, f.Kind, f.Type, f.Dims, f.Metric,
	)
	if err != nil {
		return fmt.Errorf("insert field: %w", err)
	}
	return nil
}

func (c *SQLiteCatalog) fieldKind(ctx context.Context, name string) (FieldKind, error) {
	var kind string
	err := c.db.QueryRowContext(ctx, `SELECT kind FROM fields WHERE name = ?`, name).Scan(&kind)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("unknown field %q", name)
	}
	if err != nil {
		return "", fmt.Errorf("lookup field kind: %w", err)
	}
	return FieldKind(kind), nil
}
