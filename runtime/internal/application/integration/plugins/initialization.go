package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/google/uuid"
)

type InitializationStore interface {
	Store
	Initialize(context.Context, func(context.Context) error) error
}

// Initialize grants only the distribution's initial catalog. Thereafter the
// ordinary installation aggregates own approval, enablement and removal.
func Initialize(ctx context.Context, store InitializationStore, packages Packages, sources func() ([]string, error)) error {
	return store.Initialize(ctx, func(ctx context.Context) (err error) {
		existing, err := store.List(ctx)
		if err != nil {
			return err
		}
		if len(existing) != 0 {
			return errors.New("plugins: uninitialized database contains installations; open a fresh data directory")
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
		return nil
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
	if err := installation.Approve(release); err != nil {
		return err
	}
	if err := installation.Enable(release); err != nil {
		return err
	}
	if err := admitServices(ctx, store, installation, release); err != nil {
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
