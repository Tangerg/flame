// Package buildidentity owns the exact identity of executable content that may
// create or restore a durable executor checkpoint.
package buildidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	Scheme = "sha256"
	Prefix = Scheme + ":"
)

var ErrInvalid = errors.New("build identity must use sha256:<64 lowercase hexadecimal characters>")

// ID is an exact SHA-256 content identity. It never trims, repairs, or aliases
// caller input because equality decides checkpoint restore compatibility.
type ID struct {
	value string
}

// Parse proves that raw is the canonical lowercase SHA-256 representation.
func Parse(raw string) (ID, error) {
	digest, ok := strings.CutPrefix(raw, Prefix)
	if !ok || len(digest) != hex.EncodedLen(sha256.Size) || digest != strings.ToLower(digest) {
		return ID{}, ErrInvalid
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return ID{}, ErrInvalid
	}
	return ID{value: raw}, nil
}

// FromSHA256 renders a computed digest in its only canonical representation.
func FromSHA256(digest [sha256.Size]byte) ID {
	return ID{value: Prefix + hex.EncodeToString(digest[:])}
}

func (i ID) String() string { return i.value }
