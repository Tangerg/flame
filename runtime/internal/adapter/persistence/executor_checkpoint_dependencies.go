package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (s *ExecutorCheckpointStore) replaceReferences(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	if err := s.replaceToolResultReferences(ctx, checkpoint); err != nil {
		return err
	}
	if err := s.replaceResourceReferences(ctx, checkpoint); err != nil {
		return err
	}
	if _, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM executor_checkpoint_bindings WHERE root_member_id=?`, checkpoint.RootMemberID); err != nil {
		return err
	}
	for index, activation := range checkpoint.Bindings {
		if _, err := plugins.DecodeActivation(activation); err != nil {
			return err
		}
		if _, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO executor_checkpoint_bindings(root_member_id,position,binding_json) VALUES(?,?,?)`, checkpoint.RootMemberID, index, activation.Binding.String()); err != nil {
			return err
		}
	}
	return nil
}

// The dependency rows advance in the transaction that advances the payload, so
// a checkpoint whose dependencies are unknown cannot exist.
func (s *ExecutorCheckpointStore) replaceResourceReferences(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	if _, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM executor_checkpoint_resources WHERE root_member_id = ?`, checkpoint.RootMemberID); err != nil {
		return fmt.Errorf("sqlite: replace executor checkpoint %q dependencies: %w", checkpoint.RootMemberID, err)
	}
	for _, dependency := range checkpoint.Resources {
		if _, err := sqlite.Connection(ctx, s.db).ExecContext(ctx,
			`INSERT INTO executor_checkpoint_resources(root_member_id, installation_id, digest) VALUES (?, ?, ?)`,
			checkpoint.RootMemberID, dependency.InstallationID.String(), dependency.Digest.String(),
		); err != nil {
			return fmt.Errorf("sqlite: record executor checkpoint %q dependency: %w", checkpoint.RootMemberID, err)
		}
	}
	return nil
}

func (s *ExecutorCheckpointStore) resourceReferences(ctx context.Context, rootID string) ([]plugin.Dependency, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx,
		`SELECT installation_id, digest FROM executor_checkpoint_resources WHERE root_member_id = ? ORDER BY installation_id, digest`,
		rootID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: load executor checkpoint %q dependencies: %w", rootID, err)
	}
	defer rows.Close()
	var dependencies []plugin.Dependency
	for rows.Next() {
		var installation, digest string
		if err := rows.Scan(&installation, &digest); err != nil {
			return nil, fmt.Errorf("sqlite: scan executor checkpoint %q dependency: %w", rootID, err)
		}
		var dependency plugin.Dependency
		if dependency.InstallationID, err = identity.ParseInstallation(installation); err != nil {
			return nil, fmt.Errorf("sqlite: decode executor checkpoint %q dependency: %w", rootID, err)
		}
		if dependency.Digest, err = identity.ParseDigest(digest); err != nil {
			return nil, fmt.Errorf("sqlite: decode executor checkpoint %q dependency: %w", rootID, err)
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, rows.Err()
}

func (s *ExecutorCheckpointStore) checkpointReferences(ctx context.Context, root string) ([]plugin.Activation, []plugin.Dependency, error) {
	resources, err := s.resourceReferences(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT binding_json FROM executor_checkpoint_bindings WHERE root_member_id=? ORDER BY position`, root)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var bindings []plugin.Activation
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, nil, err
		}
		binding, err := plugins.ReadActivation([]byte(data))
		if err != nil {
			return nil, nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, resources, rows.Err()
}

// PendingCheckpointDependencies reads retained execution graphs and immutable
// content references, without interpreting the executor's continuation payload.
func (s *ExecutorCheckpointStore) PendingCheckpointDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	var result []plugin.Dependency
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		bindings, resources, err := s.pendingReferences(ctx)
		if err != nil {
			return err
		}
		result, err = checkpointDependencies(bindings, resources)
		return err
	})
	return plugin.CompactDependencies(result), err
}

func (s *ExecutorCheckpointStore) pendingReferences(ctx context.Context) ([]plugin.Activation, []plugin.Dependency, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT root_member_id FROM executor_checkpoints checkpoint WHERE EXISTS (SELECT 1 FROM runs WHERE runs.session_id=checkpoint.session_id AND runs.state <> ?) ORDER BY root_member_id`, runStateTerminal.databaseValue())
	if err != nil {
		return nil, nil, err
	}
	var roots []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		roots = append(roots, root)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, nil, err
	}
	var bindings []plugin.Activation
	var resources []plugin.Dependency
	for _, root := range roots {
		original, content, err := s.checkpointReferences(ctx, root)
		if err != nil {
			return nil, nil, err
		}
		bindings = append(bindings, original...)
		resources = append(resources, content...)
	}
	return bindings, resources, nil
}
