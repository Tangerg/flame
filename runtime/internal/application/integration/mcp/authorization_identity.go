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

var authorizationAttemptIDExpression = regexp.MustCompile(fmt.Sprintf(
	`^%s[A-Z2-7]{%d,%d}$`,
	regexp.QuoteMeta(authorizationAttemptIDPrefix),
	minimumAuthorizationAttemptEntropyBytes,
	maximumAuthorizationAttemptEntropyBytes,
))

func AuthorizationAttemptIDPattern() string { return authorizationAttemptIDExpression.String() }

type AuthorizationAttemptID struct{ text string }

func newAuthorizationAttemptID() AuthorizationAttemptID {
	id, err := ParseAuthorizationAttemptID(authorizationAttemptIDPrefix + rand.Text())
	if err != nil {
		panic(fmt.Sprintf("mcp: generated invalid authorization attempt identity: %v", err))
	}
	return id
}

func ParseAuthorizationAttemptID(text string) (AuthorizationAttemptID, error) {
	if !authorizationAttemptIDExpression.MatchString(text) {
		return AuthorizationAttemptID{}, errors.New("mcp: invalid authorization attempt identity")
	}
	return AuthorizationAttemptID{text: text}, nil
}

func (i AuthorizationAttemptID) String() string { return i.text }

func (i AuthorizationAttemptID) Validate() error {
	if i.text == "" {
		return errors.New("mcp: unconstructed authorization attempt identity")
	}
	return nil
}
