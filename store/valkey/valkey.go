// Package valkey provides a Valkey-backed session store.
package valkey

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

// Store represents the Valkey session store.
type Store struct {
	client valkey.Client
	prefix string
}

// Option configures a Valkey-backed session store.
type Option func(*Store)

// WithPrefix sets the parameter that controls the Valkey key
// prefix, which can be used to avoid naming clashes if necessary.
func WithPrefix(prefix string) Option {
	return func(s *Store) {
		s.prefix = prefix
	}
}

// New returns a new Valkey-backed session store.
func New(client valkey.Client, opts ...Option) *Store {
	store := &Store{
		client: client,
		prefix: "scs:session:",
	}

	for _, opt := range opts {
		opt(store)
	}

	return store
}

// Find returns the data for a given session token from the Valkey store.
// If the session token is not found or is expired, the returned found flag
// will be set to false.
func (s *Store) Find(ctx context.Context, token string) ([]byte, bool, error) {
	result, err := s.client.Do(ctx, s.client.B().Get().Key(s.prefix+token).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("find session %q: %w", token, err)
	}

	return result, true, nil
}

// Commit adds a session token and data to the Valkey store with the given
// expiry time. If the session token already exists then the data and expiry
// time are updated.
func (s *Store) Commit(ctx context.Context, token string, b []byte, expiry time.Time) error {
	cmd := s.client.B().Set().Key(s.prefix + token).Value(valkey.BinaryString(b)).Pxat(expiry).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("commit session %q: %w", token, err)
	}

	return nil
}

// Delete removes a session token and corresponding data from the Valkey store.
func (s *Store) Delete(ctx context.Context, token string) error {
	cmd := s.client.B().Del().Key(s.prefix + token).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("delete session %q: %w", token, err)
	}

	return nil
}

// All returns a map containing the token and data for all active (i.e.
// not expired) sessions in the Valkey store.
func (s *Store) All(ctx context.Context) (map[string][]byte, error) {
	pattern := s.prefix + "*"
	keys := make([]string, 0)
	cursor := uint64(0)

	for {
		entry, err := s.client.Do(ctx, s.client.B().Scan().Cursor(cursor).Match(pattern).Count(500).Build()).AsScanEntry()
		if err != nil {
			return nil, fmt.Errorf("scan sessions: %w", err)
		}

		keys = append(keys, entry.Elements...)
		cursor = entry.Cursor
		if cursor == 0 {
			break
		}
	}

	if len(keys) == 0 {
		return map[string][]byte{}, nil
	}

	cmds := make(valkey.Commands, 0, len(keys))
	for _, key := range keys {
		cmds = append(cmds, s.client.B().Get().Key(key).Build())
	}

	responses := s.client.DoMulti(ctx, cmds...)

	sessions := make(map[string][]byte, len(keys))
	for index, key := range keys {
		value, err := responses[index].AsBytes()
		if err != nil {
			if valkey.IsValkeyNil(err) {
				continue
			}
			return nil, fmt.Errorf("read session %q: %w", strings.TrimPrefix(key, s.prefix), err)
		}

		token := strings.TrimPrefix(key, s.prefix)
		sessions[token] = value
	}

	return sessions, nil
}
