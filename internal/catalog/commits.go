package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
)

var ErrNoParentCommit = errors.New("head has no parent commit to undo to")
var ErrHeadMoved = errors.New("head moved since expected_head; someone else committed")

// commitRow is the full internal row, including the affected-set bitmap and set op/value
// that the public Commit type (JSON-facing) doesn't expose.
type commitRow struct {
	ID        int64
	ParentID  *int64
	Message   string
	Kind      CommitKind
	Field     string
	Op        string
	Value     string
	Bitmap    []byte
	Affected  int
	CreatedAt time.Time
}

// Head returns the current HEAD commit id, or nil if the commit log is empty.
func (c *SQLiteCatalog) Head(ctx context.Context) (*int64, error) {
	return c.getHead(ctx)
}

func (c *SQLiteCatalog) getHead(ctx context.Context) (*int64, error) {
	var id sql.NullInt64
	if err := c.db.QueryRowContext(ctx, `SELECT commit_id FROM head WHERE id = 1`).Scan(&id); err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	if !id.Valid {
		return nil, nil
	}
	return &id.Int64, nil
}

// getCommitRow reads through c.db. Never call this while a transaction on c.db is open — with
// SQLite's single-connection pool (SetMaxOpenConns(1)), the open transaction already holds the
// only connection, so this would block forever waiting for a second one that can't exist.
// getCommitRowTx is the version to use from inside a transaction.
func (c *SQLiteCatalog) getCommitRow(ctx context.Context, id int64) (*commitRow, error) {
	return getCommitRowTx(ctx, c.db, id)
}

func getCommitRowTx(ctx context.Context, q dbTx, id int64) (*commitRow, error) {
	var r commitRow
	var parentID sql.NullInt64
	var message, op, value sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT id, parent_id, message, kind, field, op, value, bitmap, affected_count, created_at
		 FROM commits WHERE id = ?`, id,
	).Scan(&r.ID, &parentID, &message, &r.Kind, &r.Field, &op, &value, &r.Bitmap, &r.Affected, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get commit %d: %w", id, err)
	}
	if parentID.Valid {
		r.ParentID = &parentID.Int64
	}
	r.Message, r.Op, r.Value = message.String, op.String, value.String
	return &r, nil
}

func (c *SQLiteCatalog) getCommit(ctx context.Context, id int64) (Commit, error) {
	row, err := c.getCommitRow(ctx, id)
	if err != nil {
		return Commit{}, err
	}
	head, err := c.getHead(ctx)
	if err != nil {
		return Commit{}, err
	}
	return Commit{
		ID: row.ID, ParentID: row.ParentID, Message: row.Message, Kind: row.Kind, Field: row.Field,
		CreatedAt: row.CreatedAt, AffectedCount: row.Affected, OpCount: row.Affected,
		IsHead: head != nil && *head == row.ID,
	}, nil
}

func (c *SQLiteCatalog) resolveSelection(ctx context.Context, sel Selection) (*roaring.Bitmap, error) {
	bm := roaring.New()
	switch sel.Mode {
	case "explicit":
		for _, id := range sel.IDs {
			bm.Add(uint32(id))
		}
	case "filter":
		if sel.Filter == nil {
			return nil, fmt.Errorf("filter selection requires a filter")
		}
		items, _, err := c.evaluate(ctx, *sel.Filter, nil)
		if err != nil {
			return nil, err
		}
		for _, s := range items {
			bm.Add(uint32(s.ID))
		}
	default:
		return nil, fmt.Errorf("unknown selection mode %q", sel.Mode)
	}
	for _, id := range sel.Excluded {
		bm.Remove(uint32(id))
	}
	return bm, nil
}

// ApplySet applies the same operation to every sample in a selection against a tags field.
//
// The commit stores the actual *delta* — not the selection — as its bitmap: for `add`, the
// samples in the selection that didn't already have the tag (S \ B); for `remove`, the ones
// that did (S ∩ B). Storing the raw selection instead would be a real bug, not just an
// inefficiency — "add cat to S" only undoes for free as "remove cat from S" when nothing in S
// already had cat. If some of S already had the tag, undoing against the full selection strips
// a tag those samples had *before* this commit, which no user action asked for. Delta-based
// storage is what actually makes undo free (api.md#post-apicommits); affected_count also
// reports the delta's size (samples that changed), not the selection's (samples matched).
func (c *SQLiteCatalog) ApplySet(ctx context.Context, sel Selection, field string, op Op, value string, message string, report ProgressFunc) (Commit, error) {
	kind, err := c.fieldKind(ctx, field)
	if err != nil {
		return Commit{}, err
	}
	if kind != KindTags {
		return Commit{}, fmt.Errorf("set commits only support tags fields, got kind %q for field %q", kind, field)
	}
	if op != OpAdd && op != OpRemove {
		return Commit{}, fmt.Errorf("invalid op %q", op)
	}

	selected, err := c.resolveSelection(ctx, sel)
	if err != nil {
		return Commit{}, err
	}
	if report != nil {
		report(0, int64(selected.GetCardinality()))
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Commit{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var parentID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT commit_id FROM head WHERE id = 1`).Scan(&parentID); err != nil {
		return Commit{}, fmt.Errorf("read head: %w", err)
	}

	tagBM, err := loadBitmap(ctx, tx, value)
	if err != nil {
		return Commit{}, err
	}

	var delta *roaring.Bitmap
	if op == OpAdd {
		delta = roaring.AndNot(selected, tagBM) // S \ B: newly tagged
	} else {
		delta = roaring.And(selected, tagBM) // S ∩ B: actually removed
	}
	delta.RunOptimize()
	bmBytes, err := delta.ToBytes()
	if err != nil {
		return Commit{}, fmt.Errorf("encode delta bitmap: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO commits (parent_id, message, kind, field, op, value, bitmap, affected_count)
		 VALUES (?, ?, 'set', ?, ?, ?, ?, ?)`,
		parentID, message, field, string(op), value, bmBytes, int(delta.GetCardinality()),
	)
	if err != nil {
		return Commit{}, fmt.Errorf("insert commit: %w", err)
	}
	commitID, err := res.LastInsertId()
	if err != nil {
		return Commit{}, fmt.Errorf("commit id: %w", err)
	}

	if op == OpAdd {
		tagBM.Or(delta)
	} else {
		tagBM.AndNot(delta)
	}
	if err := saveBitmap(ctx, tx, value, tagBM); err != nil {
		return Commit{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE head SET commit_id = ? WHERE id = 1`, commitID); err != nil {
		return Commit{}, fmt.Errorf("update head: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Commit{}, fmt.Errorf("commit tx: %w", err)
	}

	if report != nil {
		report(int64(selected.GetCardinality()), int64(selected.GetCardinality()))
	}
	return c.getCommit(ctx, commitID)
}

// ApplyPatch records a per-sample value edit, storing each prior value so undo doesn't need
// the client to know what it overwrote (api.md#post-apicommits).
func (c *SQLiteCatalog) ApplyPatch(ctx context.Context, field string, patches []Patch, message string) (Commit, error) {
	kind, err := c.fieldKind(ctx, field)
	if err != nil {
		return Commit{}, err
	}
	if kind != KindLabels {
		return Commit{}, fmt.Errorf("patch commits only support labels fields, got kind %q for field %q", kind, field)
	}
	if len(patches) == 0 {
		return Commit{}, fmt.Errorf("patch commit must have at least one patch")
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Commit{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var parentID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT commit_id FROM head WHERE id = 1`).Scan(&parentID); err != nil {
		return Commit{}, fmt.Errorf("read head: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO commits (parent_id, message, kind, field, affected_count) VALUES (?, ?, 'patch', ?, ?)`,
		parentID, message, field, len(patches),
	)
	if err != nil {
		return Commit{}, fmt.Errorf("insert commit: %w", err)
	}
	commitID, err := res.LastInsertId()
	if err != nil {
		return Commit{}, fmt.Errorf("commit id: %w", err)
	}

	for _, p := range patches {
		valueJSON, err := marshalLabel(p.Value)
		if err != nil {
			return Commit{}, err
		}

		var idx int
		var priorJSON sql.NullString
		if p.Index == nil {
			var maxIdx sql.NullInt64
			if err := tx.QueryRowContext(ctx, `SELECT MAX(idx) FROM labels WHERE sample_id = ? AND field = ?`, p.SampleID, field).Scan(&maxIdx); err != nil {
				return Commit{}, fmt.Errorf("find next label index: %w", err)
			}
			idx = 0
			if maxIdx.Valid {
				idx = int(maxIdx.Int64) + 1
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO labels (sample_id, field, idx, value) VALUES (?, ?, ?, ?)`, p.SampleID, field, idx, valueJSON); err != nil {
				return Commit{}, fmt.Errorf("insert label: %w", err)
			}
		} else {
			idx = *p.Index
			var prior string
			err := tx.QueryRowContext(ctx, `SELECT value FROM labels WHERE sample_id = ? AND field = ? AND idx = ?`, p.SampleID, field, idx).Scan(&prior)
			if err == sql.ErrNoRows {
				return Commit{}, fmt.Errorf("no label at index %d for sample %d field %q", idx, p.SampleID, field)
			}
			if err != nil {
				return Commit{}, fmt.Errorf("read prior label: %w", err)
			}
			priorJSON = sql.NullString{String: prior, Valid: true}
			if _, err := tx.ExecContext(ctx, `UPDATE labels SET value = ? WHERE sample_id = ? AND field = ? AND idx = ?`, valueJSON, p.SampleID, field, idx); err != nil {
				return Commit{}, fmt.Errorf("update label: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO commit_patches (commit_id, sample_id, idx, prior_value, new_value) VALUES (?, ?, ?, ?, ?)`,
			commitID, p.SampleID, idx, priorJSON, valueJSON,
		); err != nil {
			return Commit{}, fmt.Errorf("insert commit_patch: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE head SET commit_id = ? WHERE id = 1`, commitID); err != nil {
		return Commit{}, fmt.Errorf("update head: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Commit{}, fmt.Errorf("commit tx: %w", err)
	}
	return c.getCommit(ctx, commitID)
}

func marshalLabel(v LabelValue) (string, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode label value: %w", err)
	}
	return string(buf), nil
}

// Undo moves HEAD back to the parent of the current HEAD commit, inverting its effect —
// bitmap AndNot/Or for a set commit, restored prior values for a patch commit. Single-step,
// matching the v1 MVP's "undo = go back one commit" semantics (api.md#post-apiundo).
//
// expectedHead, when non-nil, must match live HEAD or Undo fails with ErrHeadMoved instead of
// silently undoing whatever commit happens to be at HEAD now. Without this, a single global
// HEAD plus concurrent users is a real bug, not a hypothetical: if A calls undo right as B's
// commit lands, A's "undo my last action" can instead erase B's — the client never asked for
// that and has no way to know it happened. This is optimistic concurrency, not the heavier fix
// (redefining undo as a revert-style new commit, which also buys redo and orphan-commit
// recovery) — the lazy version that closes the actual hole without redesigning the commit
// model in the same pass.
func (c *SQLiteCatalog) Undo(ctx context.Context, expectedHead *int64) (*int64, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var headArg sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT commit_id FROM head WHERE id = 1`).Scan(&headArg); err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	if !headArg.Valid {
		return nil, ErrNoParentCommit
	}
	head := headArg.Int64
	if expectedHead != nil && *expectedHead != head {
		return nil, ErrHeadMoved
	}

	row, err := getCommitRowTx(ctx, tx, head)
	if err != nil {
		return nil, err
	}

	switch row.Kind {
	case CommitSet:
		bm := roaring.New()
		if err := bm.UnmarshalBinary(row.Bitmap); err != nil {
			return nil, fmt.Errorf("decode commit bitmap: %w", err)
		}
		tagBM, err := loadBitmap(ctx, tx, row.Value)
		if err != nil {
			return nil, err
		}
		if row.Op == string(OpAdd) {
			tagBM.AndNot(bm)
		} else {
			tagBM.Or(bm)
		}
		if err := saveBitmap(ctx, tx, row.Value, tagBM); err != nil {
			return nil, err
		}

	case CommitPatch:
		rows, err := tx.QueryContext(ctx, `SELECT sample_id, idx, prior_value FROM commit_patches WHERE commit_id = ?`, row.ID)
		if err != nil {
			return nil, fmt.Errorf("read commit patches: %w", err)
		}
		type undoPatch struct {
			sampleID int64
			idx      int
			prior    sql.NullString
		}
		var patches []undoPatch
		for rows.Next() {
			var p undoPatch
			if err := rows.Scan(&p.sampleID, &p.idx, &p.prior); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit patch: %w", err)
			}
			patches = append(patches, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		for _, p := range patches {
			if p.prior.Valid {
				if _, err := tx.ExecContext(ctx, `UPDATE labels SET value = ? WHERE sample_id = ? AND field = ? AND idx = ?`, p.prior.String, p.sampleID, row.Field, p.idx); err != nil {
					return nil, fmt.Errorf("restore prior label: %w", err)
				}
			} else {
				if _, err := tx.ExecContext(ctx, `DELETE FROM labels WHERE sample_id = ? AND field = ? AND idx = ?`, p.sampleID, row.Field, p.idx); err != nil {
					return nil, fmt.Errorf("delete appended label: %w", err)
				}
			}
		}
	}

	var parentArg sql.NullInt64
	if row.ParentID != nil {
		parentArg = sql.NullInt64{Int64: *row.ParentID, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE head SET commit_id = ? WHERE id = 1`, parentArg); err != nil {
		return nil, fmt.Errorf("update head: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return row.ParentID, nil
}

// ListCommits walks parent_id from HEAD, so undone commits (still in the table, just
// unreachable from HEAD) drop out of the list instead of only losing the is_head flag.
func (c *SQLiteCatalog) ListCommits(ctx context.Context, offset, limit int) ([]Commit, error) {
	head, err := c.getHead(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := c.db.QueryContext(ctx,
		`WITH RECURSIVE chain(id, parent_id, message, kind, field, affected_count, created_at) AS (
			SELECT c.id, c.parent_id, c.message, c.kind, c.field, c.affected_count, c.created_at
			FROM commits c JOIN head h ON c.id = h.commit_id
			UNION ALL
			SELECT c.id, c.parent_id, c.message, c.kind, c.field, c.affected_count, c.created_at
			FROM commits c JOIN chain ON c.id = chain.parent_id
		 )
		 SELECT id, parent_id, message, kind, field, affected_count, created_at
		 FROM chain ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}
	defer rows.Close()

	commits := []Commit{}
	for rows.Next() {
		var cm Commit
		var parentID sql.NullInt64
		var message sql.NullString
		if err := rows.Scan(&cm.ID, &parentID, &message, &cm.Kind, &cm.Field, &cm.AffectedCount, &cm.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if parentID.Valid {
			cm.ParentID = &parentID.Int64
		}
		cm.Message = message.String
		cm.OpCount = cm.AffectedCount
		cm.IsHead = head != nil && *head == cm.ID
		commits = append(commits, cm)
	}
	return commits, rows.Err()
}

// overlayAtCommit reconstructs tag/label state as of atCommit by walking HEAD's parent chain
// back to it and undoing each commit in memory (never touching the live tables). Cost is
// proportional to how many commits separate atCommit from HEAD, per api.md#get-apisamples.
//
// ponytail: only ancestors of the current HEAD are reachable this way — a commit orphaned by
// an earlier undo (still in the table, but off the live chain) returns an error rather than
// being reconstructed. Real arbitrary-history time travel would need persisted per-commit
// snapshots, which api.md explicitly leaves as an implementation's own choice, not a
// requirement.
func (c *SQLiteCatalog) overlayAtCommit(ctx context.Context, atCommit int64, tagsByID map[int64][]string, labelsByID map[int64]map[string]map[int]LabelValue) (map[int64][]string, map[int64]map[string]map[int]LabelValue, error) {
	head, err := c.getHead(ctx)
	if err != nil {
		return nil, nil, err
	}
	if head != nil && *head == atCommit {
		return tagsByID, labelsByID, nil
	}
	if head == nil {
		if atCommit == 0 {
			return tagsByID, labelsByID, nil
		}
		return nil, nil, fmt.Errorf("commit %d not reachable from HEAD", atCommit)
	}

	var chain []*commitRow
	cur := *head
	for {
		row, err := c.getCommitRow(ctx, cur)
		if err != nil {
			return nil, nil, err
		}
		if row.ID == atCommit {
			break
		}
		chain = append(chain, row)
		if row.ParentID == nil {
			return nil, nil, fmt.Errorf("commit %d not reachable from HEAD", atCommit)
		}
		cur = *row.ParentID
	}

	tagSets := map[int64]map[string]bool{}
	for id, tags := range tagsByID {
		set := map[string]bool{}
		for _, t := range tags {
			set[t] = true
		}
		tagSets[id] = set
	}

	for _, row := range chain {
		switch row.Kind {
		case CommitSet:
			bm := roaring.New()
			if err := bm.UnmarshalBinary(row.Bitmap); err != nil {
				return nil, nil, fmt.Errorf("decode commit %d bitmap: %w", row.ID, err)
			}
			// undo = apply the inverse of what this commit did
			addBack := row.Op == string(OpRemove)
			it := bm.Iterator()
			for it.HasNext() {
				id := int64(it.Next())
				if tagSets[id] == nil {
					tagSets[id] = map[string]bool{}
				}
				if addBack {
					tagSets[id][row.Value] = true
				} else {
					delete(tagSets[id], row.Value)
				}
			}

		case CommitPatch:
			patches, err := c.getCommitPatches(ctx, row.ID)
			if err != nil {
				return nil, nil, err
			}
			for _, p := range patches {
				if labelsByID[p.sampleID] == nil {
					labelsByID[p.sampleID] = map[string]map[int]LabelValue{}
				}
				if labelsByID[p.sampleID][row.Field] == nil {
					labelsByID[p.sampleID][row.Field] = map[int]LabelValue{}
				}
				if p.prior == nil {
					delete(labelsByID[p.sampleID][row.Field], p.idx)
				} else {
					labelsByID[p.sampleID][row.Field][p.idx] = p.prior
				}
			}
		}
	}

	result := map[int64][]string{}
	for id, set := range tagSets {
		for t := range set {
			result[id] = append(result[id], t)
		}
	}
	return result, labelsByID, nil
}

type commitPatchRow struct {
	sampleID int64
	idx      int
	prior    LabelValue // nil means the patch appended, so undoing removes the entry
}

func (c *SQLiteCatalog) getCommitPatches(ctx context.Context, commitID int64) ([]commitPatchRow, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT sample_id, idx, prior_value FROM commit_patches WHERE commit_id = ?`, commitID)
	if err != nil {
		return nil, fmt.Errorf("read commit patches: %w", err)
	}
	defer rows.Close()

	var out []commitPatchRow
	for rows.Next() {
		var p commitPatchRow
		var prior sql.NullString
		if err := rows.Scan(&p.sampleID, &p.idx, &prior); err != nil {
			return nil, fmt.Errorf("scan commit patch: %w", err)
		}
		if prior.Valid {
			if err := json.Unmarshal([]byte(prior.String), &p.prior); err != nil {
				return nil, fmt.Errorf("decode prior label value: %w", err)
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
