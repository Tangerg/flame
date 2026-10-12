package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/google/uuid"
)

type InitializationStore interface {
	Store
	Initialize(context.Context, func(context.Context) error) error
}

type DefaultProvider struct {
	Role, Package, Contribution string
}

// Initialize commits the distribution catalog and configured provider choices
// once. Installation and composition owners advance their own facts thereafter.
func Initialize(ctx context.Context, store InitializationStore, packages Packages, sources func() ([]string, error), selections SelectionStore, defaults []DefaultProvider) error {
	return store.Initialize(ctx, func(ctx context.Context) (err error) {
		existing, err := store.List(ctx)
		if err != nil {
			return err
		}
		if len(existing) != 0 {
			return errors.New("plugins: uninitialized database contains installations; open a fresh data directory")
		}
		choices, err := selections.List(ctx)
		if err != nil {
			return err
		}
		if len(choices) != 0 {
			return errors.New("plugins: uninitialized database contains provider selections; open a fresh data directory")
		}
		paths, err := sources()
		if err != nil {
			return err
		}
		if len(paths) == 0 || len(paths) > plugin.MaxInstallations {
			return errors.New("plugins: distribution requires a bounded first-party catalog")
		}
		for _, source := range paths {
			if err := installFirstParty(ctx, store, packages, source); err != nil {
				return err
			}
		}
		return selectInitialProviders(ctx, store, selections, defaults)
	})
}

func installFirstParty(ctx context.Context, store Store, packages Packages, source string) (err error) {
	candidate, err := packages.Materialize(ctx, source)
	if err != nil {
		return fmt.Errorf("plugins: admit first-party package: %w", err)
	}
	defer func() { err = errors.Join(err, candidate.Discard()) }()
	release := candidate.Release()
	if len(release.Declaration().Diagnostics) != 0 {
		return errors.New("plugins: first-party package has admission diagnostics")
	}
	id, err := resourceid.ParseInstallation(uuid.NewString())
	if err != nil {
		return err
	}
	installation, err := plugin.New(id, source, release)
	if err != nil {
		return err
	}
	if err := installation.Enable(release); err != nil {
		return err
	}
	if _, err := candidate.Publish(ctx); err != nil {
		return err
	}
	if err := store.Save(ctx, installation); err != nil {
		return err
	}
	return packages.Prepare(ctx, installation, release)
}

func selectInitialProviders(ctx context.Context, store Store, selections SelectionStore, defaults []DefaultProvider) error {
	snapshots, err := store.List(ctx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(defaults))
	for _, initial := range defaults {
		if seen[initial.Role] {
			return errors.New("plugins: duplicate distribution provider role")
		}
		seen[initial.Role] = true
		var chosen *plugin.Snapshot
		for index := range snapshots {
			if snapshots[index].Selected.Declaration().Name != initial.Package {
				continue
			}
			if chosen != nil {
				return errors.New("plugins: distribution provider package is ambiguous")
			}
			chosen = &snapshots[index]
		}
		if chosen == nil {
			continue
		}
		contribution, err := mcpserver.ParseServerName(initial.Contribution)
		if err != nil {
			return err
		}
		service, found := chosen.Selected.Service(plugin.ServiceKind(initial.Role))
		if !found || service.Server != contribution {
			return errors.New("plugins: distribution provider does not declare its selected role")
		}
		selection, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: initial.Role, Installation: chosen.Installation.ID(), Contribution: contribution})
		if err != nil {
			return err
		}
		if err := selections.Save(ctx, selection); err != nil {
			return err
		}
	}
	return nil
}
