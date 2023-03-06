# Session Management

[![Go Lint](https://github.com/inquisico/go-session/actions/workflows/golangci-lint-push.yaml/badge.svg)](https://github.com/inquisico/go-session/actions/workflows/golangci-lint-push.yaml) [![Go Test](https://github.com/inquisico/go-session/actions/workflows/go-test-push.yaml/badge.svg)](https://github.com/inquisico/go-session/actions/workflows/go-test-push.yaml) [![Release Drafter](https://github.com/inquisico/go-session/actions/workflows/release-drafter.yaml/badge.svg)](https://github.com/inquisico/go-session/actions/workflows/release-drafter.yaml)

Session implements a session management pattern following the [OWASP security guidelines](https://github.com/OWASP/CheatSheetSeries/blob/master/cheatsheets/Session_Management_Cheat_Sheet.md). Session data is stored on the server, and a randomly-generated unique session token (or *session ID*) is communicated to and from the client in a session cookie. This package is based on [alexedwards/scs](https://github.com/alexedwards/scs).

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
    "github.com/alexedwards/scs/v2"
    "github.com/alexedwards/scs/v2/memstore"
    "github.com/inquisico/go-session"
    "github.com/inquisico/go-session/middleware"
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
        Secure:   false,
        SameSite: http.SameSiteLaxMode,
    }

    middleware := middleware.NewHTTPSessionManager(
        sessionManager,
        session.WithErrorFunc(errorFunc), // Optional
        session.WithCookieConfig(cookieConfig), // Optional
    )

    // Put `middleware` into your http server
    // See: https://www.alexedwards.net/blog/making-and-using-middleware
    // ...
}
```

## Creating your own store

The interface for store can be found in store/store.go. You can implement your own store that implements that interface. See [go-session/store](github.com/inquisico/go-session/store) for examples.

## Compatible session stores

Inquisico managed session stores can be found at [go-session/store](github.com/inquisico/go-session/store). If you require a more extensive set of seesion stores, you may check out [more compatible session stores](https://github.com/alexedwards/scs#configuring-the-session-store) for your desired store.
