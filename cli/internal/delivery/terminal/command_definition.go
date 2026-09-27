package terminal

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
)

func parseSlashCommand(line string) (name, argument string, ok bool) {
	if !strings.HasPrefix(line, "/") {
		return "", "", false
	}
	name, argument, _ = strings.Cut(strings.TrimPrefix(line, "/"), " ")
	return name, strings.TrimSpace(argument), true
}

func splitCommandArgument(value string) (identity, remainder string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	boundary := strings.IndexFunc(value, unicode.IsSpace)
	if boundary < 0 {
		return value, "", true
	}
	return value[:boundary], strings.TrimSpace(value[boundary:]), true
}

func trimCommandIdentity(value, identity string) (string, bool) {
	if !strings.HasPrefix(value, identity) {
		return "", false
	}
	remainder := value[len(identity):]
	if remainder == "" {
		return "", true
	}
	if boundary, _ := utf8.DecodeRuneInString(remainder); !unicode.IsSpace(boundary) {
		return "", false
	}
	return strings.TrimSpace(remainder), true
}

type localCommand struct {
	Descriptor extensions.CommandDescriptor
	Available  func(*app) extensions.CommandAvailability
	Run        func(*app, string) error
}

func (l localCommand) validate() error {
	if err := l.Descriptor.Validate(); err != nil {
		return err
	}
	if l.Run == nil {
		return fmt.Errorf("slash command %q has no handler", l.Descriptor.Name)
	}
	return nil
}
