package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

type ProviderSelectionStore struct{ db *sql.DB }

func NewProviderSelectionStore(db *sql.DB) *ProviderSelectionStore {
	return &ProviderSelectionStore{db: db}
}

func (s *ProviderSelectionStore) Get(ctx context.Context, role string) (plugin.ProviderSelection, bool, error) {
	var installation, contribution string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT installation_id,contribution_id FROM plugin_provider_selections WHERE role=?`, role).Scan(&installation, &contribution)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.ProviderSelection{}, false, nil
	}
	if err != nil {
		return plugin.ProviderSelection{}, false, err
	}
	value, err := restoreProviderSelection(role, installation, contribution)
	return value, err == nil, err
}

func (s *ProviderSelectionStore) List(ctx context.Context) ([]plugin.ProviderSelection, error) {
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, `SELECT role,installation_id,contribution_id FROM plugin_provider_selections ORDER BY role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []plugin.ProviderSelection
	for rows.Next() {
		var role, installation, contribution string
		if err := rows.Scan(&role, &installation, &contribution); err != nil {
			return nil, err
		}
		value, err := restoreProviderSelection(role, installation, contribution)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func restoreProviderSelection(role, installation, contribution string) (plugin.ProviderSelection, error) {
	id, err := resourceid.ParseInstallation(installation)
	if err != nil {
		return plugin.ProviderSelection{}, err
	}
	name, err := plugin.ParseContributionID(contribution)
	if err != nil {
		return plugin.ProviderSelection{}, err
	}
	return plugin.NewProviderSelection(plugin.SelectionRecord{Role: role, Installation: id, Contribution: name})
}

func (s *ProviderSelectionStore) Save(ctx context.Context, selection plugin.ProviderSelection) error {
	if err := selection.Validate(); err != nil {
		return err
	}
	record := selection.Snapshot()
	_, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_provider_selections(role,installation_id,contribution_id) VALUES(?,?,?) ON CONFLICT(role) DO UPDATE SET installation_id=excluded.installation_id,contribution_id=excluded.contribution_id`, record.Role, record.Installation.String(), record.Contribution.String())
	return err
}
