package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

type CompositionStore struct{ db *sql.DB }

type storedComposition struct {
	Contract jsoninput.Input           `json:"contract"`
	Members  []storedCompositionMember `json:"members"`
}

type storedCompositionMember struct {
	Installation string `json:"installation"`
	Contribution string `json:"contribution"`
}

func NewCompositionStore(db *sql.DB) *CompositionStore { return &CompositionStore{db: db} }

func (s *CompositionStore) Get(ctx context.Context, reference string) (plugin.OrderedComposition, bool, error) {
	var encoded string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT composition_json FROM plugin_compositions WHERE reference=?`, reference).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.OrderedComposition{}, false, nil
	}
	if err != nil {
		return plugin.OrderedComposition{}, false, err
	}
	value, err := restoreComposition(reference, encoded)
	return value, err == nil, err
}

func (s *CompositionStore) List(ctx context.Context) ([]plugin.OrderedComposition, error) {
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, `SELECT reference,composition_json FROM plugin_compositions ORDER BY reference`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []plugin.OrderedComposition{}
	for rows.Next() {
		var reference, encoded string
		if err := rows.Scan(&reference, &encoded); err != nil {
			return nil, err
		}
		value, err := restoreComposition(reference, encoded)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func restoreComposition(reference, encoded string) (plugin.OrderedComposition, error) {
	var stored storedComposition
	if err := decodeStoredJSON([]byte(encoded), &stored); err != nil {
		return plugin.OrderedComposition{}, err
	}
	members := make([]plugin.CompositionMember, len(stored.Members))
	for index, member := range stored.Members {
		id, err := resourceid.ParseInstallation(member.Installation)
		if err != nil {
			return plugin.OrderedComposition{}, err
		}
		contribution, err := plugin.ParseContributionID(member.Contribution)
		if err != nil {
			return plugin.OrderedComposition{}, err
		}
		members[index] = plugin.CompositionMember{Installation: id, Contribution: contribution}
	}
	record := plugin.CompositionRecord{Contract: stored.Contract, Members: members}
	value, err := plugin.NewOrderedComposition(record)
	if err != nil {
		return plugin.OrderedComposition{}, err
	}
	contract, err := capability.Parse(value.Snapshot().Contract.JSON())
	if err != nil || contract.Descriptor().Reference != reference {
		return plugin.OrderedComposition{}, errors.Join(err, errors.New("sqlite: composition reference differs from its contract"))
	}
	return value, nil
}

func (s *CompositionStore) Save(ctx context.Context, value plugin.OrderedComposition) error {
	if err := value.Validate(); err != nil {
		return err
	}
	record := value.Snapshot()
	contract, err := capability.Parse(record.Contract.JSON())
	if err != nil {
		return err
	}
	members := make([]storedCompositionMember, len(record.Members))
	for index, member := range record.Members {
		members[index] = storedCompositionMember{Installation: member.Installation.String(), Contribution: member.Contribution.String()}
	}
	encoded, err := encodeStoredJSON(storedComposition{Contract: record.Contract, Members: members})
	if err != nil {
		return err
	}
	_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_compositions(reference,composition_json) VALUES(?,?) ON CONFLICT(reference) DO UPDATE SET composition_json=excluded.composition_json`, contract.Descriptor().Reference, string(encoded))
	return err
}
