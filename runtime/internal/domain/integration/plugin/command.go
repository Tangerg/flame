package plugin

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

var (
	ErrCommandConflict    = errors.New("plugin: command binding conflicts")
	ErrCommandUnconfirmed = errors.New("plugin: command outcome is unconfirmed")
)

type CommandPhase string

const (
	CommandPrepared   CommandPhase = "prepared"
	CommandDispatched CommandPhase = "dispatched"
	CommandObserved   CommandPhase = "observed"
)

// CommandBinding is immutable routing evidence, not a business result. Keeping
// it after observation prevents an old identity from becoming a new-provider call.
type CommandBinding struct {
	ID       string
	Method   string
	Input    jsoninput.Input
	Provider Dependency
	Server   mcpserver.ServerName
	Phase    CommandPhase
}

type Command struct{ record CommandBinding }

func NewCommand(id, method string, input jsoninput.Input, provider Dependency, server mcpserver.ServerName) (*Command, error) {
	return RestoreCommand(CommandBinding{ID: id, Method: method, Input: input, Provider: provider, Server: server, Phase: CommandPrepared})
}

func RestoreCommand(record CommandBinding) (*Command, error) {
	if record.ID == "" || len(record.ID) > 256 || !utf8.ValidString(record.ID) || record.Method == "" || len(record.Method) > 256 || !utf8.ValidString(record.Method) {
		return nil, fmt.Errorf("%w: command identity or method", ErrInvalid)
	}
	if err := record.Input.Validate(); err != nil {
		return nil, err
	}
	if err := record.Provider.Validate(); err != nil {
		return nil, err
	}
	if err := record.Server.Validate(); err != nil {
		return nil, err
	}
	switch record.Phase {
	case CommandPrepared, CommandDispatched, CommandObserved:
	default:
		return nil, fmt.Errorf("%w: command phase", ErrInvalid)
	}
	return &Command{record: record}, nil
}

func (c *Command) Snapshot() CommandBinding { return c.record }
func (c *Command) Matches(method string, input jsoninput.Input) bool {
	return c.record.Method == method && c.record.Input == input
}
func (c *Command) Pending() bool { return c.record.Phase != CommandObserved }
func (c *Command) Dispatch() {
	if c.record.Phase == CommandPrepared {
		c.record.Phase = CommandDispatched
	}
}
func (c *Command) Observe() error {
	if c.record.Phase == CommandPrepared {
		return fmt.Errorf("%w: command has no dispatch admission", ErrInvalid)
	}
	c.record.Phase = CommandObserved
	return nil
}
