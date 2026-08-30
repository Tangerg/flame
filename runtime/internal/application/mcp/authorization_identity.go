package mcp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
)

const (
	authorizationAttemptIDPrefix            = "mcpauth_"
	minimumAuthorizationAttemptEntropyBytes = 26
	maximumAuthorizationAttemptEntropyBytes = 64
	// AuthorizationAttemptIDPattern is the public wire grammar shared with the
	// generated protocol validators. crypto/rand.Text currently emits 26 RFC
	// 4648 base32 bytes and may grow in a future Go release.
	AuthorizationAttemptIDPattern = `^mcpauth_[A-Z2-7]{26,64}$`
)

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
	if !strings.HasPrefix(text, authorizationAttemptIDPrefix) {
		return AuthorizationAttemptID{}, errors.New("MCP authorization attempt identity has invalid framing")
	}
	entropy := text[len(authorizationAttemptIDPrefix):]
	if len(entropy) < minimumAuthorizationAttemptEntropyBytes || len(entropy) > maximumAuthorizationAttemptEntropyBytes {
		return AuthorizationAttemptID{}, errors.New("MCP authorization attempt identity has invalid length")
	}
	for index := range len(entropy) {
		character := entropy[index]
		if character >= 'A' && character <= 'Z' || character >= '2' && character <= '7' {
			continue
		}
		return AuthorizationAttemptID{}, errors.New("MCP authorization attempt identity is not uppercase base32")
	}
	return AuthorizationAttemptID{text: text}, nil
}

func (i AuthorizationAttemptID) String() string { return i.text }

func (i AuthorizationAttemptID) Validate() error {
	_, err := ParseAuthorizationAttemptID(i.text)
	return err
}
