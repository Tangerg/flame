package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	framevalue "github.com/Tangerg/frame/value"
)

type compositionStore struct{ db *database }
type storedComposition struct {
	Contract framevalue.JSON           `json:"contract"`
	Members  []storedCompositionMember `json:"members"`
}
type storedCompositionMember struct {
	Installation string `json:"installation"`
	Contribution string `json:"contribution"`
}

func newCompositionStore(db *database) *compositionStore {
	return &compositionStore{db: db}
}
func (s *compositionStore) Get(ctx context.Context, reference string) (plugin.OrderedComposition, bool, error) {
	return scopedPair(ctx, s.db.read, func(ctx context.Context) (plugin.OrderedComposition, bool, error) {
		var encoded string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT composition_json FROM plugin_compositions WHERE reference=?`, reference).Scan(&encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return plugin.OrderedComposition{}, false, nil
		}
		if err != nil {
			return plugin.OrderedComposition{}, false, err
		}
		value, err := restoreComposition(reference, encoded)
		return value, err == nil, err
	})
}
func (s *compositionStore) List(ctx context.Context) ([]plugin.OrderedComposition, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]plugin.OrderedComposition, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT reference,composition_json FROM plugin_compositions ORDER BY reference`)
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
	})
}
func restoreComposition(reference, encoded string) (plugin.OrderedComposition, error) {
	var stored storedComposition
	if err := decodeStoredJSON([]byte(encoded), &stored); err != nil {
		return plugin.OrderedComposition{}, err
	}
	members := make([]plugin.CompositionMember, len(stored.Members))
	for index, member := range stored.Members {
		id, err := identity.ParseInstallation(member.Installation)
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
	contract, err := plugincontract.ParseCapability(value.Snapshot().Contract.Bytes())
	if err != nil || contract.Descriptor().Reference != reference {
		return plugin.OrderedComposition{}, errors.Join(err, errors.New("sqlite: composition reference differs from its contract"))
	}
	return value, nil
}
func (s *compositionStore) Save(ctx context.Context, value plugin.OrderedComposition) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		if err := value.Validate(); err != nil {
			return err
		}
		record := value.Snapshot()
		contract, err := plugincontract.ParseCapability(record.Contract.Bytes())
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
		_, err = s.db.connection(ctx).ExecContext(ctx, `INSERT INTO plugin_compositions(reference,composition_json) VALUES(?,?) ON CONFLICT(reference) DO UPDATE SET composition_json=excluded.composition_json`, contract.Descriptor().Reference, string(encoded))
		return err
	})
}
