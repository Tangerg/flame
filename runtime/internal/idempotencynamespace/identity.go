// Package idempotencynamespace owns the durable replay-store identity shared
// by persistence, discovery, and operation admission. It is stable for the
// lifetime of one database and intentionally unrelated to a process instance.
package idempotencynamespace

import (
	"errors"
	"strings"
)

const (
	Prefix    = "idp_"
	HexBytes  = 32
	TextBytes = len(Prefix) + HexBytes
)

// Pattern is the public JSON Schema spelling of the exact opaque namespace.
const Pattern = `^idp_[0-9a-f]{32}$`

// ID is one exact durable replay-store namespace.
type ID struct{ text string }

// Parse rejects normalization and accepts only the canonical lowercase hex
// spelling generated with the SQLite store.
func Parse(text string) (ID, error) {
	if len(text) != TextBytes || !strings.HasPrefix(text, Prefix) {
		return ID{}, errors.New("idempotency namespace must use the canonical idp lowercase-hex form")
	}
	for index := len(Prefix); index < len(text); index++ {
		character := text[index]
		if character >= '0' && character <= '9' || character >= 'a' && character <= 'f' {
			continue
		}
		return ID{}, errors.New("idempotency namespace must use the canonical idp lowercase-hex form")
	}
	return ID{text: text}, nil
}

// ParseOptional keeps an absent namespace distinct from a malformed one.
func ParseOptional(text string) (ID, bool, error) {
	if text == "" {
		return ID{}, false, nil
	}
	parsed, err := Parse(text)
	return parsed, err == nil, err
}

func (i ID) String() string { return i.text }

func (i ID) Validate() error {
	_, err := Parse(i.text)
	return err
}
