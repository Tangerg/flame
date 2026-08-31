package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	BuildScheme = "sha256"
	BuildPrefix = BuildScheme + ":"
)

var ErrInvalidBuild = errors.New("build identity must use sha256:<64 lowercase hexadecimal characters>")

// BuildID is an exact SHA-256 content identity. It never trims, repairs, or aliases
// caller input because equality decides checkpoint restore compatibility.
type BuildID struct {
	value string
}

// ParseBuild proves that raw is the canonical lowercase SHA-256 representation.
func ParseBuild(raw string) (BuildID, error) {
	digest, ok := strings.CutPrefix(raw, BuildPrefix)
	if !ok || len(digest) != hex.EncodedLen(sha256.Size) || digest != strings.ToLower(digest) {
		return BuildID{}, ErrInvalidBuild
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return BuildID{}, ErrInvalidBuild
	}
	return BuildID{value: raw}, nil
}

// BuildFromSHA256 renders a computed digest in its only canonical representation.
func BuildFromSHA256(digest [sha256.Size]byte) BuildID {
	return BuildID{value: BuildPrefix + hex.EncodeToString(digest[:])}
}

func (i BuildID) String() string { return i.value }
