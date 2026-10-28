package plugins

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/samber/lo"
)

type CommandStore interface {
	// Get returns a caller-owned aggregate; only Save advances durable routing.
	Get(context.Context, string) (*plugin.Command, bool, error)
	Save(context.Context, *plugin.Command) error
}

// Commands serializes original binding and dispatch admission with installation
// replacement. All network I/O happens after this boundary has been released.
type Commands struct {
	store     CommandStore
	admission *qimu.Admission
}

func NewCommands(store CommandStore, admission *qimu.Admission) (*Commands, error) {
	if lo.IsNil(store) || admission == nil {
		return nil, errors.New("plugins: command store and admission are required")
	}
	return &Commands{store: store, admission: admission}, nil
}

func (c *Commands) Get(ctx context.Context, id string) (*plugin.Command, error) {
	command, err := c.authorized(ctx, id)
	if err != nil {
		return nil, err
	}
	switch command.Snapshot().Phase {
	case plugin.CommandPrepared:
		return nil, plugin.ErrCommandUnconfirmed
	case plugin.CommandRetired:
		return nil, plugin.ErrCommandRetired
	}
	return command, nil
}

func (c *Commands) authorized(ctx context.Context, id string) (*plugin.Command, error) {
	command, found, err := c.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, plugin.ErrNotFound
	}
	record := command.Snapshot()
	if actor, managed := operation.ActorFrom(ctx); managed && !actor.Allows(record.Provider, record.Contribution, record.Contract) {
		return nil, plugin.ErrInvalid
	}
	return command, nil
}

// Retirement and dispatch compete under the same admission boundary. The retained
// route prevents a retired identity from ever being admitted as new work.
func (c *Commands) Retire(ctx context.Context, id string) error {
	return c.admission.Admit(ctx, func(ctx context.Context) error {
		command, err := c.authorized(ctx, id)
		if err != nil {
			return err
		}
		if err := command.Retire(); err != nil {
			return err
		}
		return c.store.Save(ctx, command)
	})
}

// RestoreInvocation validates a private owner's retained caller against original
// host routing, including the resource owner's original target. Only a fresh
// automatic trigger needs the background contribution's own installation target.
func (c *Commands) RestoreInvocation(ctx context.Context, invocation portable.Invocation) (context.Context, error) {
	if err := extension.ValidateInvocation(invocation); err != nil {
		return nil, err
	}
	actor, found := operation.ActorFrom(ctx)
	if !found {
		return nil, plugin.ErrCommandConflict
	}
	command, found, err := c.store.Get(ctx, invocation.ID)
	if err != nil {
		return nil, err
	}
	if found {
		record := command.Snapshot()
		if record.Invocation != invocation || record.Provider.InstallationID != actor.InstallationID() || record.Provider.Digest != actor.Release() || record.Server != actor.Server() || !command.WasDispatched() {
			return nil, plugin.ErrCommandConflict
		}
	} else if invocation.Origin != extension.Automatic || invocation.Parent != "" || invocation.Target.Scope != extension.ScopeInstallation || invocation.Target.Reference != actor.InstallationID().String() {
		return nil, plugin.ErrCommandConflict
	}
	return operation.Adopt(ctx, invocation.ID, invocation.Origin, invocation.Parent), nil
}

// Find projects an original binding before new-call discovery does network I/O.
// Bind rereads under admission before persisting, so this read cannot admit a
// competing route or race installation replacement.
func (c *Commands) Find(ctx context.Context, id, method string, input jsonvalue.Value) (*plugin.Command, bool, error) {
	command, found, err := c.store.Get(ctx, id)
	if err == nil && found && !command.Matches(method, input) {
		return nil, false, plugin.ErrCommandConflict
	}
	return command, found, err
}

// CommandAdmission captures routing and its actual activation dependencies at the
// same serialization point. It carries no private business settlement.
type CommandAdmission struct {
	Binding    ContributionBinding
	Activation plugin.Activation
}

func (c *Commands) Bind(ctx context.Context, id, method string, input jsonvalue.Value, resolve func(context.Context) (CommandAdmission, error), capture func(ContributionBinding) (portable.Invocation, error)) (*plugin.Command, error) {
	var command *plugin.Command
	err := c.admission.Admit(ctx, func(ctx context.Context) error {
		previous, found, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if found {
			if !previous.Matches(method, input) {
				return plugin.ErrCommandConflict
			}
			command = previous
			return nil
		}
		admitted, err := resolve(ctx)
		if err != nil {
			return err
		}
		if capture == nil {
			return plugin.ErrInvalid
		}
		binding := admitted.Binding
		invocation, err := capture(binding)
		if err != nil {
			return err
		}
		command, err = plugin.NewCommand(plugin.CommandBinding{ID: id, Method: method, Input: input, Contract: binding.Contract, Contribution: binding.ContributionID, Export: binding.Export, Provider: plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest}, Activation: admitted.Activation, Server: binding.ServerName, Invocation: invocation})
		if err != nil {
			return err
		}
		return c.store.Save(ctx, command)
	})
	return command, err
}

func (c *Commands) Dispatch(ctx context.Context, id string, admit func(context.Context, plugin.CommandBinding) error) error {
	return c.admission.Admit(ctx, func(ctx context.Context) error {
		command, found, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return plugin.ErrNotFound
		}
		record := command.Snapshot()
		if err := command.Dispatch(); err != nil {
			return err
		}
		if err := admit(ctx, record); err != nil {
			return err
		}
		return c.store.Save(ctx, command)
	})
}

func (c *Commands) Observe(ctx context.Context, id string) error {
	return c.admission.Admit(ctx, func(ctx context.Context) error {
		command, found, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return plugin.ErrNotFound
		}
		if err := command.Observe(); err != nil {
			return err
		}
		return c.store.Save(ctx, command)
	})
}
