package sqlite

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func (e *ExecutorCheckpointStore) replaceReferences(ctx context.Context, checkpoint ExecutorCheckpointRecord) error {
	if err := e.replaceToolResultReferences(ctx, checkpoint); err != nil {
		return err
	}
	return e.replaceInstallationDependencies(ctx, checkpoint)
}

// The dependency rows advance in the transaction that advances the payload, so
// a checkpoint whose dependencies are unknown cannot exist.
func (e *ExecutorCheckpointStore) replaceInstallationDependencies(ctx context.Context, checkpoint ExecutorCheckpointRecord) error {
	if _, err := conn(ctx, e.db).ExecContext(ctx, `DELETE FROM executor_checkpoint_installations WHERE root_member_id = ?`, checkpoint.RootMemberID); err != nil {
		return fmt.Errorf("sqlite: replace executor checkpoint %q dependencies: %w", checkpoint.RootMemberID, err)
	}
	for _, dependency := range checkpoint.Installations {
		if _, err := conn(ctx, e.db).ExecContext(ctx,
			`INSERT INTO executor_checkpoint_installations(root_member_id, installation_id, digest) VALUES (?, ?, ?)`,
			checkpoint.RootMemberID, dependency.InstallationID.String(), dependency.Digest.String(),
		); err != nil {
			return fmt.Errorf("sqlite: record executor checkpoint %q dependency: %w", checkpoint.RootMemberID, err)
		}
	}
	return nil
}

func (e *ExecutorCheckpointStore) installationDependencies(ctx context.Context, rootID string) ([]plugin.Dependency, error) {
	rows, err := conn(ctx, e.db).QueryContext(ctx,
		`SELECT installation_id, digest FROM executor_checkpoint_installations WHERE root_member_id = ? ORDER BY installation_id, digest`,
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
		if dependency.InstallationID, err = resourceid.ParseInstallation(installation); err != nil {
			return nil, fmt.Errorf("sqlite: decode executor checkpoint %q dependency: %w", rootID, err)
		}
		if dependency.Digest, err = fingerprint.ParseDigest(digest); err != nil {
			return nil, fmt.Errorf("sqlite: decode executor checkpoint %q dependency: %w", rootID, err)
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, rows.Err()
}

// PendingCheckpointDependencies is the canonical set of releases every pending
// checkpoint holds. It answers from the dependency projection alone and never
// reads a continuation payload.
func (e *ExecutorCheckpointStore) PendingCheckpointDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	rows, err := conn(ctx, e.db).QueryContext(ctx,
		`SELECT DISTINCT dependency.installation_id, dependency.digest
		   FROM executor_checkpoint_installations dependency
		   JOIN executor_checkpoints checkpoint ON checkpoint.root_member_id = dependency.root_member_id
		  WHERE EXISTS (SELECT 1 FROM runs WHERE runs.session_id = checkpoint.session_id AND runs.state <> 'terminal')`,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: checkpoint dependency query: %w", err)
	}
	defer rows.Close()
	var dependencies []plugin.Dependency
	for rows.Next() {
		var installation, digest string
		if err := rows.Scan(&installation, &digest); err != nil {
			return nil, fmt.Errorf("sqlite: scan checkpoint dependency: %w", err)
		}
		var dependency plugin.Dependency
		if dependency.InstallationID, err = resourceid.ParseInstallation(installation); err != nil {
			return nil, fmt.Errorf("sqlite: decode checkpoint dependency: %w", err)
		}
		if dependency.Digest, err = fingerprint.ParseDigest(digest); err != nil {
			return nil, fmt.Errorf("sqlite: decode checkpoint dependency: %w", err)
		}
		dependencies = append(dependencies, dependency)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: checkpoint dependency query: %w", err)
	}
	return plugin.CompactDependencies(dependencies), nil
}
