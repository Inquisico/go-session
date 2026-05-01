// Package session provides server-side session management primitives.
package session

import (
	"fmt"
	"sync/atomic"
)

type contextKey string

var contextKeyID atomic.Uint64

func generateContextKey() contextKey {
	id := contextKeyID.Add(1)
	return contextKey(fmt.Sprintf("session.%d", id))
}
