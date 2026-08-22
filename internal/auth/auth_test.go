package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"data777/internal/store"
)

func setupTestAuthStore(t *testing.T) (*Store, *store.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return NewStore(db.DB), db
}

func TestAuth_TokenLifecycle(t *testing.T) {
	ctx := context.Background()
	authStore, _ := setupTestAuthStore(t)

	// 1. Create a valid token
	meta, secret, err := authStore.Create(ctx, "sdk-token", nil)
	if err != nil {
		t.Fatalf("Create token: %v", err)
	}
	if meta.ID == "" || secret == "" {
		t.Fatalf("empty token meta or secret: %+v, secret=%q", meta, secret)
	}

	// 2. Verify valid token
	ok, err := authStore.Verify(ctx, secret)
	if err != nil || !ok {
		t.Fatalf("Verify valid token ok=%v, err=%v", ok, err)
	}

	// 3. Verify invalid token
	ok, err = authStore.Verify(ctx, "d777_invalid_secret_token")
	if err != nil || ok {
		t.Errorf("Verify invalid token ok=%v, err=%v, want ok=false", ok, err)
	}

	// 4. List tokens
	tokens, err := authStore.List(ctx)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("List tokens = %+v, err = %v, want 1 token", tokens, err)
	}

	// 5. Revoke token
	if err := authStore.Revoke(ctx, meta.ID); err != nil {
		t.Fatalf("Revoke token: %v", err)
	}

	// 6. Verify revoked token should now fail
	ok, err = authStore.Verify(ctx, secret)
	if err != nil || ok {
		t.Errorf("Verify revoked token ok=%v, want ok=false", ok)
	}
}

func TestAuth_ExpiredToken(t *testing.T) {
	ctx := context.Background()
	authStore, _ := setupTestAuthStore(t)

	// Create an expired token (expired 1 hour ago)
	past := time.Now().Add(-1 * time.Hour)
	_, secret, err := authStore.Create(ctx, "expired-token", &past)
	if err != nil {
		t.Fatalf("Create expired token: %v", err)
	}

	// Verifying expired token must return ok=false
	ok, err := authStore.Verify(ctx, secret)
	if err != nil || ok {
		t.Errorf("Verify expired token ok=%v, want ok=false", ok)
	}
}
