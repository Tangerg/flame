package maintenance

import (
	"context"
	"testing"
	"time"

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

func (dependencyMemory) PublishGeneration(context.Context, string, int64, int64, []string, time.Time) (bool, error) {
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
			_, err := NewMemoryConsolidator(nil, memory, unexpectedClient, MemoryCurationPolicyValues{})
			return err
		},
		"memory store": func() error {
			_, err := NewMemoryConsolidator(history, nil, unexpectedClient, MemoryCurationPolicyValues{})
			return err
		},
		"memory utility model resolver": func() error {
			_, err := NewMemoryConsolidator(history, memory, nil, MemoryCurationPolicyValues{})
			return err
		},
		"skill conversation reader": func() error {
			_, err := NewSkillProposalMiner(nil, proposals, nil, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill proposal submitter": func() error {
			_, err := NewSkillProposalMiner(history, nil, nil, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill typed-nil optional source": func() error {
			_, err := NewSkillProposalMiner(history, proposals, typedNilSource, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill utility model resolver": func() error {
			_, err := NewSkillProposalMiner(history, proposals, nil, nil, SkillMiningPolicyValues{})
			return err
		},
		"skill archive curator": func() error {
			_, err := NewIdleSkillArchiver(nil, SkillArchivePolicyValues{})
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
	if _, err := NewIdleSkillArchiver(skills, SkillArchivePolicyValues{}); err != nil {
		t.Fatalf("valid construction failed: %v", err)
	}
}
