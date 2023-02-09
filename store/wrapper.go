package store

import (
	"context"
	"errors"
	"time"

	"github.com/alexedwards/scs/v2"
)

type Wrapper struct {
	store scs.Store
}

// NewWrapper returns a new context store that wraps a store that does
// not support contexts.
func NewWrapper(store scs.Store) *Wrapper {
	return &Wrapper{
		store: store,
	}
}

// Find returns the data for a given session token from the MemStore instance.
// If the session token is not found or is expired, the returned exists flag will
// be set to false.
func (s *Wrapper) Find(_ context.Context, token string) ([]byte, bool, error) {
	return s.store.Find(token)
}

// Commit adds a session token and data to the MemStore instance with the given
// expiry time. If the session token already exists, then the data and expiry
// time are updated.
func (s *Wrapper) Commit(_ context.Context, token string, b []byte, expiry time.Time) error {
	return s.store.Commit(token, b, expiry)
}

// Delete removes a session token and corresponding data from the MemStore
// instance.
func (s *Wrapper) Delete(_ context.Context, token string) error {
	return s.store.Delete(token)
}

// All returns a map containing the token and data for all active (i.e.
// not expired) sessions.
func (s *Wrapper) All(ctx context.Context) (map[string][]byte, error) {
	cs, ok := s.store.(scs.IterableCtxStore)
	if ok {
		return cs.AllCtx(ctx)
	}

	is, ok := s.store.(scs.IterableStore)
	if ok {
		return is.All()
	}

	return nil, errors.New("This store does not support iteration")
}
