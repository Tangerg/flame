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

var replayNamespacePattern = fmt.Sprintf(
	`^%s[0-9a-f]{%d}$`,
	ReplayNamespacePrefix,
	replayNamespaceHexCharacters,
)

var errIdempotencyNamespaceForm = errors.New("idempotency namespace must use the canonical idp lowercase-hex form")

type ReplayNamespace struct{ text string }

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

func ReplayNamespacePattern() string { return replayNamespacePattern }

func (i ReplayNamespace) String() string { return i.text }
