package redis

import (
	"context"
	"errors"
	"regexp"
	"time"
	"unsafe"

	"github.com/go-redis/redis/v8"
)

// stringToBytes converts string to byte slice.
func stringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}

// Store represents the redis session store.
type Store struct {
	client        *redis.Client
	prefix        string
	prefixEscaped string
}

type Options func(*Store)

// WithPrefix sets the parameter that controls the Redis key
// prefix, which can be used to avoid naming clashes if necessary.
func WithPrefix(prefix string) Options {
	return func(s *Store) {
		s.prefix = prefix
		s.prefixEscaped = regexp.QuoteMeta(prefix)
	}
}

// New returns a new RedisStore instance. The pool parameter should be a pointer
// to a redigo connection pool. See https://godoc.org/github.com/gomodule/redigo/redis#Pool.
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
	cmd := s.client.Get(ctx, token)
	result, err := cmd.Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}

	return result, true, nil
}

// Commit adds a session token and data to the RedisStore instance with the
// given expiry time. If the session token already exists then the data and
// expiry time are updated.
func (s *Store) Commit(ctx context.Context, token string, b []byte, expiry time.Time) error {
	cmd := s.client.SetArgs(ctx, s.prefix+token, b, redis.SetArgs{ExpireAt: expiry})
	return cmd.Err()
}

// Delete removes a session token and corresponding data from the RedisStore
// instance.
func (s *Store) Delete(ctx context.Context, token string) error {
	cmd := s.client.Del(ctx, token)
	return cmd.Err()
}

// All returns a map containing the token and data for all active (i.e.
// not expired) sessions in the RedisStore instance.
func (s *Store) All(ctx context.Context) (map[string][]byte, error) {
	keysCmd := s.client.Keys(ctx, s.prefixEscaped+"*")
	keys, err := keysCmd.Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	// Check if empty
	keyLength := len(keys)
	if keyLength == 0 {
		return nil, nil
	}

	cmd := s.client.MGet(ctx, keys...)
	values, err := cmd.Result()
	if err != nil {
		return nil, err
	}

	if keyLength != len(values) {
		return nil, errors.New("length of keys and values do not match")
	}

	sessions := make(map[string][]byte)
	for n, k := range keys {
		v := values[n]
		if v == redis.Nil {
			continue
		}

		sessions[k] = stringToBytes(v.(string))
	}

	return sessions, nil
}
