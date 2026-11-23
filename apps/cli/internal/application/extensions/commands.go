package extensions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type SlashCommand struct {
	Descriptor CommandDescriptor
	Available  func(CommandRequest) CommandAvailability
	Execute    func(context.Context, CommandRequest) (CommandResult, error)
}

type ArgumentMode string

const (
	NoArguments       ArgumentMode = ""
	OptionalArguments ArgumentMode = "optional"
	RequiredArguments ArgumentMode = "required"
)

func (a ArgumentMode) Validate() error {
	if a != NoArguments && a != OptionalArguments && a != RequiredArguments {
		return fmt.Errorf("slash command argument mode %q is invalid", a)
	}
	return nil
}

func (a ArgumentMode) TakesInput() bool { return a != NoArguments }

func (a ArgumentMode) ValidateInvocation(name, argument string) error {
	argument = strings.TrimSpace(argument)
	switch a {
	case NoArguments:
		if argument != "" {
			return fmt.Errorf("/%s does not accept arguments", name)
		}
	case OptionalArguments:
	case RequiredArguments:
		if argument == "" {
			return fmt.Errorf("/%s needs an argument", name)
		}
	default:
		return fmt.Errorf("/%s has an invalid argument contract", name)
	}
	return nil
}

type CommandDescriptor struct {
	Name      string
	Title     string
	Category  string
	Aliases   []string
	Arguments ArgumentMode
}

func (c CommandDescriptor) Validate() error {
	switch {
	case c.Name == "":
		return errors.New("slash command has no name")
	case !validCommandIdentity(c.Name):
		return fmt.Errorf("slash command %q has an invalid name", c.Name)
	case strings.TrimSpace(c.Title) == "":
		return fmt.Errorf("slash command %q has no title", c.Name)
	default:
		if err := c.Arguments.Validate(); err != nil {
			return fmt.Errorf("slash command %q: %w", c.Name, err)
		}
		return c.validateAliases()
	}
}

func (c CommandDescriptor) validateAliases() error {
	seen := map[string]struct{}{c.Name: {}}
	for _, alias := range c.Aliases {
		if !validCommandIdentity(alias) {
			return fmt.Errorf("slash command %q has invalid alias %q", c.Name, alias)
		}
		if _, duplicate := seen[alias]; duplicate {
			return fmt.Errorf("slash command %q repeats name or alias %q", c.Name, alias)
		}
		seen[alias] = struct{}{}
	}
	return nil
}

const InstalledViewCommandPrefix = "view:"

func validCommandIdentity(value string) bool {
	return value != "" && !strings.HasPrefix(value, InstalledViewCommandPrefix) && utf8.ValidString(value) && !strings.ContainsRune(value, '/') &&
		!strings.ContainsFunc(value, func(character rune) bool { return unicode.IsSpace(character) || !unicode.IsPrint(character) })
}

func (c CommandDescriptor) CategoryLabel() string {
	if category := strings.TrimSpace(c.Category); category != "" {
		return category
	}
	return "General"
}

func (c CommandDescriptor) Identities() []string {
	return append([]string{c.Name}, c.Aliases...)
}

type CommandAvailability struct {
	reason string
}

func CommandAvailable() CommandAvailability { return CommandAvailability{} }

func CommandUnavailable(reason string) CommandAvailability {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "not available in the current context"
	}
	return CommandAvailability{reason: reason}
}

func (a CommandAvailability) Enabled() bool { return a.reason == "" }

func (a CommandAvailability) Reason() string { return a.reason }

type CommandRequest struct {
	Argument       string
	Workspace      string
	LocalDirectory string
	SessionID      string
}

type CommandResult struct {
	Message string
}

func (s SlashCommand) Validate() error {
	if err := s.Descriptor.Validate(); err != nil {
		return err
	}
	if s.Execute == nil {
		return fmt.Errorf("slash command %q has no handler", s.Descriptor.Name)
	}
	return nil
}
