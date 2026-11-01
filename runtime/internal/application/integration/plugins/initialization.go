package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/qimu"
	"github.com/Tangerg/qimu/portable"
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
	if err := installDistribution(ctx, host, store, packages, paths); err != nil {
		return err
	}
	if err := selectInitialProviders(ctx, store, selections, defaults); err != nil {
		return err
	}
	return host.Admission().Admit(ctx, func(admitted context.Context) error { return store.MarkInitialized(admitted) })
}

// Preflight holds the same immutable candidates through publication. Failure to
// identify any source leaves existing installation decisions untouched.
func installDistribution(ctx context.Context, host *qimu.Host, store Store, packages packages, paths []string) (err error) {
	distribution := make([]distributionCandidate, 0, len(paths))
	defer func() {
		for _, source := range distribution {
			err = errors.Join(err, source.candidate.Discard())
		}
	}()
	names := make(map[string]string, len(paths))
	for _, source := range paths {
		candidate, err := packages.Materialize(ctx, source)
		if err != nil {
			return fmt.Errorf("plugins: preflight distribution source %q: %w", source, err)
		}
		distribution = append(distribution, distributionCandidate{source: source, candidate: candidate})
		name := candidate.Release().Declaration().Name
		if previous, exists := names[name]; exists {
			return fmt.Errorf("plugins: distribution sources %q and %q publish the same package %q", previous, source, name)
		}
		names[name] = source
	}
	for _, source := range distribution {
		if err := source.install(ctx, host, store, packages); err != nil {
			return fmt.Errorf("plugins: initialize distribution source %q: %w", source.source, err)
		}
	}
	return nil
}

type distributionCandidate struct {
	source    string
	candidate Candidate
}

func (entry distributionCandidate) install(ctx context.Context, host *qimu.Host, store Store, packages packages) error {
	release := entry.candidate.Release()
	source := entry.source
	id, err := firstPartyID(release.Declaration().Name)
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
	// Keep identifiable failed releases visible to installation inspection and repair.
	// Only a release without admission findings can participate in default roles.
	if len(release.Declaration().Diagnostics) == 0 {
		installation, err = installation.Enable(release)
		if err != nil {
			return err
		}
	}
	if _, err := entry.candidate.Publish(ctx); err != nil {
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
	if _, err := host.Install(ctx, qimu.InstallationConfig{ID: id.Host(), Plugin: projection.Plugin, Configuration: configuration, Enabled: installation.Enabled()}); err != nil {
		return err
	}
	return packages.Prepare(ctx, installation, release)
}

func firstPartyID(name string) (identity.InstallationID, error) {
	return identity.ParseInstallation(uuid.NewSHA1(uuid.NameSpaceURL, []byte("io.github.tangerg.flame/first-party/"+name)).String())
}

func selectInitialProviders(ctx context.Context, store Store, selections SelectionStore, defaults []DefaultProvider) error {
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
		id, err := firstPartyID(initial.Package)
		if err != nil {
			return err
		}
		chosen, found, err := store.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found || !chosen.Installation.Enabled() {
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
		for _, role := range declared.SelectionRoles() {
			if err := initializeSelection(ctx, selections, existing, plugin.SelectionRecord{Role: role, Installation: chosen.Installation.ID(), Contribution: contribution}); err != nil {
				return err
			}
		}
	}
	return nil
}

func initializeSelection(ctx context.Context, selections SelectionStore, existing []plugin.ProviderSelection, record plugin.SelectionRecord) error {
	for _, current := range existing {
		if current.Snapshot().Role != record.Role {
			continue
		}
		if current.Snapshot() != record {
			return errors.New("plugins: unfinished distribution provider differs from its original choice")
		}
		return nil
	}
	selection, err := plugin.NewProviderSelection(record)
	if err != nil {
		return err
	}
	return selections.Save(ctx, selection)
}
