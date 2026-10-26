package runs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const (
	buildScheme = "sha256"
	buildPrefix = buildScheme + ":"
)

var errInvalidBuild = errors.New("build identity must use sha256:<64 lowercase hexadecimal characters>")

// BuildID is an exact SHA-256 content identity. It never trims, repairs, or aliases
// caller input because equality decides checkpoint restore compatibility.
type BuildID struct {
	value string
}

// ParseBuild proves that raw is the canonical lowercase SHA-256 representation.
func ParseBuild(raw string) (BuildID, error) {
	digest, ok := strings.CutPrefix(raw, buildPrefix)
	if !ok {
		return BuildID{}, errInvalidBuild
	}
	if err := identity.ValidateLowercaseHex(digest, hex.EncodedLen(sha256.Size)); err != nil {
		return BuildID{}, errInvalidBuild
	}
	return BuildID{value: raw}, nil
}

// BuildFromSHA256 renders a computed digest in its only canonical representation.
func BuildFromSHA256(digest [sha256.Size]byte) BuildID {
	return BuildID{value: buildPrefix + hex.EncodeToString(digest[:])}
}

func (i BuildID) String() string { return i.value }
