package mcp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
)

const (
	authorizationAttemptIDPrefix            = "mcpauth_"
	minimumAuthorizationAttemptEntropyBytes = 26
	maximumAuthorizationAttemptEntropyBytes = 64
)

// crypto/rand.Text currently emits 26 RFC 4648 base32 bytes and may grow.
var authorizationAttemptIDExpression = regexp.MustCompile(fmt.Sprintf(
	`^%s[A-Z2-7]{%d,%d}$`,
	regexp.QuoteMeta(authorizationAttemptIDPrefix),
	minimumAuthorizationAttemptEntropyBytes,
	maximumAuthorizationAttemptEntropyBytes,
))

func AuthorizationAttemptIDPattern() string { return authorizationAttemptIDExpression.String() }

// AuthorizationAttemptID identifies one process-local interactive OAuth flow
// throughout its pending and retained-terminal lifetime.
type AuthorizationAttemptID struct{ text string }

func newAuthorizationAttemptID() AuthorizationAttemptID {
	id, err := ParseAuthorizationAttemptID(authorizationAttemptIDPrefix + rand.Text())
	if err != nil {
		panic(fmt.Sprintf("mcp: generated invalid authorization attempt identity: %v", err))
	}
	return id
}

// ParseAuthorizationAttemptID rejects normalization and accepts only the
// uppercase base32 material emitted by the owning generator.
func ParseAuthorizationAttemptID(text string) (AuthorizationAttemptID, error) {
	if !authorizationAttemptIDExpression.MatchString(text) {
		return AuthorizationAttemptID{}, errors.New("mcp: invalid authorization attempt identity")
	}
	return AuthorizationAttemptID{text: text}, nil
}

func (i AuthorizationAttemptID) String() string { return i.text }

// Validate reports whether the attempt identity was parsed. Its exact form is
// established there, so an unconstructed attempt is all this can reject.
func (i AuthorizationAttemptID) Validate() error {
	if i.text == "" {
		return errors.New("mcp: unconstructed authorization attempt identity")
	}
	return nil
}
