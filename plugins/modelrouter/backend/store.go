package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/sqlite"
	"github.com/Tangerg/flame/plugins/modelrouter/backend/domain"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

type store struct{ db *sql.DB }

var errStoredPolicy = errors.New("model router: invalid stored policy")

func openStore(ctx context.Context, path string) (*store, error) {
	initial, err := json.Marshal(domain.NewPolicy().Snapshot())
	if err != nil {
		return nil, err
	}
	db, err := sqlitebase.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS policy (id INTEGER PRIMARY KEY CHECK(id=1), snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`,
		`INSERT INTO policy(id,snapshot) VALUES(1,'` + string(initial) + `')`,
	})
	if err != nil {
		return nil, err
	}
	s := &store{db: db}
	if _, err := s.read(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *store) read(ctx context.Context) (domain.Policy, error) {
	var encoded string
	if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM policy WHERE id=1`).Scan(&encoded); err != nil {
		return domain.Policy{}, errors.Join(errStoredPolicy, err)
	}
	var snapshot domain.Snapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return domain.Policy{}, errors.Join(errStoredPolicy, err)
	}
	policy, err := domain.Restore(snapshot)
	if err != nil {
		return domain.Policy{}, errors.Join(errStoredPolicy, err)
	}
	return policy, nil
}

func (s *store) replace(ctx context.Context, expected int64, rules []domain.Rule) (domain.Snapshot, error) {
	policy, err := s.read(ctx)
	if err != nil {
		return domain.Snapshot{}, err
	}
	next, err := policy.Replace(expected, rules)
	if err != nil {
		return domain.Snapshot{}, err
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return domain.Snapshot{}, err
	}
	_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `UPDATE policy SET snapshot=? WHERE id=1`, string(encoded))
	return next.Snapshot(), err
}
