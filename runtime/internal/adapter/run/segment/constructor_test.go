package segment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
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

// TestNewFinalizerRejectsPartialTitleMaintenance pins what Finish and title
// then rely on: the three title ports arrive together or not at all, so neither
// re-checks any of them.
func TestNewFinalizerRejectsPartialTitleMaintenance(t *testing.T) {
	stores := &fakeStores{session: &fakeSession{}}
	complete := TitleMaintenance{Sessions: stores.session, Generator: stores, Tasks: inlineTaskLauncher{}}
	if _, err := NewFinalizer(FinalizerConfig{Titles: &complete}); err != nil {
		t.Fatalf("complete title maintenance: %v", err)
	}
	for _, test := range []struct {
		name string
		want string
		drop func(*TitleMaintenance)
	}{
		{name: "sessions", want: "session titles", drop: func(m *TitleMaintenance) { m.Sessions = nil }},
		{name: "generator", want: "title generator", drop: func(m *TitleMaintenance) { m.Generator = nil }},
		{name: "tasks", want: "task launcher", drop: func(m *TitleMaintenance) { m.Tasks = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			partial := complete
			test.drop(&partial)
			_, err := NewFinalizer(FinalizerConfig{Titles: &partial})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewFinalizer error = %v, want one naming %q", err, test.want)
			}
		})
	}
}

func mustNewEffects(cfg Config) *Effects {
	if dependency.Missing(cfg.Schedules) {
		cfg.Schedules = inertSchedules{}
	}
	if dependency.Missing(cfg.GoalRuns) {
		cfg.GoalRuns = inertGoalRuns{}
	}
	if dependency.Missing(cfg.ToolResults) {
		cfg.ToolResults = inertToolResults{}
	}
	interrupts := &fakeInterrupts{}
	if dependency.Missing(cfg.Interrupts) {
		cfg.Interrupts = interrupts
	}
	if dependency.Missing(cfg.ResumeClaims) {
		if claims, ok := cfg.Interrupts.(ResumeClaimStore); ok {
			cfg.ResumeClaims = claims
		} else {
			cfg.ResumeClaims = interrupts
		}
	}
	if dependency.Missing(cfg.Sessions) {
		cfg.Sessions = &fakeSession{}
	}
	if dependency.Missing(cfg.Transcript) {
		cfg.Transcript = &fakeTranscript{}
	}
	items := inertItems{}
	if dependency.Missing(cfg.ItemReplacer) {
		if replacer, ok := cfg.Transcript.(ItemReplacer); ok {
			cfg.ItemReplacer = replacer
		} else {
			cfg.ItemReplacer = items
		}
	}
	if dependency.Missing(cfg.ToolApprovals) {
		if approvals, ok := cfg.Transcript.(ToolApprovalStore); ok {
			cfg.ToolApprovals = approvals
		} else {
			cfg.ToolApprovals = items
		}
	}
	if dependency.Missing(cfg.ModelInvocations) {
		cfg.ModelInvocations = inertModelInvocations{}
	}
	if dependency.Missing(cfg.ToolInvocations) {
		cfg.ToolInvocations = inertToolInvocations{}
	}
	if dependency.Missing(cfg.Conversation) {
		cfg.Conversation = &fakeStores{}
	}
	if dependency.Missing(cfg.State) {
		cfg.State = &fakeRunState{}
	}
	if dependency.Missing(cfg.RunProgress) {
		if progress, ok := cfg.State.(RunProgressWriter); ok {
			cfg.RunProgress = progress
		} else {
			cfg.RunProgress = &fakeRunState{}
		}
	}
	if dependency.Missing(cfg.ExecutorCheckpoints) {
		cfg.ExecutorCheckpoints = &recordingExecutorCheckpointStore{}
	}
	if dependency.Missing(cfg.ChildRunStarts) {
		cfg.ChildRunStarts = &fakeChildRunStarts{}
	}
	if dependency.Missing(cfg.Tx) {
		cfg.Tx = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
	}
	effects, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return effects
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

// The segment write-sets require every store; a test that does not exercise a
// capability supplies an inert one rather than an absent one.
type inertSchedules struct{}

func (inertSchedules) Accept(context.Context, schedule.Acceptance) error   { return nil }
func (inertSchedules) RecordRun(context.Context, schedule.RunRecord) error { return nil }

type inertGoalRuns struct{}

func (inertGoalRuns) RecordRun(context.Context, goal.RunRecord) error { return nil }

type inertToolResults struct{}

func (inertToolResults) Bind(context.Context, string, string, string, toolresult.Ref) error {
	return nil
}
func (inertToolResults) Discard(context.Context, string, toolresult.Ref) error { return nil }
