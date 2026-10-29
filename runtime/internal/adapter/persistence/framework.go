package persistence

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// The framework authority and original Flame business owners share one file
// and one admission. The installation tables are indexes of committed Host
// records; only the Host's transaction can advance them.
func openFrameworkDatabase(ctx context.Context, path string) (*sqlite.Store, *qimu.Admission, *InstallationStore, error) {
	admission := qimu.NewAdmission()
	var store *sqlite.Store
	var installations *InstallationStore
	var err error
	store, err = sqlite.Open(ctx, sqlite.Config{
		Path: path, Admission: admission,
		Owners: []sqlite.OwnerSchema{{
			ID: "flame", Statements: currentSchema(),
			References: func(ctx context.Context, executor sqlite.Executor, installation qimu.InstallationID) ([]qimu.Dependency, error) {
				var dependencies []qimu.Dependency
				err := store.WithTransaction(ctx, executor, func(ctx context.Context) error {
					checkpoints, err := newExecutorCheckpointStore(store.Database()).PendingCheckpointDependencies(ctx)
					if err != nil {
						return err
					}
					commands, err := newPluginCommandStore(store.Database(), nil).PendingCommandDependencies(ctx)
					if err != nil {
						return err
					}
					for _, dependency := range append(checkpoints, commands...) {
						if dependency.InstallationID.String() == installation.String() {
							dependencies = append(dependencies, qimu.Dependency{InstallationID: installation, Release: qimu.Digest(dependency.Digest.String())})
						}
					}
					return nil
				})
				return qimu.CompactDependencies(dependencies), err
			},
		}},
		ProjectInstallation: func(ctx context.Context, executor sqlite.Executor, change qimu.InstallationChange) error {
			return store.WithTransaction(ctx, executor, func(ctx context.Context) error {
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
					var staged *string
					if err := executor.QueryRowContext(ctx, `SELECT staged_digest FROM plugin_installations WHERE id=?`, record.ID.String()).Scan(&staged); err != nil {
						return err
					}
					if staged != nil {
						digest, err := identity.ParseDigest(*staged)
						if err != nil {
							return err
						}
						record.Staged = &digest
					}
				}
				return installations.projectRecord(ctx, record)
			})
		},
	})
	if err != nil {
		return nil, nil, nil, errors.Join(err, store.Close())
	}
	installations = newInstallationStore(newReleaseStore(store.Database()))
	owner, err := store.Owner("flame")
	if err == nil {
		err = admission.RegisterUseOwner(owner)
	}
	if err != nil {
		return nil, nil, nil, errors.Join(err, store.Close())
	}
	return store, admission, installations, nil
}
