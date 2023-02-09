package session

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/mockstore"
	"github.com/inquisico/session/store"
)

const (
	foo     = "foo"
	bar     = "bar"
	example = "example"
)

func TestSessionDataFromContext(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("the code did not panic")
		}
	}()

	m := NewManager()
	m.getSessionDataFromContext(context.Background())
}

func TestSessionManager_Load(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(tt *testing.T) {
		s := NewManager()
		s.IdleTimeout = time.Hour * 24

		ctx := context.Background()
		expected := example
		exampleDeadline := time.Now().Add(time.Hour)

		encodedValue, err := s.Codec.Encode(exampleDeadline, map[string]interface{}{
			"things": "stuff",
		})
		if err != nil {
			tt.Errorf("unexpected error encoding value: %v", err)
		}

		if err := s.Store.Commit(ctx, expected, encodedValue, exampleDeadline); err != nil {
			tt.Errorf("error committing to session store: %v", err)
		}

		newCtx, err := s.Load(ctx, expected)
		if err != nil {
			tt.Errorf("error loading from session manager: %v", err)
		}
		if newCtx == nil {
			tt.Error("returned context is unexpectedly nil")
		}

		sd, ok := newCtx.Value(s.contextKey).(*sessionData)
		if !ok {
			tt.Error("sessionData not present in returned context")
		}
		if sd == nil {
			tt.Error("sessionData present in returned context unexpectedly nil")
			return
		}

		actual := sd.token

		if expected != actual {
			tt.Errorf("expected %s to equal %s", expected, actual)
		}
	})

	t.Run("with preexisting session data", func(t *testing.T) {
		m := NewManager()

		obligatorySessionData := &sessionData{}
		ctx := context.WithValue(context.Background(), m.contextKey, obligatorySessionData)
		expected := example

		newCtx, err := m.Load(ctx, expected)
		if err != nil {
			t.Errorf("error loading from session manager: %v", err)
		}
		if newCtx == nil {
			t.Error("returned context is unexpectedly nil")
		}
	})

	t.Run("with empty token", func(t *testing.T) {
		m := NewManager()

		ctx := context.Background()
		expected := ""
		exampleDeadline := time.Now().Add(time.Hour)

		encodedValue, err := m.Codec.Encode(exampleDeadline, map[string]interface{}{
			"things": "stuff",
		})
		if err != nil {
			t.Errorf("unexpected error encoding value: %v", err)
		}

		if err := m.Store.Commit(ctx, expected, encodedValue, exampleDeadline); err != nil {
			t.Errorf("error committing to session store: %v", err)
		}

		newCtx, err := m.Load(ctx, "")
		if err != nil {
			t.Errorf("error loading from session manager: %v", err)
		}
		if newCtx == nil {
			t.Error("returned context is unexpectedly nil")
		}

		sd, ok := newCtx.Value(m.contextKey).(*sessionData)
		if !ok {
			t.Error("sessionData not present in returned context")
		}
		if sd == nil {
			t.Error("sessionData present in returned context unexpectedly nil")
			return
		}

		actual := sd.token

		if expected != actual {
			t.Errorf("expected %s to equal %s", expected, actual)
		}
	})

	t.Run("with error finding token in store", func(t *testing.T) {
		m := NewManager()
		s := &mockstore.MockStore{}

		ctx := context.Background()
		expected := example

		s.ExpectFind(expected, []byte{}, true, errors.New("arbitrary"))
		m.Store = store.NewStoreWrapper(s)

		newCtx, err := m.Load(ctx, expected)
		if err == nil {
			t.Errorf("no error loading from session manager: %v", err)
		}
		if newCtx != ctx {
			t.Error("returned context is unexpectedly not the old context")
		}
	})

	t.Run("with unfound token in store", func(t *testing.T) {
		m := NewManager()

		ctx := context.Background()
		exampleToken := example
		expected := ""

		newCtx, err := m.Load(ctx, exampleToken)
		if err != nil {
			t.Errorf("error loading from session manager: %v", err)
		}
		if newCtx == nil {
			t.Error("returned context is unexpectedly nil")
		}

		sd, ok := newCtx.Value(m.contextKey).(*sessionData)
		if !ok {
			t.Error("sessionData not present in returned context")
		}
		if sd == nil {
			t.Error("sessionData present in returned context unexpectedly nil")
			return
		}

		actual := sd.token

		if expected != actual {
			t.Errorf("expected %s to equal %s", expected, actual)
		}
	})

	t.Run("with error decoding found token", func(t *testing.T) {
		s := NewManager()

		ctx := context.Background()
		expected := example
		exampleDeadline := time.Now().Add(time.Hour)

		if err := s.Store.Commit(ctx, expected, []byte(""), exampleDeadline); err != nil {
			t.Errorf("error committing to session store: %v", err)
		}

		newCtx, err := s.Load(ctx, expected)
		if err == nil {
			t.Errorf("no error loading from session manager: %v", err)
		}
		if newCtx != ctx {
			t.Error("returned context is unexpectedly not the old context")
		}
	})
}

func TestSessionManager_Commit(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(tt *testing.T) {
		m := NewManager()
		m.IdleTimeout = time.Hour * 24

		expectedToken := example
		expectedExpiry := time.Now().Add(time.Hour)

		ctx := context.WithValue(context.Background(), m.contextKey, &sessionData{
			deadline: expectedExpiry,
			token:    expectedToken,
			values: map[string]interface{}{
				"blah": "blah",
			},
			mu: sync.Mutex{},
		})

		actualToken, actualExpiry, err := m.Commit(ctx)
		if expectedToken != actualToken {
			tt.Errorf("expected token to equal %q, but received %q", expectedToken, actualToken)
		}
		if expectedExpiry != actualExpiry {
			tt.Errorf("expected expiry to equal %v, but received %v", expectedExpiry, actualExpiry)
		}
		if err != nil {
			tt.Errorf("unexpected error returned: %v", err)
		}
	})

	t.Run("with empty token", func(t *testing.T) {
		m := NewManager()
		m.IdleTimeout = time.Hour * 24

		expectedToken := "XO6_D4NBpGP3D_BtekxTEO6o2ZvOzYnArauSQbgg" // #nosec
		expectedExpiry := time.Now().Add(time.Hour)

		ctx := context.WithValue(context.Background(), m.contextKey, &sessionData{
			deadline: expectedExpiry,
			token:    expectedToken,
			values: map[string]interface{}{
				"blah": "blah",
			},
			mu: sync.Mutex{},
		})

		actualToken, actualExpiry, err := m.Commit(ctx)
		if expectedToken != actualToken {
			t.Errorf("expected token to equal %q, but received %q", expectedToken, actualToken)
		}
		if expectedExpiry != actualExpiry {
			t.Errorf("expected expiry to equal %v, but received %v", expectedExpiry, actualExpiry)
		}
		if err != nil {
			t.Errorf("unexpected error returned: %v", err)
		}
	})

	t.Run("with expired deadline", func(t *testing.T) {
		m := NewManager()
		m.IdleTimeout = time.Millisecond

		expectedToken := example
		expectedExpiry := time.Now().Add(time.Hour * -100)

		ctx := context.WithValue(context.Background(), m.contextKey, &sessionData{
			deadline: time.Now().Add(time.Hour * 24),
			token:    expectedToken,
			values: map[string]interface{}{
				"blah": "blah",
			},
			mu: sync.Mutex{},
		})

		actualToken, actualExpiry, err := m.Commit(ctx)
		if expectedToken != actualToken {
			t.Errorf("expected token to equal %q, but received %q", expectedToken, actualToken)
		}
		if expectedExpiry == actualExpiry {
			t.Errorf("expected expiry not to equal %v", actualExpiry)
		}
		if err != nil {
			t.Errorf("unexpected error returned: %v", err)
		}
	})

	t.Run("with error committing to store", func(t *testing.T) {
		m := NewManager()
		m.IdleTimeout = time.Hour * 24

		s := &mockstore.MockStore{}
		expectedErr := errors.New("arbitrary")

		sd := &sessionData{
			deadline: time.Now().Add(time.Hour),
			token:    example,
			values: map[string]interface{}{
				"blah": "blah",
			},
			mu: sync.Mutex{},
		}
		expectedBytes, err := m.Codec.Encode(sd.deadline, sd.values)
		if err != nil {
			t.Errorf("unexpected encode error: %v", err)
		}

		ctx := context.WithValue(context.Background(), m.contextKey, sd)

		s.ExpectCommit(sd.token, expectedBytes, sd.deadline, expectedErr)
		m.Store = store.NewStoreWrapper(s)

		actualToken, _, err := m.Commit(ctx)
		if actualToken != "" {
			t.Error("expected empty token")
		}
		if err == nil {
			t.Error("expected error not returned")
		}
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	ctx := m.addSessionDataToContext(context.Background(), sd)

	m.Put(ctx, foo, bar)

	if sd.values[foo] != bar {
		t.Errorf("got %q: expected %q", sd.values[foo], bar)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	ctx := m.addSessionDataToContext(context.Background(), sd)

	str, ok := m.Get(ctx, foo).(string)
	if !ok {
		t.Errorf("could not convert %T to string", m.Get(ctx, foo))
	}

	if str != bar {
		t.Errorf("got %q: expected %q", str, bar)
	}
}

func TestPop(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	ctx := m.addSessionDataToContext(context.Background(), sd)

	str, ok := m.Pop(ctx, foo).(string)
	if !ok {
		t.Errorf("could not convert %T to string", m.Get(ctx, foo))
	}

	if str != bar {
		t.Errorf("got %q: expected %q", str, bar)
	}

	_, ok = sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}
}

func TestRemove(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	ctx := m.addSessionDataToContext(context.Background(), sd)

	m.Remove(ctx, foo)

	if sd.values[foo] != nil {
		t.Errorf("got %v: expected %v", sd.values[foo], nil)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}
}

func TestClear(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	sd.values["baz"] = "boz"
	ctx := m.addSessionDataToContext(context.Background(), sd)

	if err := m.Clear(ctx); err != nil {
		t.Errorf("unexpected error encountered clearing session: %v", err)
	}

	if sd.values[foo] != nil {
		t.Errorf("got %v: expected %v", sd.values[foo], nil)
	}

	if sd.values["baz"] != nil {
		t.Errorf("got %v: expected %v", sd.values["baz"], nil)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}
}

func TestExists(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	ctx := m.addSessionDataToContext(context.Background(), sd)

	if !m.Exists(ctx, foo) {
		t.Errorf("got %v: expected %v", m.Exists(ctx, foo), true)
	}

	if m.Exists(ctx, "baz") {
		t.Errorf("got %v: expected %v", m.Exists(ctx, "baz"), false)
	}
}

func TestKeys(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	sd.values["woo"] = "waa"
	ctx := m.addSessionDataToContext(context.Background(), sd)

	keys := m.Keys(ctx)
	if !reflect.DeepEqual(keys, []string{foo, "woo"}) {
		t.Errorf("got %v: expected %v", keys, []string{foo, "woo"})
	}
}

func TestGetString(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	ctx := m.addSessionDataToContext(context.Background(), sd)

	str := m.GetString(ctx, foo)
	if str != bar {
		t.Errorf("got %q: expected %q", str, bar)
	}

	str = m.GetString(ctx, "baz")
	if str != "" {
		t.Errorf("got %q: expected %q", str, "")
	}
}

func TestGetBool(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = true
	ctx := m.addSessionDataToContext(context.Background(), sd)

	b := m.GetBool(ctx, foo)
	if b != true {
		t.Errorf("got %v: expected %v", b, true)
	}

	b = m.GetBool(ctx, "baz")
	if b != false {
		t.Errorf("got %v: expected %v", b, false)
	}
}

func TestGetInt(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = 123
	ctx := m.addSessionDataToContext(context.Background(), sd)

	i := m.GetInt(ctx, foo)
	if i != 123 {
		t.Errorf("got %v: expected %d", i, 123)
	}

	i = m.GetInt(ctx, "baz")
	if i != 0 {
		t.Errorf("got %v: expected %d", i, 0)
	}
}

func TestGetFloat(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = 123.456
	ctx := m.addSessionDataToContext(context.Background(), sd)

	f := m.GetFloat(ctx, foo)
	if f != 123.456 {
		t.Errorf("got %v: expected %f", f, 123.456)
	}

	f = m.GetFloat(ctx, "baz")
	if f != 0 {
		t.Errorf("got %v: expected %f", f, 0.00)
	}
}

func TestGetBytes(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = []byte(bar)
	ctx := m.addSessionDataToContext(context.Background(), sd)

	b := m.GetBytes(ctx, foo)
	if !bytes.Equal(b, []byte(bar)) {
		t.Errorf("got %v: expected %v", b, []byte(bar))
	}

	b = m.GetBytes(ctx, "baz")
	if b != nil {
		t.Errorf("got %v: expected %v", b, nil)
	}
}

func TestGetTime(t *testing.T) {
	t.Parallel()

	now := time.Now()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = now
	ctx := m.addSessionDataToContext(context.Background(), sd)

	tm := m.GetTime(ctx, foo)
	if tm != now {
		t.Errorf("got %v: expected %v", tm, now)
	}

	tm = m.GetTime(ctx, "baz")
	if !tm.IsZero() {
		t.Errorf("got %v: expected %v", tm, time.Time{})
	}
}

func TestPopString(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = bar
	ctx := m.addSessionDataToContext(context.Background(), sd)

	str := m.PopString(ctx, foo)
	if str != bar {
		t.Errorf("got %q: expected %q", str, bar)
	}

	_, ok := sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}

	str = m.PopString(ctx, bar)
	if str != "" {
		t.Errorf("got %q: expected %q", str, "")
	}
}

func TestPopBool(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = true
	ctx := m.addSessionDataToContext(context.Background(), sd)

	b := m.PopBool(ctx, foo)
	if b != true {
		t.Errorf("got %v: expected %v", b, true)
	}

	_, ok := sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}

	b = m.PopBool(ctx, bar)
	if b != false {
		t.Errorf("got %v: expected %v", b, false)
	}
}

func TestPopInt(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = 123
	ctx := m.addSessionDataToContext(context.Background(), sd)

	i := m.PopInt(ctx, foo)
	if i != 123 {
		t.Errorf("got %d: expected %d", i, 123)
	}

	_, ok := sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}

	i = m.PopInt(ctx, bar)
	if i != 0 {
		t.Errorf("got %d: expected %d", i, 0)
	}
}

func TestPopFloat(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = 123.456
	ctx := m.addSessionDataToContext(context.Background(), sd)

	f := m.PopFloat(ctx, foo)
	if f != 123.456 {
		t.Errorf("got %f: expected %f", f, 123.456)
	}

	_, ok := sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}

	f = m.PopFloat(ctx, bar)
	if f != 0.0 {
		t.Errorf("got %f: expected %f", f, 0.0)
	}
}

func TestPopBytes(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = []byte(bar)
	ctx := m.addSessionDataToContext(context.Background(), sd)

	b := m.PopBytes(ctx, foo)
	if !bytes.Equal(b, []byte(bar)) {
		t.Errorf("got %v: expected %v", b, []byte(bar))
	}
	_, ok := sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}

	b = m.PopBytes(ctx, bar)
	if b != nil {
		t.Errorf("got %v: expected %v", b, nil)
	}
}

func TestPopTime(t *testing.T) {
	t.Parallel()

	now := time.Now()
	m := NewManager()
	sd := newSessionData(time.Hour)
	sd.values[foo] = now
	ctx := m.addSessionDataToContext(context.Background(), sd)

	tm := m.PopTime(ctx, foo)
	if tm != now {
		t.Errorf("got %v: expected %v", tm, now)
	}

	_, ok := sd.values[foo]
	if ok {
		t.Errorf("got %v: expected %v", ok, false)
	}

	if sd.status != Modified {
		t.Errorf("got %v: expected %v", sd.status, "modified")
	}

	tm = m.PopTime(ctx, "baz")
	if !tm.IsZero() {
		t.Errorf("got %v: expected %v", tm, time.Time{})
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()

	m := NewManager()
	sd := newSessionData(time.Hour)
	ctx := m.addSessionDataToContext(context.Background(), sd)

	status := m.Status(ctx)
	if status != Unmodified {
		t.Errorf("got %d: expected %d", status, Unmodified)
	}

	m.Put(ctx, foo, bar)
	status = m.Status(ctx)
	if status != Modified {
		t.Errorf("got %d: expected %d", status, Modified)
	}

	if err := m.Destroy(ctx); err != nil {
		t.Errorf("unexpected error destroying session data: %v", err)
	}

	status = m.Status(ctx)
	if status != Destroyed {
		t.Errorf("got %d: expected %d", status, Destroyed)
	}
}
