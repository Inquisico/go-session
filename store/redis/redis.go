// Package redis provides a Redis-backed session store.
package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store represents the redis session store.
type Store struct {
	client *redis.Client
	prefix string
}

// Options configures a Redis-backed session store.
type Options func(*Store)

// WithPrefix sets the parameter that controls the Redis key
// prefix, which can be used to avoid naming clashes if necessary.
func WithPrefix(prefix string) Options {
	return func(s *Store) {
		s.prefix = prefix
	}
}

// New returns a new Redis-backed session store.
func New(client *redis.Client, opts ...Options) *Store {
	store := &Store{
		client: client,
		prefix: "scs:session:",
	}

	for _, opt := range opts {
		opt(store)
	}

	return store
}

// Find returns the data for a given session token from the RedisStore instance.
// If the session token is not found or is expired, the returned exists flag
// will be set to false.
func (s *Store) Find(ctx context.Context, token string) (b []byte, exists bool, err error) {
	cmd := s.client.Get(ctx, s.prefix+token)
	result, err := cmd.Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("find session %q: %w", token, err)
	}

	return result, true, nil
}

// Commit adds a session token and data to the RedisStore instance with the
// given expiry time. If the session token already exists then the data and
// expiry time are updated.
func (s *Store) Commit(ctx context.Context, token string, b []byte, expiry time.Time) error {
	cmd := s.client.SetArgs(ctx, s.prefix+token, b, redis.SetArgs{ExpireAt: expiry})
	if err := cmd.Err(); err != nil {
		return fmt.Errorf("commit session %q: %w", token, err)
	}

	return nil
}

// Delete removes a session token and corresponding data from the RedisStore
// instance.
func (s *Store) Delete(ctx context.Context, token string) error {
	cmd := s.client.Del(ctx, s.prefix+token)
	if err := cmd.Err(); err != nil {
		return fmt.Errorf("delete session %q: %w", token, err)
	}

	return nil
}

// All returns a map containing the token and data for all active (i.e.
// not expired) sessions in the RedisStore instance.
func (s *Store) All(ctx context.Context) (map[string][]byte, error) {
	iter := s.client.Scan(ctx, 0, s.prefix+"*", 0).Iterator()
	keys := make([]string, 0)
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan sessions: %w", err)
	}

	if len(keys) == 0 {
		return nil, nil
	}

	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, 0, len(keys))
	for _, key := range keys {
		cmds = append(cmds, pipe.Get(ctx, key))
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("load sessions: %w", err)
	}

	sessions := make(map[string][]byte, len(keys))
	for index, key := range keys {
		value, err := cmds[index].Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read session %q: %w", strings.TrimPrefix(key, s.prefix), err)
		}

		token := strings.TrimPrefix(key, s.prefix)
		sessions[token] = value
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	return sessions, nil
}
