package valkey

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
)

// miniredis backs these tests because the store only uses RESP commands
// (GET/SET/SCAN/DEL with PXAT) that are identical across Redis and Valkey.
// If Valkey-specific commands are added, swap in a real Valkey instance.
func newTestStore(t *testing.T, opts ...Option) (*Store, *miniredis.Miniredis, valkey.Client) {
	t.Helper()

	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{server.Addr()},
		DisableCache: true,
	})
	if err != nil {
		server.Close()
		t.Fatalf("create valkey client: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
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

func TestStoreFindReturnsCommittedPayload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, _, _ := newTestStore(t)

	if err := store.Commit(ctx, "token", []byte("payload"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit session: %v", err)
	}

	got, found, err := store.Find(ctx, "token")
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	if !found {
		t.Fatal("expected session to be found")
	}
	if string(got) != "payload" {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestStoreFindMissingTokenReturnsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, _, _ := newTestStore(t)

	_, found, err := store.Find(ctx, "missing")
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	if found {
		t.Fatal("expected missing token to report not found")
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
	if err := client.Do(ctx, client.B().Set().Key("unrelated:key").Value("ignore").Build()).Error(); err != nil {
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

func TestStoreWithPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, server, _ := newTestStore(t, WithPrefix("custom:"))

	if err := store.Commit(ctx, "token", []byte("payload"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit session: %v", err)
	}

	if !server.Exists("custom:token") {
		t.Fatal("expected key to use custom prefix")
	}
}
