package plugins

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

type CommandStore interface {
	Get(context.Context, string) (*plugin.Command, bool, error)
	Save(context.Context, *plugin.Command) error
}

// Commands serializes original binding and dispatch admission with installation
// replacement. All network I/O happens after this boundary has been released.
type Commands struct {
	store     CommandStore
	admission *Admission
}

func NewCommands(store CommandStore, admission *Admission) (*Commands, error) {
	if dependency.Missing(store) || admission == nil {
		return nil, errors.New("plugins: command store and admission are required")
	}
	return &Commands{store: store, admission: admission}, nil
}

// RestoreInvocation validates a private owner's retained caller against original
// host routing. A fresh background trigger may only originate automatically in
// its own installation; human/model callers require an already dispatched route.
func (c *Commands) RestoreInvocation(ctx context.Context, invocation capability.Invocation) (context.Context, error) {
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	actor, found := operation.ActorFrom(ctx)
	if !found || invocation.Target.Scope != "installation" || invocation.Target.Reference != actor.InstallationID().String() {
		return nil, plugin.ErrCommandConflict
	}
	command, found, err := c.store.Get(ctx, invocation.ID)
	if err != nil {
		return nil, err
	}
	if found {
		record := command.Snapshot()
		if record.Invocation != invocation || record.Provider.InstallationID != actor.InstallationID() || record.Provider.Digest != actor.Release() || record.Server != actor.Server() || record.Phase == plugin.CommandPrepared {
			return nil, plugin.ErrCommandConflict
		}
	} else if invocation.Origin != capability.Automatic || invocation.Parent != "" {
		return nil, plugin.ErrCommandConflict
	}
	return operation.Adopt(ctx, invocation.ID, invocation.Origin, invocation.Parent), nil
}

// Find projects an original binding before new-call discovery does network I/O.
// Bind rereads under admission before persisting, so this read cannot admit a
// competing route or race installation replacement.
func (c *Commands) Find(ctx context.Context, id, method string, input jsoninput.Input) (*plugin.Command, bool, error) {
	command, found, err := c.store.Get(ctx, id)
	if err == nil && found && !command.Matches(method, input) {
		return nil, false, plugin.ErrCommandConflict
	}
	return command, found, err
}

func (c *Commands) Bind(ctx context.Context, id, method string, input jsoninput.Input, resolve func(context.Context) (ContributionBinding, error), capture func(ContributionBinding) (capability.Invocation, error)) (*plugin.Command, error) {
	var command *plugin.Command
	err := c.admission.Under(ctx, func() error {
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
		binding, err := resolve(ctx)
		if err != nil {
			return err
		}
		if capture == nil {
			return plugin.ErrInvalid
		}
		invocation, err := capture(binding)
		if err != nil {
			return err
		}
		command, err = plugin.NewCommand(plugin.CommandBinding{ID: id, Method: method, Input: input, Contract: binding.Contract, Contribution: binding.ContributionID, Export: binding.Export, Provider: plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest}, Server: binding.ServerName, Invocation: invocation})
		if err != nil {
			return err
		}
		return c.store.Save(ctx, command)
	})
	return command, err
}

func (c *Commands) Dispatch(ctx context.Context, id string, admit func(context.Context, plugin.CommandBinding) error) error {
	return c.admission.Under(ctx, func() error {
		command, found, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return plugin.ErrNotFound
		}
		if command.Snapshot().Phase != plugin.CommandPrepared {
			return plugin.ErrCommandUnconfirmed
		}
		if err := admit(ctx, command.Snapshot()); err != nil {
			return err
		}
		command.Dispatch()
		return c.store.Save(ctx, command)
	})
}

func (c *Commands) Observe(ctx context.Context, id string) error {
	return c.admission.Under(ctx, func() error {
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
