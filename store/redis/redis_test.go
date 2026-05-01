package redis

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T, opts ...Option) (*Store, *miniredis.Miniredis, *goredis.Client) {
	t.Helper()

	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}

	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("close redis client: %v", err)
		}
		server.Close()
	})

	return New(client, opts...), server, client
}

func TestStoreDeleteRemovesPrefixedKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, server, _ := newTestStore(t)

	if err := store.Commit(ctx, "token", []byte("payload"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit session: %v", err)
	}

	if !server.Exists("scs:session:token") {
		t.Fatal("expected prefixed session key to exist")
	}

	if err := store.Delete(ctx, "token"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if server.Exists("scs:session:token") {
		t.Fatal("expected prefixed session key to be deleted")
	}
}

func TestStoreAllReturnsUnprefixedTokens(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, _, client := newTestStore(t)

	if err := store.Commit(ctx, "token-one", []byte("alpha"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit first session: %v", err)
	}
	if err := store.Commit(ctx, "token-two", []byte("beta"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit second session: %v", err)
	}
	if err := client.Set(ctx, "unrelated:key", []byte("ignore"), 0).Err(); err != nil {
		t.Fatalf("seed unrelated key: %v", err)
	}

	sessions, err := store.All(ctx)
	if err != nil {
		t.Fatalf("iterate sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if string(sessions["token-one"]) != "alpha" {
		t.Fatalf("unexpected first session payload: %q", sessions["token-one"])
	}
	if string(sessions["token-two"]) != "beta" {
		t.Fatalf("unexpected second session payload: %q", sessions["token-two"])
	}
	if _, ok := sessions["scs:session:token-one"]; ok {
		t.Fatal("expected All to return raw session tokens, not prefixed keys")
	}
}
