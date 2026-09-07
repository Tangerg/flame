package segment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func TestNewRejectsMalformedDependencies(t *testing.T) {
	var typedNilInterrupts *fakeInterrupts
	for _, test := range []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "empty", cfg: Config{}, want: "interrupt store"},
		{name: "typed nil", cfg: Config{Interrupts: typedNilInterrupts}, want: "interrupt store"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.cfg); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewFinalizerRejectsPartialTitleMaintenance(t *testing.T) {
	_, err := NewFinalizer(FinalizerConfig{Titles: &TitleMaintenance{}})
	if err == nil || !strings.Contains(err.Error(), "session titles") {
		t.Fatalf("NewFinalizer error = %v", err)
	}
}

func mustNewEffects(cfg Config) *Effects {
	effects, err := New(testEffectsConfig(cfg))
	if err != nil {
		panic(err)
	}
	return effects
}

func testEffectsConfig(cfg Config) Config {
	if nilDependency(cfg.Schedules) {
		cfg.Schedules = &unexpectedSchedules{}
	}
	if nilDependency(cfg.GoalRuns) {
		cfg.GoalRuns = &unexpectedGoalRuns{}
	}
	if nilDependency(cfg.ToolResults) {
		cfg.ToolResults = &fakeToolResults{}
	}
	interrupts := &fakeInterrupts{}
	if nilDependency(cfg.Interrupts) {
		cfg.Interrupts = interrupts
	}
	if nilDependency(cfg.ResumeClaims) {
		if claims, ok := cfg.Interrupts.(ResumeClaimStore); ok {
			cfg.ResumeClaims = claims
		} else {
			cfg.ResumeClaims = interrupts
		}
	}
	if nilDependency(cfg.Sessions) {
		cfg.Sessions = &fakeSession{}
	}
	if nilDependency(cfg.Transcript) {
		cfg.Transcript = &fakeTranscript{}
	}
	items := inertItems{}
	if nilDependency(cfg.ItemReplacer) {
		if replacer, ok := cfg.Transcript.(ItemReplacer); ok {
			cfg.ItemReplacer = replacer
		} else {
			cfg.ItemReplacer = items
		}
	}
	if nilDependency(cfg.ToolApprovals) {
		if approvals, ok := cfg.Transcript.(ToolApprovalStore); ok {
			cfg.ToolApprovals = approvals
		} else {
			cfg.ToolApprovals = items
		}
	}
	if nilDependency(cfg.ModelInvocations) {
		cfg.ModelInvocations = inertModelInvocations{}
	}
	if nilDependency(cfg.ToolInvocations) {
		cfg.ToolInvocations = inertToolInvocations{}
	}
	if nilDependency(cfg.Conversation) {
		cfg.Conversation = &fakeStores{}
	}
	if nilDependency(cfg.State) {
		cfg.State = &fakeRunState{}
	}
	if nilDependency(cfg.RunProgress) {
		if progress, ok := cfg.State.(RunProgressWriter); ok {
			cfg.RunProgress = progress
		} else {
			cfg.RunProgress = &fakeRunState{}
		}
	}
	if nilDependency(cfg.ExecutorCheckpoints) {
		cfg.ExecutorCheckpoints = &recordingExecutorCheckpointStore{}
	}
	if nilDependency(cfg.ChildRunStarts) {
		cfg.ChildRunStarts = &fakeChildRunStarts{}
	}
	if nilDependency(cfg.Tx) {
		cfg.Tx = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
	}
	return cfg
}

func mustNewFinalizer(cfg FinalizerConfig) *Finalizer {
	finalizer, err := NewFinalizer(cfg)
	if err != nil {
		panic(err)
	}
	return finalizer
}

type inertItems struct{}

func (inertItems) Item(context.Context, string) (transcript.Item, bool, error) {
	return transcript.Item{}, false, nil
}

func (inertItems) ReplaceItem(context.Context, transcript.Replacement) error {
	return nil
}

type inertModelInvocations struct{}

func (inertModelInvocations) StartModelInvocation(context.Context, string, string, string, string, time.Time) error {
	return nil
}
func (inertModelInvocations) CompleteModelInvocation(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertModelInvocations) FailModelInvocation(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertModelInvocations) MarkModelInvocationUnknown(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}

type inertToolInvocations struct{}

func (inertToolInvocations) StartToolInvocation(context.Context, string, string, string, string, string, time.Time) error {
	return nil
}
func (inertToolInvocations) CompleteToolInvocation(context.Context, string, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertToolInvocations) MarkToolInvocationIncomplete(context.Context, string, string, string, string, string, time.Time, time.Time) error {
	return nil
}

func TestNewRequiresDurableProductStores(t *testing.T) {
	for _, test := range []struct {
		name string
		omit func(*Config, bool)
	}{
		{"schedule store", func(cfg *Config, typedNil bool) {
			cfg.Schedules = nil
			if typedNil {
				cfg.Schedules = (*unexpectedSchedules)(nil)
			}
		}},
		{"goal run recorder", func(cfg *Config, typedNil bool) {
			cfg.GoalRuns = nil
			if typedNil {
				cfg.GoalRuns = (*unexpectedGoalRuns)(nil)
			}
		}},
		{"tool result store", func(cfg *Config, typedNil bool) {
			cfg.ToolResults = nil
			if typedNil {
				cfg.ToolResults = (*fakeToolResults)(nil)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, typedNil := range []bool{false, true} {
				cfg := testEffectsConfig(Config{})
				test.omit(&cfg, typedNil)
				if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), test.name) {
					t.Fatalf("New with typed nil %t error = %v, want %q", typedNil, err, test.name)
				}
			}
		})
	}
}

type unexpectedSchedules struct{}

func (*unexpectedSchedules) Accept(context.Context, schedule.Acceptance) error {
	return errors.New("unexpected schedule acceptance")
}

func (*unexpectedSchedules) RecordRun(context.Context, schedule.RunRecord) error {
	return errors.New("unexpected schedule run record")
}

type unexpectedGoalRuns struct{}

func (*unexpectedGoalRuns) RecordRun(context.Context, goal.RunRecord) error {
	return errors.New("unexpected goal run record")
}
