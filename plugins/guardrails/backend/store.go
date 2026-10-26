package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
)

type store struct{ db *sql.DB }

var errStoredPolicy = errors.New("guardrails: invalid stored policy")

func openStore(ctx context.Context, path string) (*store, error) {
	initial, err := json.Marshal(guardrail.NewPolicy().Snapshot())
	if err != nil {
		return nil, err
	}
	db, err := sqlite.OpenSchema(ctx, path, []string{
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

func (s *store) read(ctx context.Context) (guardrail.Policy, error) {
	var encoded string
	if err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM policy WHERE id=1`).Scan(&encoded); err != nil {
		return guardrail.Policy{}, errors.Join(errStoredPolicy, err)
	}
	var snapshot guardrail.Snapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return guardrail.Policy{}, errors.Join(errStoredPolicy, err)
	}
	policy, err := guardrail.Restore(snapshot)
	if err != nil {
		return guardrail.Policy{}, errors.Join(errStoredPolicy, err)
	}
	return policy, nil
}

func (s *store) replace(ctx context.Context, expected int64, phrases []string) (guardrail.Summary, error) {
	policy, err := s.read(ctx)
	if err != nil {
		return guardrail.Summary{}, err
	}
	next, err := policy.Replace(expected, phrases)
	if err != nil {
		return guardrail.Summary{}, err
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return guardrail.Summary{}, err
	}
	_, err = sqlite.Connection(ctx, s.db).ExecContext(ctx, `UPDATE policy SET snapshot=? WHERE id=1`, string(encoded))
	return next.Summary(), err
}
