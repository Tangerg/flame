package main

import (
	"context"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"

	"github.com/Tangerg/frame/sqlite"
)

const policyTable = "policy"

type store struct {
	db     sqlite.Database
	policy *pluginstorage.Snapshot[guardrail.Snapshot]
}

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		pluginstorage.SnapshotSchema(policyTable),
	})
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	policy, err := pluginstorage.OpenSnapshot(ctx, db, policyTable, guardrail.NewPolicy().Snapshot())
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	s := &store{db: db, policy: policy}
	if _, err := s.read(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *store) read(ctx context.Context) (guardrail.Policy, error) {
	snapshot, err := s.policy.Read(ctx)
	if err != nil {
		return guardrail.Policy{}, err
	}
	policy, err := guardrail.Restore(snapshot)
	if err != nil {
		return guardrail.Policy{}, errors.Join(pluginstorage.ErrStoredSnapshot, err)
	}
	return policy, nil
}

func (s *store) prepareReplacement(ctx context.Context, expected int64, phrases []string) (pluginstorage.CommandPlan[guardrail.Summary], error) {
	policy, err := s.read(ctx)
	if err != nil {
		return pluginstorage.CommandPlan[guardrail.Summary]{}, err
	}
	next, err := policy.Replace(expected, phrases)
	if err != nil {
		return pluginstorage.CommandPlan[guardrail.Summary]{}, err
	}
	return pluginstorage.Replace(s.policy, next.Summary(), next.Snapshot())
}
