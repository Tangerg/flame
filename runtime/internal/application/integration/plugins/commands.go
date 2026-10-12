package plugins

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
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

func (c *Commands) Bind(ctx context.Context, id, method string, input fingerprint.Digest, resolve func(context.Context) (ServiceBinding, error)) (*plugin.Command, error) {
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
		command, err = plugin.NewCommand(id, method, input, plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest}, binding.ServerName)
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
