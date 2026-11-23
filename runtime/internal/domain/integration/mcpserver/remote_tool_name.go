package mcpserver

import (
	"errors"
	"fmt"
	"regexp"
)

const MaximumRemoteToolNameCharacters = 128

var (
	remoteToolNamePattern = fmt.Sprintf(
		`^[A-Za-z0-9_.-]{1,%d}$`,
		MaximumRemoteToolNameCharacters,
	)
	remoteToolNameExpression = regexp.MustCompile(remoteToolNamePattern)

	ErrInvalidRemoteToolName = errors.New("mcpserver: invalid remote tool name")
)

type RemoteToolName struct {
	value string
}

func ParseRemoteToolName(raw string) (RemoteToolName, error) {
	if !remoteToolNameExpression.MatchString(raw) {
		return RemoteToolName{}, fmt.Errorf(
			"%w: %q must match %s and contain at most %d characters",
			ErrInvalidRemoteToolName,
			raw,
			remoteToolNamePattern,
			MaximumRemoteToolNameCharacters,
		)
	}
	return RemoteToolName{value: raw}, nil
}

func (n RemoteToolName) Validate() error {
	if !remoteToolNameExpression.MatchString(n.value) {
		return fmt.Errorf("%w: zero or malformed value", ErrInvalidRemoteToolName)
	}
	return nil
}

func (n RemoteToolName) String() string { return n.value }

func RemoteToolNamePattern() string { return remoteToolNamePattern }
