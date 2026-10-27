package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/google/uuid"
)

type initializationStore interface {
	Store
	Initialized(context.Context) (bool, error)
	MarkInitialized(context.Context) error
}

type DefaultProvider struct {
	Role, Package, Contribution string
}

// Initialize commits the distribution catalog and configured provider choices
// once. Installation and composition owners advance their own facts thereafter.
func Initialize(ctx context.Context, host *qimu.Host, store initializationStore, packages packages, sources func() ([]string, error), selections SelectionStore, defaults []DefaultProvider) error {
	if host == nil {
		return plugin.ErrInvalid
	}
	initialized, err := store.Initialized(ctx)
	if err != nil || initialized {
		return err
	}
	paths, err := sources()
	if err != nil {
		return err
	}
	if len(paths) == 0 || len(paths) > plugin.MaxInstallations {
		return errors.New("plugins: distribution requires a bounded first-party catalog")
	}
	for _, source := range paths {
		if err := installFirstParty(ctx, host, store, packages, source); err != nil {
			return err
		}
	}
	if err := selectInitialProviders(ctx, store, selections, defaults); err != nil {
		return err
	}
	return host.Admission().Admit(ctx, func(admitted context.Context) error { return store.MarkInitialized(admitted) })
}

func installFirstParty(ctx context.Context, host *qimu.Host, store Store, packages packages, source string) (err error) {
	candidate, err := packages.Materialize(ctx, source)
	if err != nil {
		return fmt.Errorf("plugins: admit first-party package: %w", err)
	}
	defer func() { err = errors.Join(err, candidate.Discard()) }()
	release := candidate.Release()
	if len(release.Declaration().Diagnostics) != 0 {
		return errors.New("plugins: first-party package has admission diagnostics")
	}
	id, err := identity.ParseInstallation(uuid.NewSHA1(uuid.NameSpaceURL, []byte("io.github.tangerg.flame/first-party/"+release.Declaration().Name)).String())
	if err != nil {
		return err
	}
	if existing, found, err := store.Get(ctx, id); err != nil {
		return err
	} else if found {
		if existing.Selected.Digest() != release.Digest() || existing.Installation.Snapshot().Source != source {
			return fmt.Errorf("%w: unfinished distribution differs from its original source", plugin.ErrStale)
		}
		return packages.Prepare(ctx, existing.Installation, existing.Selected)
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
	projection, err := ProjectFrameworkRelease(release)
	if err != nil {
		return err
	}
	if err := RegisterFrameworkRelease(host, projection); err != nil {
		return err
	}
	configuration, err := EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		return err
	}
	if _, err := host.Install(ctx, qimu.InstallationConfig{ID: qimu.InstallationID(id.String()), Plugin: projection.Plugin, Configuration: configuration, Enabled: true}); err != nil {
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
	existing, err := selections.List(ctx)
	if err != nil {
		return err
	}
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
		contribution, err := plugin.ParseContributionID(initial.Contribution)
		if err != nil {
			return err
		}
		declared, found := chosen.Selected.Contribution(contribution)
		if !found {
			return errors.New("plugins: distribution contribution is not declared")
		}
		contract, err := portable.ParseCapability(declared.Contract.JSON())
		if err != nil {
			return err
		}
		descriptor := contract.Descriptor()
		if descriptor.Reference != initial.Role || descriptor.Composition != portable.Selection {
			return errors.New("plugins: distribution provider does not declare its selected role")
		}
		selection, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: initial.Role, Installation: chosen.Installation.ID(), Contribution: contribution})
		if err != nil {
			return err
		}
		completed := false
		for _, current := range existing {
			if current.Snapshot().Role != initial.Role {
				continue
			}
			if current.Snapshot() != selection.Snapshot() {
				return errors.New("plugins: unfinished distribution provider differs from its original choice")
			}
			completed = true
		}
		if completed {
			continue
		}
		if err := selections.Save(ctx, selection); err != nil {
			return err
		}
	}
	return nil
}
