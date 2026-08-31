package identity

import (
	"errors"
	"strings"
)

const (
	IdempotencyNamespacePrefix    = "idp_"
	IdempotencyNamespaceHexBytes  = 32
	IdempotencyNamespaceTextBytes = len(IdempotencyNamespacePrefix) + IdempotencyNamespaceHexBytes
)

// IdempotencyNamespacePattern is the public JSON Schema spelling of the exact opaque namespace.
const IdempotencyNamespacePattern = `^idp_[0-9a-f]{32}$`

// IdempotencyNamespace is one exact durable replay-store namespace.
type IdempotencyNamespace struct{ text string }

// ParseIdempotencyNamespace rejects normalization and accepts only the canonical lowercase hex
// spelling generated with the SQLite store.
func ParseIdempotencyNamespace(text string) (IdempotencyNamespace, error) {
	if len(text) != IdempotencyNamespaceTextBytes || !strings.HasPrefix(text, IdempotencyNamespacePrefix) {
		return IdempotencyNamespace{}, errors.New("idempotency namespace must use the canonical idp lowercase-hex form")
	}
	for index := len(IdempotencyNamespacePrefix); index < len(text); index++ {
		character := text[index]
		if character >= '0' && character <= '9' || character >= 'a' && character <= 'f' {
			continue
		}
		return IdempotencyNamespace{}, errors.New("idempotency namespace must use the canonical idp lowercase-hex form")
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

func (i IdempotencyNamespace) Validate() error {
	_, err := ParseIdempotencyNamespace(i.text)
	return err
}
