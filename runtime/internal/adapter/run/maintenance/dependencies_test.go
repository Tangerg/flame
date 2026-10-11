package maintenance

import (
	"testing"
)

func TestMaintenanceConstructorsRejectMissingDependencies(t *testing.T) {
	history := newCompactionTestStore()
	proposals := &fakeProposalSubmitter{}
	skills := &fakeIdleSkillArchiver{}
	var typedNilHistory *compactionTestStore
	var typedNilSource *fakeSkillSource

	tests := map[string]func() error{
		"compactor typed-nil conversation store": func() error {
			_, err := NewCompactor(typedNilHistory, unexpectedClient, nil, CompactionPolicyValues{}, new(recordingSessionContextInvalidator))
			return err
		},
		"compactor utility model resolver": func() error {
			_, err := NewCompactor(history, nil, nil, CompactionPolicyValues{}, new(recordingSessionContextInvalidator))
			return err
		},
		"compactor context invalidator": func() error {
			_, err := NewCompactor(history, unexpectedClient, nil, CompactionPolicyValues{}, nil)
			return err
		},
		"compactor typed-nil context invalidator": func() error {
			_, err := NewCompactor(history, unexpectedClient, nil, CompactionPolicyValues{}, (*recordingSessionContextInvalidator)(nil))
			return err
		},
		"skill conversation reader": func() error {
			_, err := NewSkillProposalMiner(nil, proposals, fakeSkillSource{}, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill proposal submitter": func() error {
			_, err := NewSkillProposalMiner(history, nil, fakeSkillSource{}, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill source": func() error {
			_, err := NewSkillProposalMiner(history, proposals, nil, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill typed-nil source": func() error {
			_, err := NewSkillProposalMiner(history, proposals, typedNilSource, unexpectedClient, SkillMiningPolicyValues{})
			return err
		},
		"skill utility model resolver": func() error {
			_, err := NewSkillProposalMiner(history, proposals, fakeSkillSource{}, nil, SkillMiningPolicyValues{})
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
