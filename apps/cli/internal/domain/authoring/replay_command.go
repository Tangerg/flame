package authoring

import (
	"encoding/hex"
	"errors"
	"strings"
)

const commandIDPrefix = "cli_"

const CommandIDEntropyBytes = 16

type CommandID string

func NewCommandID(entropy [CommandIDEntropyBytes]byte) CommandID {
	return CommandID(commandIDPrefix + hex.EncodeToString(entropy[:]))
}

func (c CommandID) Validate() error {
	value := string(c)
	encoded, ok := strings.CutPrefix(value, commandIDPrefix)
	if !ok || len(encoded) != CommandIDEntropyBytes*2 {
		return errors.New("command id has an invalid shape")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return errors.New("command id has invalid entropy")
	}
	return nil
}
