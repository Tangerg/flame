package maintenance

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

type dependencyMemory struct{}

func (dependencyMemory) AppendLedger(context.Context, agentmemory.FactBatch) ([]agentmemory.LedgerFact, error) {
	return nil, nil
}

func (dependencyMemory) PendingLedger(context.Context, string, int64, int) ([]agentmemory.LedgerFact, error) {
	return nil, nil
}

func (dependencyMemory) State(context.Context, string) (agentmemory.State, error) {
	return agentmemory.State{}, nil
}

func (dependencyMemory) PublishGeneration(context.Context, agentmemory.Publication) (bool, error) {
	return false, nil
}

func (dependencyMemory) Items(context.Context, agentmemory.Scope, string) ([]agentmemory.Item, error) {
	return nil, nil
}

func TestMaintenanceConstructorsRejectMissingDependencies(t *testing.T) {
	history := newCompactionTestStore()
	memory := dependencyMemory{}
	proposals := &fakeProposalSubmitter{}
	skills := &fakeIdleSkillArchiver{}
	var typedNilHistory *compactionTestStore
	var typedNilSource *fakeSkillSource

	tests := map[string]func() error{
		"compactor typed-nil conversation store": func() error {
			_, err := NewCompactor(typedNilHistory, unexpectedClient, nil, CompactionPolicyValues{}, nil)
			return err
		},
		"compactor utility model resolver": func() error {
			_, err := NewCompactor(history, nil, nil, CompactionPolicyValues{}, nil)
			return err
		},
		"memory conversation reader": func() error {
			_, err := NewMemoryConsolidator(nil, memory, unexpectedClient)
			return err
		},
		"memory store": func() error {
			_, err := NewMemoryConsolidator(history, nil, unexpectedClient)
			return err
		},
		"memory utility model resolver": func() error {
			_, err := NewMemoryConsolidator(history, memory, nil)
			return err
		},
		"skill conversation reader": func() error {
			_, err := NewSkillProposalMiner(nil, proposals, nil, unexpectedClient)
			return err
		},
		"skill proposal submitter": func() error {
			_, err := NewSkillProposalMiner(history, nil, nil, unexpectedClient)
			return err
		},
		"skill typed-nil source": func() error {
			_, err := NewSkillProposalMiner(history, proposals, typedNilSource, unexpectedClient)
			return err
		},
		"skill utility model resolver": func() error {
			_, err := NewSkillProposalMiner(history, proposals, fakeSkillSource{}, nil)
			return err
		},
		"skill archive curator": func() error {
			_, err := NewIdleSkillArchiver(nil)
			return err
		},
	}
	for name, construct := range tests {
		t.Run(name, func(t *testing.T) {
			if err := construct(); err == nil {
				t.Fatal("construction succeeded with a missing dependency")
			}
		})
	}
	if _, err := NewIdleSkillArchiver(skills); err != nil {
		t.Fatalf("valid construction failed: %v", err)
	}
}
