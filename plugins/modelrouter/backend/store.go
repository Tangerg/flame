package main

import (
	"context"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"

	"github.com/Tangerg/frame/sqlite"
)

const policyTable = "policy"

type store struct {
	db     sqlite.Database
	policy *pluginstorage.Snapshot[routing.Snapshot]
}

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		pluginstorage.SnapshotSchema(policyTable),
	})
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	policy, err := pluginstorage.OpenSnapshot(ctx, db, policyTable, routing.NewPolicy().Snapshot())
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	s := &store{db: db, policy: policy}
	if _, err := s.read(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *store) read(ctx context.Context) (routing.Policy, error) {
	snapshot, err := s.policy.Read(ctx)
	if err != nil {
		return routing.Policy{}, err
	}
	policy, err := routing.Restore(snapshot)
	if err != nil {
		return routing.Policy{}, errors.Join(pluginstorage.ErrStoredSnapshot, err)
	}
	return policy, nil
}

func (s *store) prepareReplacement(ctx context.Context, expected int64, rules []routing.Rule) (pluginstorage.CommandPlan[routing.Snapshot], error) {
	policy, err := s.read(ctx)
	if err != nil {
		return pluginstorage.CommandPlan[routing.Snapshot]{}, err
	}
	next, err := policy.Replace(expected, rules)
	if err != nil {
		return pluginstorage.CommandPlan[routing.Snapshot]{}, err
	}
	return pluginstorage.Replace(s.policy, next.Snapshot(), next.Snapshot())
}
