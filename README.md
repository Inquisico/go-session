# Session Management

[![Go Lint](https://github.com/Inquisico/session/actions/workflows/golangci-lint-push.yaml/badge.svg)](https://github.com/Inquisico/session/actions/workflows/golangci-lint-push.yaml) [![Go Test](https://github.com/Inquisico/session/actions/workflows/go-test-push.yaml/badge.svg)](https://github.com/Inquisico/session/actions/workflows/go-test-push.yaml) [![Release Drafter](https://github.com/Inquisico/session/actions/workflows/release-drafter.yaml/badge.svg)](https://github.com/Inquisico/session/actions/workflows/release-drafter.yaml)

Session implements a session management pattern following the [OWASP security guidelines](https://github.com/OWASP/CheatSheetSeries/blob/master/cheatsheets/Session_Management_Cheat_Sheet.md). Session data is stored on the server, and a randomly-generated unique session token (or *session ID*) is communicated to and from the client in a session cookie. This package is based on [alexedwards/scs]("https://github.com/alexedwards/scs").

