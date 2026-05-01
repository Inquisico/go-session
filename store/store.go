// Package store defines the interfaces used by session stores.
package store

import (
	"context"
	"time"
)

// Store is an interface for session stores.
type Store interface {
	// Delete removes a session token and its data from the store.
	Delete(ctx context.Context, token string) (err error)

	// Find returns the data for the given session token. If the token is not
	// found or has expired, found is false and err is nil.
	Find(ctx context.Context, token string) (b []byte, found bool, err error)

	// Commit stores the given session data under the token, replacing any
	// previous value, and sets the entry's expiry time.
	Commit(ctx context.Context, token string, b []byte, expiry time.Time) (err error)

	// All returns the data for every active (non-expired) session keyed by
	// token. Implementations that do not support iteration should return an
	// error.
	All(ctx context.Context) (map[string][]byte, error)
}
