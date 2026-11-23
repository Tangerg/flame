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

type BuildID struct {
	value string
}

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

func BuildFromSHA256(digest [sha256.Size]byte) BuildID {
	return BuildID{value: buildPrefix + hex.EncodeToString(digest[:])}
}

func (i BuildID) String() string { return i.value }
