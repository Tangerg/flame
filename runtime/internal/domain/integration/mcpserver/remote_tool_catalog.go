package mcpserver

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	MaxRemoteToolsPerServer = 2_048

	MaxRemoteToolDescriptionBytes = 64 << 10
)

var errInvalidRemoteToolCatalog = errors.New("mcpserver: invalid remote tool catalog")

func ValidateRemoteToolCount(count int) error {
	if count < 0 || count > MaxRemoteToolsPerServer {
		return fmt.Errorf(
			"%w: %d tools exceeds %d",
			errInvalidRemoteToolCatalog,
			count,
			MaxRemoteToolsPerServer,
		)
	}
	return nil
}

func ValidateRemoteToolDescription(description string) error {
	if !utf8.ValidString(description) {
		return fmt.Errorf("%w: description is not valid UTF-8", errInvalidRemoteToolCatalog)
	}
	if len(description) > MaxRemoteToolDescriptionBytes {
		return fmt.Errorf(
			"%w: description uses %d bytes, maximum %d",
			errInvalidRemoteToolCatalog,
			len(description),
			MaxRemoteToolDescriptionBytes,
		)
	}
	return nil
}
