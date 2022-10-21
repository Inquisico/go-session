package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
)

// Status represents the state of the session data during a request cycle.
type Status int

const (
	// Unmodified indicates that the session data hasn't been changed in the
	// current request cycle.
	Unmodified Status = iota

	// Modified indicates that the session data has been changed in the current
	// request cycle.
	Modified

	// Destroyed indicates that the session data has been destroyed in the
	// current request cycle.
	Destroyed
)

var (
	ErrUnmodified = errors.New("unmodified")
)

type Option func(*sessionData)

func WithLifetime(lifetime time.Duration) Option {
	return func(s *sessionData) {
		s.deadline = time.Now().Add(lifetime).UTC()
	}
}

func WithDeadline(deadline time.Time) Option {
	return func(s *sessionData) {
		s.deadline = deadline
	}
}

type sessionData struct {
	deadline time.Time
	status   Status
	token    string
	values   map[string]interface{}
	mu       sync.Mutex
}

func newSessionData(lifetime time.Duration) *sessionData {
	return &sessionData{
		deadline: time.Now().Add(lifetime).UTC(),
		status:   Unmodified,
		values:   make(map[string]interface{}),
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type contextKey string

var (
	contextKeyID      uint64
	contextKeyIDMutex = &sync.Mutex{}
)

func generateContextKey() contextKey {
	contextKeyIDMutex.Lock()
	defer contextKeyIDMutex.Unlock()
	atomic.AddUint64(&contextKeyID, 1)
	return contextKey(fmt.Sprintf("session.%d", contextKeyID))
}

// Manager holds the configuration settings for your sessions.
type Manager struct {
	// IdleTimeout controls the maximum length of time a session can be inactive
	// before it expires. For example, some applications may wish to set this so
	// there is a timeout after 20 minutes of inactivity. By default IdleTimeout
	// is not set and there is no inactivity timeout.
	IdleTimeout time.Duration

	// Lifetime controls the maximum length of time that a session is valid for
	// before it expires. The lifetime is an 'absolute expiry' which is set when
	// the session is first created and does not change. The default value is 24
	// hours.
	Lifetime time.Duration

	// Store controls the session store where the session data is persisted.
	Store scs.Store

	// Cookie contains the configuration settings for session cookies.
	Cookie scs.SessionCookie

	// Codec controls the encoder/decoder used to transform session data to a
	// byte slice for use by the session store. By default session data is
	// encoded/decoded using encoding/gob.
	Codec scs.Codec

	// contextKey is the key used to set and retrieve the session data from a
	// context.Context. It's automatically generated to ensure uniqueness.
	contextKey contextKey
}

// NewManager returns a new session manager with the default options. It is safe for
// concurrent use.
func NewManager() *Manager {
	s := &Manager{
		IdleTimeout: 0,
		Lifetime:    24 * time.Hour,
		Store:       memstore.New(),
		Codec:       scs.GobCodec{},
		contextKey:  generateContextKey(),
		Cookie: scs.SessionCookie{
			Name:     "session",
			Domain:   "",
			HttpOnly: true,
			Path:     "/",
			Persist:  true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		},
	}
	return s
}

func (s *Manager) Expire(ctx context.Context, expiry time.Time) {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.deadline = expiry
	sd.status = Modified
}

// Load retrieves the session data for the given token from the session store,
// and returns a new context.Context containing the session data. If no matching
// token is found then this will create a new session.
//
// Most applications will use the LoadAndSave() middleware and will not need to
// use this method.
func (s *Manager) Load(ctx context.Context, token string, options ...Option) (context.Context, error) {
	if _, ok := ctx.Value(s.contextKey).(*sessionData); ok {
		return ctx, nil
	}

	if token == "" {
		sd := newSessionData(s.Lifetime)
		for _, option := range options {
			option(sd)
		}
		return s.addSessionDataToContext(ctx, sd), nil
	}

	b, found, err := s.doStoreFind(ctx, token)
	if err != nil {
		return ctx, err
	} else if !found {
		sd := newSessionData(s.Lifetime)
		for _, option := range options {
			option(sd)
		}
		return s.addSessionDataToContext(ctx, sd), nil
	}

	sd := &sessionData{
		status: Unmodified,
		token:  token,
	}
	if sd.deadline, sd.values, err = s.Codec.Decode(b); err != nil {
		return ctx, err
	}

	// Mark the session data as modified if an idle timeout is being used. This
	// will force the session data to be re-committed to the session store with
	// a new expiry time.
	if s.IdleTimeout > 0 {
		sd.status = Modified
	}

	return s.addSessionDataToContext(ctx, sd), nil
}

// Save checks if the session data has been Modified or Destroyed,
// and commit it if the requirements are met. If the token is Unmodified, an
// UnmodifiedErr will be returned
//
// Most applications will use the LoadAndSave() middleware and will not need to
// use this method.
func (s *Manager) Save(ctx context.Context) (string, time.Time, error) {
	switch s.Status(ctx) {
	case Modified:
		token, expiry, err := s.Commit(ctx)
		if err != nil {
			return "", time.Time{}, err
		}

		return token, expiry, nil
	case Destroyed:
		return "", time.Time{}, nil
	}

	return "", time.Time{}, ErrUnmodified
}

// Commit saves the session data to the session store and returns the session
// token and expiry time.
//
// Most applications will use the LoadAndSave() middleware and will not need to
// use this method.
func (s *Manager) Commit(ctx context.Context) (string, time.Time, error) {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	if sd.token == "" {
		var err error
		if sd.token, err = generateToken(); err != nil {
			return "", time.Time{}, err
		}
	}

	b, err := s.Codec.Encode(sd.deadline, sd.values)
	if err != nil {
		return "", time.Time{}, err
	}

	expiry := sd.deadline
	if s.IdleTimeout > 0 {
		ie := time.Now().Add(s.IdleTimeout).UTC()
		if ie.Before(expiry) {
			expiry = ie
		}
	}

	if err := s.doStoreCommit(ctx, sd.token, b, expiry); err != nil {
		return "", time.Time{}, err
	}

	return sd.token, expiry, nil
}

// Destroy deletes the session data from the session store and sets the session
// status to Destroyed. Any further operations in the same request cycle will
// result in a new session being created.
func (s *Manager) Destroy(ctx context.Context, options ...Option) error {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	err := s.doStoreDelete(ctx, sd.token)
	if err != nil {
		return err
	}

	sd.status = Destroyed

	// Reset everything else to defaults.
	sd.token = ""
	sd.deadline = time.Now().Add(s.Lifetime).UTC()
	for _, option := range options {
		option(sd)
	}
	for key := range sd.values {
		delete(sd.values, key)
	}

	return nil
}

// Put adds a key and corresponding value to the session data. Any existing
// value for the key will be replaced. The session data status will be set to
// Modified.
func (s *Manager) Put(ctx context.Context, key string, val interface{}) {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	sd.values[key] = val
	sd.status = Modified
	sd.mu.Unlock()
}

// Get returns the value for a given key from the session data. The return
// value has the type interface{} so will usually need to be type asserted
// before you can use it. For example:
//
//	foo, ok := session.Get(r, "foo").(string)
//	if !ok {
//		return errors.New("type assertion to string failed")
//	}
//
// Also see the GetString(), GetInt(), GetBytes() and other helper methods which
// wrap the type conversion for common types.
func (s *Manager) Get(ctx context.Context, key string) interface{} {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	return sd.values[key]
}

// Pop acts like a one-time Get. It returns the value for a given key from the
// session data and deletes the key and value from the session data. The
// session data status will be set to Modified. The return value has the type
// interface{} so will usually need to be type asserted before you can use it.
func (s *Manager) Pop(ctx context.Context, key string) interface{} {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	val, exists := sd.values[key]
	if !exists {
		return nil
	}
	delete(sd.values, key)
	sd.status = Modified

	return val
}

// Remove deletes the given key and corresponding value from the session data.
// The session data status will be set to Modified. If the key is not present
// this operation is a no-op.
func (s *Manager) Remove(ctx context.Context, key string) {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	_, exists := sd.values[key]
	if !exists {
		return
	}

	delete(sd.values, key)
	sd.status = Modified
}

// Clear removes all data for the current session. The session token and
// lifetime are unaffected. If there is no data in the current session this is
// a no-op.
func (s *Manager) Clear(ctx context.Context) error {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	if len(sd.values) == 0 {
		return nil
	}

	for key := range sd.values {
		delete(sd.values, key)
	}
	sd.status = Modified
	return nil
}

// Exists returns true if the given key is present in the session data.
func (s *Manager) Exists(ctx context.Context, key string) bool {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	_, exists := sd.values[key]
	sd.mu.Unlock()

	return exists
}

// Keys returns a slice of all key names present in the session data, sorted
// alphabetically. If the data contains no data then an empty slice will be
// returned.
func (s *Manager) Keys(ctx context.Context) []string {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	keys := make([]string, len(sd.values))
	i := 0
	for key := range sd.values {
		keys[i] = key
		i++
	}
	sd.mu.Unlock()

	sort.Strings(keys)
	return keys
}

// RenewToken updates the session data to have a new session token while
// retaining the current session data. The session lifetime is also reset and
// the session data status will be set to Modified.
//
// The old session token and accompanying data are deleted from the session store.
//
// To mitigate the risk of session fixation attacks, it's important that you call
// RenewToken before making any changes to privilege levels (e.g. login and
// logout operations).

// See https://github.com/OWASP/CheatSheetSeries/blob/master/cheatsheets/
// Session_Management_Cheat_Sheet.md#renew-the-session-id-after-any-privilege-level-change
// for additional information.
func (s *Manager) RenewToken(ctx context.Context, options ...Option) error {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	err := s.doStoreDelete(ctx, sd.token)
	if err != nil {
		return err
	}

	newToken, err := generateToken()
	if err != nil {
		return err
	}

	sd.token = newToken
	sd.deadline = time.Now().Add(s.Lifetime).UTC()
	for _, option := range options {
		option(sd)
	}
	sd.status = Modified

	return nil
}

// MergeSession is used to merge in data from a different session in case strict
// session tokens are lost across an oauth or similar redirect flows. Use Clear()
// if no values of the new session are to be used.
func (s *Manager) MergeSession(ctx context.Context, token string) error {
	sd := s.getSessionDataFromContext(ctx)

	b, found, err := s.doStoreFind(ctx, token)
	if err != nil {
		return err
	} else if !found {
		return nil
	}

	deadline, values, err := s.Codec.Decode(b)
	if err != nil {
		return err
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	// If it is the same session, nothing needs to be done.
	if sd.token == token {
		return nil
	}

	if deadline.After(sd.deadline) {
		sd.deadline = deadline
	}

	for k, v := range values {
		sd.values[k] = v
	}

	sd.status = Modified
	return s.doStoreDelete(ctx, token)
}

// Status returns the current status of the session data.
func (s *Manager) Status(ctx context.Context) Status {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	return sd.status
}

// GetString returns the string value for a given key from the session data.
// The zero value for a string ("") is returned if the key does not exist or the
// value could not be type asserted to a string.
func (s *Manager) GetString(ctx context.Context, key string) string {
	val := s.Get(ctx, key)
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// GetBool returns the bool value for a given key from the session data. The
// zero value for a bool (false) is returned if the key does not exist or the
// value could not be type asserted to a bool.
func (s *Manager) GetBool(ctx context.Context, key string) bool {
	val := s.Get(ctx, key)
	b, ok := val.(bool)
	if !ok {
		return false
	}
	return b
}

// GetInt returns the int value for a given key from the session data. The
// zero value for an int (0) is returned if the key does not exist or the
// value could not be type asserted to an int.
func (s *Manager) GetInt(ctx context.Context, key string) int {
	val := s.Get(ctx, key)
	i, ok := val.(int)
	if !ok {
		return 0
	}
	return i
}

// GetInt64 returns the int64 value for a given key from the session data. The
// zero value for an int64 (0) is returned if the key does not exist or the
// value could not be type asserted to an int64.
func (s *Manager) GetInt64(ctx context.Context, key string) int64 {
	val := s.Get(ctx, key)
	i, ok := val.(int64)
	if !ok {
		return 0
	}
	return i
}

// GetInt32 returns the int value for a given key from the session data. The
// zero value for an int32 (0) is returned if the key does not exist or the
// value could not be type asserted to an int32.
func (s *Manager) GetInt32(ctx context.Context, key string) int32 {
	val := s.Get(ctx, key)
	i, ok := val.(int32)
	if !ok {
		return 0
	}
	return i
}

// GetFloat returns the float64 value for a given key from the session data. The
// zero value for an float64 (0) is returned if the key does not exist or the
// value could not be type asserted to a float64.
func (s *Manager) GetFloat(ctx context.Context, key string) float64 {
	val := s.Get(ctx, key)
	f, ok := val.(float64)
	if !ok {
		return 0
	}
	return f
}

// GetBytes returns the byte slice ([]byte) value for a given key from the session
// data. The zero value for a slice (nil) is returned if the key does not exist
// or could not be type asserted to []byte.
func (s *Manager) GetBytes(ctx context.Context, key string) []byte {
	val := s.Get(ctx, key)
	b, ok := val.([]byte)
	if !ok {
		return nil
	}
	return b
}

// GetTime returns the time.Time value for a given key from the session data. The
// zero value for a time.Time object is returned if the key does not exist or the
// value could not be type asserted to a time.Time. This can be tested with the
// time.IsZero() method.
func (s *Manager) GetTime(ctx context.Context, key string) time.Time {
	val := s.Get(ctx, key)
	t, ok := val.(time.Time)
	if !ok {
		return time.Time{}
	}
	return t
}

// PopString returns the string value for a given key and then deletes it from the
// session data. The session data status will be set to Modified. The zero
// value for a string ("") is returned if the key does not exist or the value
// could not be type asserted to a string.
func (s *Manager) PopString(ctx context.Context, key string) string {
	val := s.Pop(ctx, key)
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// PopBool returns the bool value for a given key and then deletes it from the
// session data. The session data status will be set to Modified. The zero
// value for a bool (false) is returned if the key does not exist or the value
// could not be type asserted to a bool.
func (s *Manager) PopBool(ctx context.Context, key string) bool {
	val := s.Pop(ctx, key)
	b, ok := val.(bool)
	if !ok {
		return false
	}
	return b
}

// PopInt returns the int value for a given key and then deletes it from the
// session data. The session data status will be set to Modified. The zero
// value for an int (0) is returned if the key does not exist or the value could
// not be type asserted to an int.
func (s *Manager) PopInt(ctx context.Context, key string) int {
	val := s.Pop(ctx, key)
	i, ok := val.(int)
	if !ok {
		return 0
	}
	return i
}

// PopFloat returns the float64 value for a given key and then deletes it from the
// session data. The session data status will be set to Modified. The zero
// value for an float64 (0) is returned if the key does not exist or the value
// could not be type asserted to a float64.
func (s *Manager) PopFloat(ctx context.Context, key string) float64 {
	val := s.Pop(ctx, key)
	f, ok := val.(float64)
	if !ok {
		return 0
	}
	return f
}

// PopBytes returns the byte slice ([]byte) value for a given key and then
// deletes it from the from the session data. The session data status will be
// set to Modified. The zero value for a slice (nil) is returned if the key does
// not exist or could not be type asserted to []byte.
func (s *Manager) PopBytes(ctx context.Context, key string) []byte {
	val := s.Pop(ctx, key)
	b, ok := val.([]byte)
	if !ok {
		return nil
	}
	return b
}

// PopTime returns the time.Time value for a given key and then deletes it from
// the session data. The session data status will be set to Modified. The zero
// value for a time.Time object is returned if the key does not exist or the
// value could not be type asserted to a time.Time.
func (s *Manager) PopTime(ctx context.Context, key string) time.Time {
	val := s.Pop(ctx, key)
	t, ok := val.(time.Time)
	if !ok {
		return time.Time{}
	}
	return t
}

// RememberMe controls whether the session cookie is persistent (i.e  whether it
// is retained after a user closes their browser). RememberMe only has an effect
// if you have set SessionManager.Cookie.Persist = false (the default is true) and
// you are using the standard LoadAndSave() middleware.
func (s *Manager) RememberMe(ctx context.Context, val bool) {
	s.Put(ctx, "__rememberMe", val)
}

// Iterate retrieves all active (i.e. not expired) sessions from the store and
// executes the provided function fn for each session. If the session store
// being used does not support iteration then Iterate will panic.
func (s *Manager) Iterate(ctx context.Context, fn func(context.Context) error) error {
	allSessions, err := s.doStoreAll(ctx)
	if err != nil {
		return err
	}

	for token, b := range allSessions {
		sd := &sessionData{
			status: Unmodified,
			token:  token,
		}

		sd.deadline, sd.values, err = s.Codec.Decode(b)
		if err != nil {
			return err
		}

		ctx = s.addSessionDataToContext(ctx, sd)

		err = fn(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Deadline returns the 'absolute' expiry time for the session. Please note
// that if you are using an idle timeout, it is possible that a session will
// expire due to non-use before the returned deadline.
func (s *Manager) Deadline(ctx context.Context) time.Time {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	return sd.deadline
}

// Token returns the session token. Please note that this will return the
// empty string "" if it is called before the session has been committed to
// the store.
func (s *Manager) Token(ctx context.Context) string {
	sd := s.getSessionDataFromContext(ctx)

	sd.mu.Lock()
	defer sd.mu.Unlock()

	return sd.token
}

func (s *Manager) addSessionDataToContext(ctx context.Context, sd *sessionData) context.Context {
	return context.WithValue(ctx, s.contextKey, sd)
}

func (s *Manager) getSessionDataFromContext(ctx context.Context) *sessionData {
	c, ok := ctx.Value(s.contextKey).(*sessionData)
	if !ok {
		panic("scs: no session data in context")
	}
	return c
}

func (s *Manager) doStoreDelete(ctx context.Context, token string) (err error) {
	c, ok := s.Store.(interface {
		DeleteCtx(context.Context, string) error
	})
	if ok {
		return c.DeleteCtx(ctx, token)
	}
	return s.Store.Delete(token)
}

func (s *Manager) doStoreFind(ctx context.Context, token string) (b []byte, found bool, err error) {
	c, ok := s.Store.(interface {
		FindCtx(context.Context, string) ([]byte, bool, error)
	})
	if ok {
		return c.FindCtx(ctx, token)
	}
	return s.Store.Find(token)
}

func (s *Manager) doStoreCommit(ctx context.Context, token string, b []byte, expiry time.Time) (err error) {
	c, ok := s.Store.(interface {
		CommitCtx(context.Context, string, []byte, time.Time) error
	})
	if ok {
		return c.CommitCtx(ctx, token, b, expiry)
	}
	return s.Store.Commit(token, b, expiry)
}

func (s *Manager) doStoreAll(ctx context.Context) (map[string][]byte, error) {
	cs, ok := s.Store.(scs.IterableCtxStore)
	if ok {
		return cs.AllCtx(ctx)
	}

	is, ok := s.Store.(scs.IterableStore)
	if ok {
		return is.All()
	}

	panic(fmt.Sprintf("type %T does not support iteration", s.Store))
}
