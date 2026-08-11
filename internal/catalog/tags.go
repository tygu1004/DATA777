package catalog

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/RoaringBitmap/roaring/v2"
)

// dbTx is satisfied by both *sql.DB and *sql.Tx, so bitmap helpers work whether or not a
// caller is inside a transaction.
type dbTx interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func loadBitmap(ctx context.Context, q dbTx, tag string) (*roaring.Bitmap, error) {
	var buf []byte
	err := q.QueryRowContext(ctx, `SELECT bitmap FROM tag_bitmaps WHERE tag = ?`, tag).Scan(&buf)
	if err == sql.ErrNoRows {
		return roaring.New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("load bitmap %q: %w", tag, err)
	}
	bm := roaring.New()
	if err := bm.UnmarshalBinary(buf); err != nil {
		return nil, fmt.Errorf("decode bitmap %q: %w", tag, err)
	}
	return bm, nil
}

func saveBitmap(ctx context.Context, q dbTx, tag string, bm *roaring.Bitmap) error {
	bm.RunOptimize()
	buf, err := bm.ToBytes()
	if err != nil {
		return fmt.Errorf("encode bitmap %q: %w", tag, err)
	}
	_, err = q.ExecContext(ctx,
		`INSERT INTO tag_bitmaps (tag, bitmap) VALUES (?, ?)
		 ON CONFLICT(tag) DO UPDATE SET bitmap = excluded.bitmap`,
		tag, buf,
	)
	if err != nil {
		return fmt.Errorf("save bitmap %q: %w", tag, err)
	}
	return nil
}
