package persistence

import (
	"context"
	"errors"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"

	"github.com/Tangerg/frame/sqlite"
)

// The framework authority and original Flame business owners share one file
// and one admission. The installation tables are indexes of committed Host
// records; only the Host's transaction can advance them.
func openFrameworkDatabase(ctx context.Context, path string) (*sqlite.Store, *frame.Admission, *InstallationStore, error) {
	admission := frame.NewAdmission()
	var store *sqlite.Store
	var db *database
	var installations *InstallationStore
	var err error
	store, err = sqlite.Open(ctx, sqlite.Config{
		Path: path, Admission: admission,
		Owners: []sqlite.OwnerSchema{{
			ID: "flame", Statements: currentSchema(),
			References: func(ctx context.Context, executor sqlite.Reader, installation frame.InstallationID) ([]frame.Dependency, error) {
				var dependencies []frame.Dependency
				err := db.withReader(ctx, executor, func(ctx context.Context) error {
					checkpoints, err := newExecutorCheckpointStore(db).PendingCheckpointDependencies(ctx)
					if err != nil {
						return err
					}
					commands, err := newPluginCommandStore(db, nil).PendingCommandDependencies(ctx)
					if err != nil {
						return err
					}
					for _, dependency := range append(checkpoints, commands...) {
						if dependency.InstallationID.String() == installation.String() {
							dependencies = append(dependencies, frame.Dependency{InstallationID: installation, Release: dependency.Digest})
						}
					}
					return nil
				})
				return frame.CompactDependencies(dependencies), err
			},
			ProjectInstallation: func(ctx context.Context, executor sqlite.Executor, change frame.InstallationChange) error {
				return db.withExecutor(ctx, executor, func(ctx context.Context) error {
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
							digest, err := framevalue.ParseDigest(*staged)
							if err != nil {
								return err
							}
							record.Staged = &digest
						}
					}
					return installations.projectRecord(ctx, record)
				})
			},
		}},
	})
	if err != nil {
		if store != nil {
			err = errors.Join(err, store.Close())
		}
		return nil, nil, nil, err
	}
	db, err = newDatabase(store, admission)
	if err != nil {
		return nil, nil, nil, errors.Join(err, store.Close())
	}
	installations = newInstallationStore(newReleaseStore(db))
	owner, err := store.Owner("flame")
	if err == nil {
		err = admission.RegisterUseOwner(owner)
	}
	if err != nil {
		return nil, nil, nil, errors.Join(err, store.Close())
	}
	return store, admission, installations, nil
}
