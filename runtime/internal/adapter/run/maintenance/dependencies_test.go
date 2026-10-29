package maintenance

import (
	"testing"
)

func TestMaintenanceConstructorsRejectMissingDependencies(t *testing.T) {
	history := newCompactionTestStore()
	var typedNilHistory *compactionTestStore

	tests := map[string]func() error{
		"compactor typed-nil conversation store": func() error {
			_, err := NewCompactor(typedNilHistory, unexpectedClient, nil, CompactionPolicyValues{})
			return err
		},
		"compactor utility model resolver": func() error {
			_, err := NewCompactor(history, nil, nil, CompactionPolicyValues{})
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
}
