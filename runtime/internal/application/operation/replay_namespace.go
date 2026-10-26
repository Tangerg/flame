package operation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const (
	ReplayNamespacePrefix        = "idp_"
	ReplayNamespaceEntropyBytes  = 16
	replayNamespaceHexCharacters = ReplayNamespaceEntropyBytes * 2
)

// replayNamespacePattern is the public JSON Schema spelling of the exact
// opaque namespace. It is derived from the prefix and length the parser
// enforces so the published contract cannot describe a different namespace
// than the one the Runtime accepts.
var replayNamespacePattern = fmt.Sprintf(
	`^%s[0-9a-f]{%d}$`,
	ReplayNamespacePrefix,
	replayNamespaceHexCharacters,
)

var errIdempotencyNamespaceForm = errors.New("idempotency namespace must use the canonical idp lowercase-hex form")

// ReplayNamespace is one exact durable replay-store namespace.
type ReplayNamespace struct{ text string }

// ParseReplayNamespace rejects normalization and accepts only the canonical lowercase hex
// spelling generated with the SQLite store.
func ParseReplayNamespace(text string) (ReplayNamespace, error) {
	digest, ok := strings.CutPrefix(text, ReplayNamespacePrefix)
	if !ok {
		return ReplayNamespace{}, errIdempotencyNamespaceForm
	}
	if err := identity.ValidateLowercaseHex(digest, replayNamespaceHexCharacters); err != nil {
		return ReplayNamespace{}, errIdempotencyNamespaceForm
	}
	return ReplayNamespace{text: text}, nil
}

// ReplayNamespacePattern projects the exact namespace grammar to protocol generation.
func ReplayNamespacePattern() string { return replayNamespacePattern }

func (i ReplayNamespace) String() string { return i.text }
