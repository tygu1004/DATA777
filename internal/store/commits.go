package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNoParentCommit = errors.New("head has no parent commit to undo to")

type TagOp struct {
	SampleID int64  `json:"sample_id"`
	Tag      string `json:"tag"`
	Op       string `json:"op"` // "add" or "remove"
}

type Commit struct {
	ID        int64     `json:"id"`
	ParentID  *int64    `json:"parent_id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	OpCount   int       `json:"op_count"`
	IsHead    bool      `json:"is_head"`
}

// CreateCommit records a new commit with the given tag ops, applies them forward to the
// materialized sample_tags table, and advances HEAD — all inside one transaction so a batch
// tag operation is atomic and undoable as a single step.
func (db *DB) CreateCommit(message string, ops []TagOp) (*Commit, error) {
	if len(ops) == 0 {
		return nil, errors.New("commit must have at least one op")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var parentID sql.NullInt64
	if err := tx.QueryRow(`SELECT commit_id FROM head WHERE id = 1`).Scan(&parentID); err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}

	res, err := tx.Exec(`INSERT INTO commits (parent_id, message) VALUES (?, ?)`, parentID, message)
	if err != nil {
		return nil, fmt.Errorf("insert commit: %w", err)
	}
	commitID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("commit id: %w", err)
	}

	for _, op := range ops {
		if op.Op != "add" && op.Op != "remove" {
			return nil, fmt.Errorf("invalid op %q for sample %d", op.Op, op.SampleID)
		}
		if _, err := tx.Exec(
			`INSERT INTO commit_ops (commit_id, sample_id, tag, op) VALUES (?, ?, ?, ?)`,
			commitID, op.SampleID, op.Tag, op.Op,
		); err != nil {
			return nil, fmt.Errorf("insert commit_op: %w", err)
		}
		if err := applyTagOp(tx, op); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(`UPDATE head SET commit_id = ? WHERE id = 1`, commitID); err != nil {
		return nil, fmt.Errorf("update head: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return db.getCommit(commitID)
}

func applyTagOp(tx *sql.Tx, op TagOp) error {
	var err error
	switch op.Op {
	case "add":
		_, err = tx.Exec(`INSERT INTO sample_tags (sample_id, tag) VALUES (?, ?) ON CONFLICT DO NOTHING`, op.SampleID, op.Tag)
	case "remove":
		_, err = tx.Exec(`DELETE FROM sample_tags WHERE sample_id = ? AND tag = ?`, op.SampleID, op.Tag)
	}
	if err != nil {
		return fmt.Errorf("apply tag op: %w", err)
	}
	return nil
}

func invertOp(op TagOp) TagOp {
	inverted := op
	if op.Op == "add" {
		inverted.Op = "remove"
	} else {
		inverted.Op = "add"
	}
	return inverted
}

// Undo moves HEAD back to the parent of the current HEAD commit, applying each of the
// current HEAD commit's ops in reverse to sample_tags. Single-step only, matching v0's
// "undo = go back one commit" semantics.
func (db *DB) Undo() (*int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var headCommitID sql.NullInt64
	if err := tx.QueryRow(`SELECT commit_id FROM head WHERE id = 1`).Scan(&headCommitID); err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	if !headCommitID.Valid {
		return nil, ErrNoParentCommit
	}

	var parentID sql.NullInt64
	if err := tx.QueryRow(`SELECT parent_id FROM commits WHERE id = ?`, headCommitID.Int64).Scan(&parentID); err != nil {
		return nil, fmt.Errorf("read commit parent: %w", err)
	}

	rows, err := tx.Query(`SELECT sample_id, tag, op FROM commit_ops WHERE commit_id = ?`, headCommitID.Int64)
	if err != nil {
		return nil, fmt.Errorf("read commit ops: %w", err)
	}
	var ops []TagOp
	for rows.Next() {
		var op TagOp
		if err := rows.Scan(&op.SampleID, &op.Tag, &op.Op); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan commit op: %w", err)
		}
		ops = append(ops, op)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, op := range ops {
		if err := applyTagOp(tx, invertOp(op)); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(`UPDATE head SET commit_id = ? WHERE id = 1`, parentID); err != nil {
		return nil, fmt.Errorf("update head: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	if !parentID.Valid {
		return nil, nil
	}
	return &parentID.Int64, nil
}

func (db *DB) GetHead() (*int64, error) {
	var commitID sql.NullInt64
	if err := db.QueryRow(`SELECT commit_id FROM head WHERE id = 1`).Scan(&commitID); err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	if !commitID.Valid {
		return nil, nil
	}
	return &commitID.Int64, nil
}

func (db *DB) getCommit(id int64) (*Commit, error) {
	head, err := db.GetHead()
	if err != nil {
		return nil, err
	}

	var c Commit
	var parentID sql.NullInt64
	var message sql.NullString
	if err := db.QueryRow(
		`SELECT id, parent_id, message, created_at FROM commits WHERE id = ?`, id,
	).Scan(&c.ID, &parentID, &message, &c.CreatedAt); err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}
	if parentID.Valid {
		c.ParentID = &parentID.Int64
	}
	c.Message = message.String
	c.IsHead = head != nil && *head == c.ID

	if err := db.QueryRow(`SELECT COUNT(*) FROM commit_ops WHERE commit_id = ?`, id).Scan(&c.OpCount); err != nil {
		return nil, fmt.Errorf("count commit ops: %w", err)
	}
	return &c, nil
}

func (db *DB) ListCommits(offset, limit int) ([]Commit, error) {
	head, err := db.GetHead()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(
		`SELECT c.id, c.parent_id, c.message, c.created_at, COUNT(o.id)
		 FROM commits c LEFT JOIN commit_ops o ON o.commit_id = c.id
		 GROUP BY c.id ORDER BY c.id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}
	defer rows.Close()

	var commits []Commit
	for rows.Next() {
		var c Commit
		var parentID sql.NullInt64
		var message sql.NullString
		if err := rows.Scan(&c.ID, &parentID, &message, &c.CreatedAt, &c.OpCount); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if parentID.Valid {
			c.ParentID = &parentID.Int64
		}
		c.Message = message.String
		c.IsHead = head != nil && *head == c.ID
		commits = append(commits, c)
	}
	return commits, rows.Err()
}
