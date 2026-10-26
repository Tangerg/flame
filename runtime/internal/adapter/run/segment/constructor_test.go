package segment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
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

type unexpectedCompactions struct{}

func (unexpectedCompactions) ApplyCompaction(context.Context, runs.ConversationCompactionPlan) error {
	return errors.New("unexpected conversation compaction")
}

type inertContextState struct{}

func (inertContextState) ForgetSessionContext(string) {}

func mustNewEffects(cfg Config) *Effects {
	if cfg.Compactions == nil {
		cfg.Compactions = unexpectedCompactions{}
	}
	if cfg.ContextState == nil {
		cfg.ContextState = inertContextState{}
	}
	if lo.IsNil(cfg.ExecutionTrees) {
		cfg.ExecutionTrees = unusedExecutionTrees{}
	}
	if lo.IsNil(cfg.CommandAcceptances) {
		cfg.CommandAcceptances = inertCommandAcceptances{}
	}
	if lo.IsNil(cfg.ActorAuthority) {
		cfg.ActorAuthority = inertActorAuthority{}
	}
	if lo.IsNil(cfg.GoalRuns) {
		cfg.GoalRuns = inertGoalRuns{}
	}
	if lo.IsNil(cfg.ToolResults) {
		cfg.ToolResults = inertToolResults{}
	}
	if lo.IsNil(cfg.RememberedRules) {
		cfg.RememberedRules = inertRememberedRules{}
	}
	if lo.IsNil(cfg.AcceptedContinuations) {
		cfg.AcceptedContinuations = inertAcceptedContinuations{}
	}
	if lo.IsNil(cfg.Interrupts) {
		cfg.Interrupts = &fakeInterrupts{}
	}
	if lo.IsNil(cfg.Sessions) {
		cfg.Sessions = &fakeSession{}
	}
	if lo.IsNil(cfg.Transcript) {
		cfg.Transcript = &fakeTranscript{}
	}
	items := inertItems{}
	if lo.IsNil(cfg.ItemReplacer) {
		if replacer, ok := cfg.Transcript.(ItemReplacer); ok {
			cfg.ItemReplacer = replacer
		} else {
			cfg.ItemReplacer = items
		}
	}
	if lo.IsNil(cfg.ModelInvocations) {
		cfg.ModelInvocations = inertModelInvocations{}
	}
	if lo.IsNil(cfg.ToolInvocations) {
		cfg.ToolInvocations = inertToolInvocations{}
	}
	if lo.IsNil(cfg.Conversation) {
		cfg.Conversation = &fakeStores{}
	}
	if lo.IsNil(cfg.State) {
		cfg.State = &fakeRunState{}
	}
	if lo.IsNil(cfg.RunProgress) {
		if progress, ok := cfg.State.(RunProgressWriter); ok {
			cfg.RunProgress = progress
		} else {
			cfg.RunProgress = &fakeRunState{}
		}
	}
	if lo.IsNil(cfg.ExecutorCheckpoints) {
		cfg.ExecutorCheckpoints = &recordingExecutorCheckpointStore{}
	}
	if lo.IsNil(cfg.ChildRunStarts) {
		cfg.ChildRunStarts = &fakeChildRunStarts{}
	}
	if lo.IsNil(cfg.Tx) {
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

func (inertModelInvocations) StartModelInvocation(context.Context, string, string, string, string, time.Time, run.ModelCallPurpose) error {
	return nil
}

func (inertModelInvocations) CompleteModelInvocation(context.Context, string, string, string, string, time.Time, time.Time, *int64, *chat.Usage) error {
	return nil
}

func (inertModelInvocations) FailModelInvocation(context.Context, string, string, string, string, time.Time, time.Time, *int64, *chat.Usage, *chat.Response) error {
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
type inertCommandAcceptances struct{}

func (inertCommandAcceptances) Save(context.Context, runs.CommandAcceptance) error { return nil }
func (inertCommandAcceptances) Lookup(context.Context, string) (runs.CommandAcceptanceRecord, error) {
	return runs.CommandAcceptanceRecord{}, nil
}

type inertActorAuthority struct{}

func (inertActorAuthority) Admit(context.Context, operation.Actor) error { return nil }

type inertGoalRuns struct{}

func (inertGoalRuns) RecordRun(context.Context, run.Run) error { return nil }

type inertToolResults struct{}

func (inertToolResults) Discard(context.Context, string, transcript.ToolResultRef) error { return nil }

type inertRememberedRules struct{}

func (inertRememberedRules) Put(context.Context, approval.Rule) error { return nil }

type inertAcceptedContinuations struct{}

func (inertAcceptedContinuations) Put(context.Context, runs.AcceptedContinuation) error { return nil }
func (inertAcceptedContinuations) Delete(context.Context, string) error                 { return nil }

type unusedExecutionTrees struct{}

func (unusedExecutionTrees) SaveExecutionTree(context.Context, runs.ExecutionTreeUpdate) error {
	return errors.New("unexpected execution tree write")
}
func (unusedExecutionTrees) LoadExecutionTree(context.Context, string, string) (runs.ExecutionTreeHead, bool, error) {
	return runs.ExecutionTreeHead{}, false, errors.New("unexpected execution tree read")
}
func (unusedExecutionTrees) ExecutionResultCommitted(context.Context, string, runs.ResultPublication) (bool, error) {
	return false, errors.New("unexpected result read")
}
