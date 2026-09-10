package identity

import (
	"errors"
	"fmt"
	"strings"
)

const (
	IdempotencyNamespacePrefix        = "idp_"
	IdempotencyNamespaceHexCharacters = 32
)

// IdempotencyNamespacePattern is the public JSON Schema spelling of the exact
// opaque namespace. It is derived from the prefix and length the parser
// enforces so the published contract cannot describe a different namespace
// than the one the Runtime accepts.
var IdempotencyNamespacePattern = fmt.Sprintf(
	`^%s[0-9a-f]{%d}$`,
	IdempotencyNamespacePrefix,
	IdempotencyNamespaceHexCharacters,
)

var errIdempotencyNamespaceForm = errors.New("idempotency namespace must use the canonical idp lowercase-hex form")

// IdempotencyNamespace is one exact durable replay-store namespace.
type IdempotencyNamespace struct{ text string }

// ParseIdempotencyNamespace rejects normalization and accepts only the canonical lowercase hex
// spelling generated with the SQLite store.
func ParseIdempotencyNamespace(text string) (IdempotencyNamespace, error) {
	digest, ok := strings.CutPrefix(text, IdempotencyNamespacePrefix)
	if !ok {
		return IdempotencyNamespace{}, errIdempotencyNamespaceForm
	}
	if err := ValidateLowercaseHex(digest, IdempotencyNamespaceHexCharacters); err != nil {
		return IdempotencyNamespace{}, errIdempotencyNamespaceForm
	}
	return IdempotencyNamespace{text: text}, nil
}

// ParseOptionalIdempotencyNamespace keeps an absent namespace distinct from a malformed one.
func ParseOptionalIdempotencyNamespace(text string) (IdempotencyNamespace, bool, error) {
	if text == "" {
		return IdempotencyNamespace{}, false, nil
	}
	parsed, err := ParseIdempotencyNamespace(text)
	return parsed, err == nil, err
}

func (i IdempotencyNamespace) String() string { return i.text }

// Validate proves that i was parsed. The canonical lowercase-hex spelling is
// established there, so an unconstructed namespace is all this can reject.
func (i IdempotencyNamespace) Validate() error {
	if i.text == "" {
		return errIdempotencyNamespaceForm
	}
	return nil
}
