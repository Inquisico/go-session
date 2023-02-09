package store

import (
	"context"
	"time"
)

// Store is an interface for session stores.
type Store interface {
	// DeleteCtx is the same as Store.Delete, except it takes a context.Context.
	Delete(ctx context.Context, token string) (err error)

	// FindCtx is the same as Store.Find, except it takes a context.Context.
	Find(ctx context.Context, token string) (b []byte, found bool, err error)

	// CommitCtx is the same as Store.Commit, except it takes a context.Context.
	Commit(ctx context.Context, token string, b []byte, expiry time.Time) (err error)

	// AllCtx is the same as IterableStore.All, expect it takes a
	// context.Context.
	All(ctx context.Context) (map[string][]byte, error)
}
