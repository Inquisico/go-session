package middleware

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/inquisico/go-session"
	"github.com/rs/zerolog/log"
)

func defaultErrorFunc(w http.ResponseWriter, _ *http.Request, err error) {
	log.Error().Err(err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

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

type Option func(*HTTPSessionManager)

func WithErrorFunc(errorFunc func(http.ResponseWriter, *http.Request, error)) Option {
	return func(m *HTTPSessionManager) {
		m.errorFunc = errorFunc
	}
}

func WithCookieConfig(cookieConfig scs.SessionCookie) Option {
	return func(m *HTTPSessionManager) {
		m.cookieConfig = cookieConfig
	}
}

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
		bw := &bufferedResponseWriter{ResponseWriter: w}
		next.ServeHTTP(bw, sr)

		if sr.MultipartForm != nil {
			err := sr.MultipartForm.RemoveAll()
			s.errorFunc(w, r, err)
		}

		token, expiry, err := s.manager.Save(ctx)
		switch err {
		case nil:
			s.WriteSessionCookie(ctx, w, token, expiry)
		case session.ErrUnmodified:
		default:
			s.errorFunc(w, r, err)
			return
		}

		w.Header().Add("Vary", "Cookie")

		if bw.code != 0 {
			w.WriteHeader(bw.code)
		}
		_, err = w.Write(bw.buf.Bytes())
		log.Error().Err(err)
	})
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
		Name:     s.cookieConfig.Name,
		Value:    token,
		Path:     s.cookieConfig.Path,
		Domain:   s.cookieConfig.Domain,
		Secure:   s.cookieConfig.Secure,
		HttpOnly: s.cookieConfig.HttpOnly,
		SameSite: s.cookieConfig.SameSite,
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

type bufferedResponseWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	code        int
	wroteHeader bool
}

func (bw *bufferedResponseWriter) Write(b []byte) (int, error) {
	return bw.buf.Write(b)
}

func (bw *bufferedResponseWriter) WriteHeader(code int) {
	if !bw.wroteHeader {
		bw.code = code
		bw.wroteHeader = true
	}
}

func (bw *bufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj := bw.ResponseWriter.(http.Hijacker)
	return hj.Hijack()
}

func (bw *bufferedResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := bw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
