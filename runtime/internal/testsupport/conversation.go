// Package testsupport provides shared Runtime test builders and in-memory
// fakes. Production code must use semantic Domain and Application paths.
package testsupport

import (
	"sync"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
)

// ConversationStore is a synchronized test double for Flame's mutable
// Session-keyed conversation port, including atomic retention operations.
type ConversationStore struct {
	mu       sync.Mutex
	messages map[history.ConversationID][]chat.Message
}
