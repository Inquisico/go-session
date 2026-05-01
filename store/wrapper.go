package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexedwards/scs/v2"
)

// Wrapper adapts an scs store to this package's context-aware store interface.
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
	b, found, err := s.store.Find(token)
	if err != nil {
		return nil, false, fmt.Errorf("find session %q: %w", token, err)
	}

	return b, found, nil
}

// Commit adds a session token and data to the MemStore instance with the given
// expiry time. If the session token already exists, then the data and expiry
// time are updated.
func (s *Wrapper) Commit(_ context.Context, token string, b []byte, expiry time.Time) error {
	if err := s.store.Commit(token, b, expiry); err != nil {
		return fmt.Errorf("commit session %q: %w", token, err)
	}

	return nil
}

// Delete removes a session token and corresponding data from the MemStore
// instance.
func (s *Wrapper) Delete(_ context.Context, token string) error {
	if err := s.store.Delete(token); err != nil {
		return fmt.Errorf("delete session %q: %w", token, err)
	}

	return nil
}

// All returns a map containing the token and data for all active (i.e.
// not expired) sessions.
func (s *Wrapper) All(ctx context.Context) (map[string][]byte, error) {
	cs, ok := s.store.(scs.IterableCtxStore)
	if ok {
		allSessions, err := cs.AllCtx(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sessions: %w", err)
		}

		return allSessions, nil
	}

	is, ok := s.store.(scs.IterableStore)
	if ok {
		allSessions, err := is.All()
		if err != nil {
			return nil, fmt.Errorf("list sessions: %w", err)
		}

		return allSessions, nil
	}

	return nil, errors.New("this store does not support iteration")
}
