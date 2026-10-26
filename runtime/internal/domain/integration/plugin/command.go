package plugin

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

var (
	ErrCommandConflict    = errors.New("plugin: command binding conflicts")
	ErrCommandUnconfirmed = errors.New("plugin: command outcome is unconfirmed")
	ErrCommandRetired     = errors.New("plugin: command retired before dispatch")
)

type CommandPhase string

const (
	CommandPrepared   CommandPhase = "prepared"
	CommandDispatched CommandPhase = "dispatched"
	CommandObserved   CommandPhase = "observed"
	CommandRetired    CommandPhase = "retired"
)

// CommandBinding is immutable routing evidence, not a business result. Keeping
// it after observation prevents an old identity from becoming a new-provider call.
type CommandBinding struct {
	ID           string
	Method       string
	Input        jsoninput.Input
	Contract     jsoninput.Input
	Contribution ContributionID
	Export       string
	Provider     Dependency
	Activation   Activation
	Server       mcpserver.ServerName
	Phase        CommandPhase
	Invocation   capability.Invocation
}

type Command struct{ record CommandBinding }

func NewCommand(record CommandBinding) (*Command, error) {
	record.Phase = CommandPrepared
	return RestoreCommand(record)
}

func RestoreCommand(record CommandBinding) (*Command, error) {
	if record.ID == "" || len(record.ID) > 256 || !utf8.ValidString(record.ID) || record.Method == "" || len(record.Method) > 256 || !utf8.ValidString(record.Method) {
		return nil, fmt.Errorf("%w: command identity or method", ErrInvalid)
	}
	if err := record.Input.Validate(); err != nil {
		return nil, err
	}
	if err := record.Contribution.Validate(); err != nil {
		return nil, err
	}
	if !capability.ValidExportName(record.Export) {
		return nil, fmt.Errorf("%w: command export", ErrInvalid)
	}
	contract, err := capability.Parse(record.Contract.JSON())
	if err != nil {
		return nil, err
	}
	method, found := contract.Method(record.Method)
	if !found || method.Kind() != capability.Mutation {
		return nil, fmt.Errorf("%w: command does not belong to its original capability", ErrInvalid)
	}
	if err := record.Invocation.Validate(); err != nil {
		return nil, err
	}
	if record.Invocation.ID != record.ID || record.Invocation.Target.Scope != method.Descriptor().Target {
		return nil, fmt.Errorf("%w: command invocation differs from its identity or target contract", ErrInvalid)
	}
	record.Contract = contract.Identity()
	if err := record.Provider.Validate(); err != nil {
		return nil, err
	}
	if err := record.Activation.Validate(); err != nil {
		return nil, err
	}
	if !slices.Contains(record.Activation.Dependencies, record.Provider) {
		return nil, fmt.Errorf("%w: command dependencies omit the original provider", ErrInvalid)
	}
	record.Activation = record.Activation.Clone()
	if record.Invocation.Target.Scope == capability.ScopeInstallation && record.Invocation.Target.Reference != record.Provider.InstallationID.String() {
		return nil, fmt.Errorf("%w: command target differs from its installation", ErrInvalid)
	}
	if err := record.Server.Validate(); err != nil {
		return nil, err
	}
	switch record.Phase {
	case CommandPrepared, CommandDispatched, CommandObserved, CommandRetired:
	default:
		return nil, fmt.Errorf("%w: command phase", ErrInvalid)
	}
	return &Command{record: record}, nil
}

func (c *Command) Snapshot() CommandBinding {
	record := c.record
	record.Activation = record.Activation.Clone()
	return record
}
func (c *Command) Matches(method string, input jsoninput.Input) bool {
	return c.record.Method == method && c.record.Input == input
}
func (c *Command) Pending() bool {
	return c.record.Phase == CommandPrepared || c.record.Phase == CommandDispatched
}
func (c *Command) WasDispatched() bool {
	return c.record.Phase == CommandDispatched || c.record.Phase == CommandObserved
}
func (c *Command) Dispatch() error {
	if c.record.Phase == CommandRetired {
		return ErrCommandRetired
	}
	if c.record.Phase != CommandPrepared {
		return ErrCommandUnconfirmed
	}
	c.record.Phase = CommandDispatched
	return nil
}
func (c *Command) Retire() error {
	switch c.record.Phase {
	case CommandPrepared, CommandRetired:
		c.record.Phase = CommandRetired
		return nil
	default:
		return ErrCommandUnconfirmed
	}
}
func (c *Command) Observe() error {
	if !c.WasDispatched() {
		return fmt.Errorf("%w: command has no dispatch admission", ErrInvalid)
	}
	c.record.Phase = CommandObserved
	return nil
}
