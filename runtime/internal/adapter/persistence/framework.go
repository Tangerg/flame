package persistence

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
	"github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
)

const flameOwner = "flame"

func openFrameworkDatabase(ctx context.Context, path string) (sqlite.Store, framehost.Admission, *InstallationStore, error) {
	admission := framehost.NewAdmission()
	owner := &frameworkOwner{admission: admission}
	store, err := sqlite.Open(ctx, sqlite.Config{
		Path: path, Admission: admission,
		Owners: []sqlite.OwnerSchema{{
			ID: flameOwner, Statements: currentSchema(),
			References: owner.references, ProjectInstallation: owner.projectInstallation,
		}},
	})
	if err != nil {
		if store.Valid() {
			err = errors.Join(err, store.Close())
		}
		return sqlite.Store{}, framehost.Admission{}, nil, err
	}
	owner.db, err = newDatabase(store, admission)
	if err != nil {
		return sqlite.Store{}, framehost.Admission{}, nil, errors.Join(err, store.Close())
	}
	owner.installations = newInstallationStore(newReleaseStore(owner.db))
	useOwner, err := store.Owner(flameOwner)
	if err == nil {
		err = admission.RegisterUseOwner(useOwner)
	}
	if err != nil {
		return sqlite.Store{}, framehost.Admission{}, nil, errors.Join(err, store.Close())
	}
	return store, admission, owner.installations, nil
}

type frameworkOwner struct {
	db            *database
	admission     framehost.Admission
	installations *InstallationStore
}

func (o *frameworkOwner) references(ctx context.Context, executor sqlite.Reader, installation framehost.InstallationID) ([]framevalue.Digest, error) {
	var releases []framevalue.Digest
	err := o.db.withReader(ctx, executor, func(ctx context.Context) error {
		checkpoints, err := newExecutorCheckpointStore(o.db, o.admission).PendingCheckpointDependencies(ctx)
		if err != nil {
			return err
		}
		commands, err := pendingCommandDependencies(ctx, o.db)
		if err != nil {
			return err
		}
		for _, dependency := range append(checkpoints, commands...) {
			if dependency.InstallationID.Host() == installation {
				releases = append(releases, dependency.Digest)
			}
		}
		return nil
	})
	return releases, err
}

func (o *frameworkOwner) projectInstallation(ctx context.Context, executor sqlite.Executor, change framehost.InstallationChange) error {
	return o.db.withExecutor(ctx, executor, func(ctx context.Context) error {
		if change.After == nil {
			if change.Before == nil {
				return plugin.ErrInvalid
			}
			_, err := executor.ExecContext(ctx, `DELETE FROM plugin_installations WHERE id=?`, change.Before.ID)
			return err
		}
		record, err := plugins.DecodeFrameworkConfiguration(*change.After)
		if err != nil {
			return err
		}
		if change.Before != nil && change.Before.Release == change.After.Release {
			staged, err := stagedRelease(ctx, executor, record.ID.String())
			if err != nil {
				return err
			}
			if staged != nil {
				record.Staged = staged
			}
		}
		return o.installations.projectRecord(ctx, record)
	})
}

func stagedRelease(ctx context.Context, executor sqlite.Executor, installationID string) (*framevalue.Digest, error) {
	var staged *string
	if err := executor.QueryRowContext(ctx, `SELECT staged_digest FROM plugin_installations WHERE id=?`, installationID).Scan(&staged); err != nil {
		return nil, err
	}
	if staged == nil {
		return nil, nil
	}
	digest, err := framevalue.ParseDigest(*staged)
	if err != nil {
		return nil, err
	}
	return &digest, nil
}
