# Session Management

[![Go Lint](https://github.com/inquisico/go-session/actions/workflows/golangci-lint-push.yaml/badge.svg)](https://github.com/inquisico/go-session/actions/workflows/golangci-lint-push.yaml) [![Go Test](https://github.com/inquisico/go-session/actions/workflows/go-test-push.yaml/badge.svg)](https://github.com/inquisico/go-session/actions/workflows/go-test-push.yaml) [![Release Drafter](https://github.com/inquisico/go-session/actions/workflows/release-drafter.yaml/badge.svg)](https://github.com/inquisico/go-session/actions/workflows/release-drafter.yaml)

Session implements a server-side session management pattern informed by the [OWASP session management guidance](https://github.com/OWASP/CheatSheetSeries/blob/master/cheatsheets/Session_Management_Cheat_Sheet.md). Session data is stored on the server, and a randomly-generated unique session token (or *session ID*) is communicated to and from the client in a session cookie. This package is based on [alexedwards/scs](https://github.com/alexedwards/scs).

## Why go-session

We wanted to provide a package that was more extensible, flexible, and has additional features. By using sound coding patterns, our package allows you to easily substitute one middleware for another, for example for different HTTP servers such as Echo, Fiber, and Gin. You may also extend on existing one to provide more features. We also added the ability to customize every new session. If you would like to contribute, please open an issue with a feature request, or a PR directly if you think you have a fantastic new feature.

## Usage

From your terminal, run:
```
$ go get github.com/inquisico/go-session
```

### Code example

```go
import (
    "net/http"
    "time"

    "github.com/alexedwards/scs/v2"
    "github.com/alexedwards/scs/v2/memstore"
    session "github.com/inquisico/go-session"
    sessionmiddleware "github.com/inquisico/go-session/middleware"
    "github.com/inquisico/go-session/store"
)

func main() {
    sessionManager := session.NewManager(
        session.WithDefaultTTL(time.Second), // Optional
        session.WithDefaultIdleTimeout(200*time.Millisecond), // Optional
        session.WithStore(store.NewWrapper(memstore.New())) // Optional (note: you will need to wrap the stores when using stores from github.com/alexedwards/scs)
    )

    cookieConfig := scs.SessionCookie{
        Name:     "session",
        Domain:   "",
        HttpOnly: true,
        Path:     "/",
        Persist:  true,
        Secure:   true,
        SameSite: http.SameSiteStrictMode,
    }

    httpSessionManager := sessionmiddleware.NewHTTPSessionManager(
        sessionManager,
        sessionmiddleware.WithCookieConfig(cookieConfig), // Optional
    )

    // Put `httpSessionManager` into your http server
    // See: https://www.alexedwards.net/blog/making-and-using-middleware
    // ...
}
```

For local HTTP development, set `Secure` to `false`. If your application depends on cross-site redirects or OAuth callbacks, `SameSite=Lax` or `SameSite=None` may be more appropriate than `Strict`.

## Creating your own store

The store interface can be found in `store/store.go`. You can implement your own store by satisfying that interface. See [go-session/store](https://github.com/inquisico/go-session/tree/main/store) for examples.

## Compatible session stores

Inquisico-managed session stores can be found at [go-session/store](https://github.com/inquisico/go-session/tree/main/store). If you require a more extensive set of session stores, you may check out [more compatible session stores](https://github.com/alexedwards/scs#configuring-the-session-store) for your desired store.
