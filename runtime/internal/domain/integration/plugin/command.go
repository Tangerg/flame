package plugin

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

	"github.com/Tangerg/frame"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
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

func (p CommandPhase) Valid() bool {
	return p == CommandPrepared || p == CommandDispatched || p == CommandObserved || p == CommandRetired
}

func validCommandText(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= MaximumCommandTextCharacters
}

type CommandBinding struct {
	ID           string
	Method       string
	Input        framevalue.JSON
	Contract     framevalue.JSON
	Contribution ContributionID
	Export       string
	Provider     Dependency
	Activation   Activation
	Server       mcpserver.ServerName
	Phase        CommandPhase
	Invocation   framecall.Invocation
}

type Command struct{ record CommandBinding }

func NewCommand(record CommandBinding) (*Command, error) {
	record.Phase = CommandPrepared
	return RestoreCommand(record)
}

func RestoreCommand(record CommandBinding) (*Command, error) {
	if !validCommandText(record.ID) || !validCommandText(record.Method) {
		return nil, fmt.Errorf("%w: command identity or method", ErrInvalid)
	}
	if !record.Phase.Valid() {
		return nil, fmt.Errorf("%w: command phase", ErrInvalid)
	}
	if err := record.Contribution.Validate(); err != nil {
		return nil, err
	}
	if !plugincontract.ValidExportName(record.Export) {
		return nil, fmt.Errorf("%w: command export", ErrInvalid)
	}
	if err := record.Server.Validate(); err != nil {
		return nil, err
	}
	contract, err := record.validateContract()
	if err != nil {
		return nil, err
	}
	if err := record.validateProvenance(); err != nil {
		return nil, err
	}
	record.Contract = contract
	record.Activation = record.Activation.Clone()
	return &Command{record: record}, nil
}

func (record CommandBinding) validateContract() (framevalue.JSON, error) {
	if err := record.Input.Validate(); err != nil {
		return framevalue.JSON{}, err
	}
	contract, err := plugincontract.ParseCapability(record.Contract.Bytes())
	if err != nil {
		return framevalue.JSON{}, err
	}
	method, found := contract.Method(record.Method)
	if !found || method.Kind() != frame.Command {
		return framevalue.JSON{}, fmt.Errorf("%w: command does not belong to its original capability", ErrInvalid)
	}
	if err := method.ValidateInput(record.Input); err != nil {
		return framevalue.JSON{}, fmt.Errorf("%w: command input differs from its original contract: %w", ErrInvalid, err)
	}
	if err := extension.ValidateInvocation(record.Invocation); err != nil {
		return framevalue.JSON{}, err
	}
	if record.Invocation.ID != record.ID || record.Invocation.Target.Scope != method.Descriptor().Target {
		return framevalue.JSON{}, fmt.Errorf("%w: command invocation differs from its identity or target contract", ErrInvalid)
	}
	return contract.Identity(), nil
}

func (record CommandBinding) validateProvenance() error {
	if err := record.Provider.Validate(); err != nil {
		return err
	}
	if err := record.Activation.Validate(); err != nil {
		return err
	}
	if !slices.Contains(record.Activation.Dependencies, record.Provider) {
		return fmt.Errorf("%w: command dependencies omit the original provider", ErrInvalid)
	}
	if record.Invocation.Target.Scope == extension.ScopeInstallation && record.Invocation.Target.Reference != record.Provider.InstallationID.String() {
		return fmt.Errorf("%w: command target differs from its installation", ErrInvalid)
	}
	return nil
}

func (c *Command) Snapshot() CommandBinding {
	record := c.record
	record.Activation = record.Activation.Clone()
	return record
}

func (c *Command) SameBinding(other *Command) bool {
	a, b := c.record, other.record
	return a.ID == b.ID && a.Method == b.Method && a.Input == b.Input && a.Contract == b.Contract &&
		a.Provider == b.Provider && a.Server == b.Server && a.Contribution == b.Contribution &&
		a.Export == b.Export && a.Invocation == b.Invocation && a.Activation.Equal(b.Activation)
}

func (c *Command) Matches(method string, input framevalue.JSON) bool {
	return c.record.Method == method && c.record.Input == input
}

var pendingCommandPhases = []CommandPhase{CommandPrepared, CommandDispatched}

func PendingCommandPhases() []CommandPhase { return slices.Clone(pendingCommandPhases) }

func (c *Command) Pending() bool { return slices.Contains(pendingCommandPhases, c.record.Phase) }

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
