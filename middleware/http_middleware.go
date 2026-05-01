// Package middleware provides HTTP middleware for loading and saving sessions.
package middleware

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/rs/zerolog/log"

	"github.com/inquisico/go-session"
)

func defaultErrorFunc(w http.ResponseWriter, _ *http.Request, err error) {
	log.Error().Err(err).Msg("session middleware error")
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// HTTPSessionManager integrates a session manager with net/http middleware.
type HTTPSessionManager struct {
	manager *session.Manager

	// cookieConfig contains the configuration settings for session cookies.
	cookieConfig scs.SessionCookie

	// errorFunc allows you to control behavior when an error is encountered by
	// the LoadAndSave middleware. The default behavior is for a HTTP 500
	// "Internal Server Error" message to be sent to the client and the error
	// logged using Go's standard logger. If a custom errorFunc is set, then
	// control will be passed to this instead. A typical use would be to provide
	// a function which logs the error and returns a customized HTML error page.
	errorFunc func(http.ResponseWriter, *http.Request, error)
}

// Option configures an HTTPSessionManager.
type Option func(*HTTPSessionManager)

// WithErrorFunc sets the function used to handle middleware errors.
func WithErrorFunc(errorFunc func(http.ResponseWriter, *http.Request, error)) Option {
	return func(m *HTTPSessionManager) {
		m.errorFunc = errorFunc
	}
}

// WithCookieConfig sets the cookie configuration used for session cookies.
func WithCookieConfig(cookieConfig scs.SessionCookie) Option {
	return func(m *HTTPSessionManager) {
		m.cookieConfig = cookieConfig
	}
}

// NewHTTPSessionManager returns a new HTTP session middleware manager.
func NewHTTPSessionManager(manager *session.Manager, opts ...Option) *HTTPSessionManager {
	m := &HTTPSessionManager{
		manager:   manager,
		errorFunc: defaultErrorFunc,
		cookieConfig: scs.SessionCookie{
			Name:     "session",
			Domain:   "",
			HttpOnly: true,
			Path:     "/",
			Persist:  true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		},
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// LoadAndSave provides middleware which automatically loads and saves session
// data for the current request, and communicates the session token to and from
// the client in a cookie.
func (s *HTTPSessionManager) LoadAndSave(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Cookie")

		var token string
		cookie, err := r.Cookie(s.cookieConfig.Name)
		if err == nil {
			token = cookie.Value
		}

		ctx, err := s.manager.Load(r.Context(), token)
		if err != nil {
			s.errorFunc(w, r, err)
			return
		}

		sr := r.WithContext(ctx)
		sw := &sessionResponseWriter{
			ResponseWriter: w,
			request:        sr,
			sessionManager: s,
		}
		next.ServeHTTP(sw, sr)

		if sr.MultipartForm != nil {
			if err := sr.MultipartForm.RemoveAll(); err != nil {
				if sw.written {
					log.Error().Err(err).Msg("remove multipart form")
				} else {
					s.errorFunc(w, r, fmt.Errorf("remove multipart form: %w", err))
				}
				return
			}
		}

		if !sw.written {
			if err := s.commitAndWriteSessionCookie(w, sr); err != nil {
				s.errorFunc(w, r, err)
				return
			}
		}
	})
}

func (s *HTTPSessionManager) commitAndWriteSessionCookie(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	token, expiry, err := s.manager.Save(ctx)
	if err != nil {
		if errors.Is(err, session.ErrUnmodified) {
			return nil
		}

		return fmt.Errorf("save session: %w", err)
	}

	s.WriteSessionCookie(ctx, w, token, expiry)

	return nil
}

// WriteSessionCookie writes a cookie to the HTTP response with the provided
// token as the cookie value and expiry as the cookie expiry time. The expiry
// time will be included in the cookie only if the session is set to persist
// or has had RememberMe(true) called on it. If expiry is an empty time.Time
// struct (so that it's IsZero() method returns true) the cookie will be
// marked with a historical expiry time and negative max-age (so the browser
// deletes it).
//
// Most applications will use the LoadAndSave() middleware and will not need to
// use this method.
func (s *HTTPSessionManager) WriteSessionCookie(ctx context.Context, w http.ResponseWriter, token string,
	expiry time.Time) {
	cookie := &http.Cookie{
		Name:        s.cookieConfig.Name,
		Value:       token,
		Path:        s.cookieConfig.Path,
		Domain:      s.cookieConfig.Domain,
		Secure:      s.cookieConfig.Secure,
		HttpOnly:    s.cookieConfig.HttpOnly,
		Partitioned: s.cookieConfig.Partitioned,
		SameSite:    s.cookieConfig.SameSite,
	}

	if expiry.IsZero() {
		cookie.Expires = time.Unix(1, 0)
		cookie.MaxAge = -1
	} else if s.cookieConfig.Persist || s.manager.GetBool(ctx, "__rememberMe") {
		cookie.Expires = time.Unix(expiry.Unix()+1, 0)        // Round up to the nearest second.
		cookie.MaxAge = int(time.Until(expiry).Seconds() + 1) // Round up to the nearest second.
	}

	w.Header().Add("Set-Cookie", cookie.String())
	w.Header().Add("Cache-Control", `no-cache="Set-Cookie"`)
}

type sessionResponseWriter struct {
	http.ResponseWriter
	request        *http.Request
	sessionManager *HTTPSessionManager
	written        bool
}

func (sw *sessionResponseWriter) Write(b []byte) (int, error) {
	if !sw.written {
		if err := sw.sessionManager.commitAndWriteSessionCookie(sw.ResponseWriter, sw.request); err != nil {
			sw.sessionManager.errorFunc(sw.ResponseWriter, sw.request, err)
			return 0, err
		}
		sw.written = true
	}

	n, err := sw.ResponseWriter.Write(b)
	if err != nil {
		return n, fmt.Errorf("write response body: %w", err)
	}

	return n, nil
}

func (sw *sessionResponseWriter) WriteHeader(code int) {
	if !sw.written {
		if err := sw.sessionManager.commitAndWriteSessionCookie(sw.ResponseWriter, sw.request); err != nil {
			sw.sessionManager.errorFunc(sw.ResponseWriter, sw.request, err)
			return
		}
		sw.written = true
	}

	sw.ResponseWriter.WriteHeader(code)
}

func (sw *sessionResponseWriter) Flush() {
	if !sw.written {
		if err := sw.sessionManager.commitAndWriteSessionCookie(sw.ResponseWriter, sw.request); err != nil {
			sw.sessionManager.errorFunc(sw.ResponseWriter, sw.request, err)
			return
		}
		sw.written = true
	}

	if flusher, ok := sw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (sw *sessionResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if !sw.written {
		if err := sw.sessionManager.commitAndWriteSessionCookie(sw.ResponseWriter, sw.request); err != nil {
			return nil, nil, err
		}
		sw.written = true
	}

	hj, ok := sw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, fmt.Errorf("hijack response: %w", err)
	}

	return conn, rw, nil
}

func (sw *sessionResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := sw.ResponseWriter.(http.Pusher); ok {
		if err := pusher.Push(target, opts); err != nil {
			return fmt.Errorf("push response: %w", err)
		}

		return nil
	}
	return http.ErrNotSupported
}

func (sw *sessionResponseWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}
