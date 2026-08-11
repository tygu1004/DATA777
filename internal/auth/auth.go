// Package auth implements bearer tokens for non-browser clients (docs/api.md#authentication):
// the Python SDK, scripts, and plugin operators, none of which carry a browser session.
//
// ponytail: the dashboard itself keeps today's no-login behavior — docs/api.md only specifies
// token issuance/verification, not a session/cookie login flow, so inventing one here would be
// scope this pass wasn't asked to cover. A request with no Authorization header is treated as
// the trusted local dashboard, same as the pre-v2 MVP; a request that does send a bearer token
// must present a valid one. Real multi-user login is exactly the kind of "opt-in, not core"
// concern architecture.md already draws a line around for SSO/RBAC.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type TokenMeta struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func randomHex(n int) string {
	buf := make([]byte, n)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Create returns the token's id and its secret — the secret is shown once (api.md#authentication)
// and only its hash is stored.
func (s *Store) Create(ctx context.Context, name string, expiresAt *time.Time) (TokenMeta, string, error) {
	meta := TokenMeta{ID: "tok_" + randomHex(6), Name: name, CreatedAt: time.Now(), ExpiresAt: expiresAt}
	secret := "d777_" + randomHex(24)

	_, err := s.db.ExecContext(ctx, `INSERT INTO tokens (id, name, hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		meta.ID, meta.Name, hash(secret), meta.CreatedAt, meta.ExpiresAt)
	if err != nil {
		return TokenMeta{}, "", fmt.Errorf("insert token: %w", err)
	}
	return meta, secret, nil
}

func (s *Store) List(ctx context.Context) ([]TokenMeta, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at, expires_at FROM tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()

	out := []TokenMeta{}
	for rows.Next() {
		var t TokenMeta
		var expires sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &expires); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		if expires.Valid {
			t.ExpiresAt = &expires.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Revoke(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("token %q not found", id)
	}
	return nil
}

// Verify reports whether secret matches a live (non-expired) token.
func (s *Store) Verify(ctx context.Context, secret string) (bool, error) {
	if !strings.HasPrefix(secret, "d777_") {
		return false, nil
	}
	want := hash(secret)

	rows, err := s.db.QueryContext(ctx, `SELECT hash, expires_at FROM tokens`)
	if err != nil {
		return false, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var h string
		var expires sql.NullTime
		if err := rows.Scan(&h, &expires); err != nil {
			return false, fmt.Errorf("scan token: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) != 1 {
			continue
		}
		if expires.Valid && time.Now().After(expires.Time) {
			return false, nil
		}
		return true, nil
	}
	return false, rows.Err()
}

// Middleware rejects a request that presents a bearer token that doesn't verify. A request
// with no Authorization header passes through as the trusted local dashboard — see the
// package doc for why.
func Middleware(store *Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			next.ServeHTTP(w, r)
			return
		}
		secret, ok := strings.CutPrefix(header, "Bearer ")
		if !ok {
			http.Error(w, `{"error":"malformed Authorization header"}`, http.StatusUnauthorized)
			return
		}
		valid, err := store.Verify(r.Context(), secret)
		if err != nil {
			http.Error(w, `{"error":"token verification failed"}`, http.StatusInternalServerError)
			return
		}
		if !valid {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
