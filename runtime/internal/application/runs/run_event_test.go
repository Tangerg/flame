package runs

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/plan"
	"github.com/Tangerg/flame/runtime/internal/domain/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/transcript"
)

func TestToolItemStartCannotDivergeFromItsDurableFact(t *testing.T) {
	arguments, err := tool.ParseArguments(`{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("ParseArguments: %v", err)
	}
	item, err := transcript.NewToolCall(transcript.ItemIdentity{
		SessionID: "session-1", RunID: "run-1", ItemID: "item-1",
		OccurredAt: time.Unix(1, 0).UTC(),
	}, transcript.ToolInvocation{Name: "read_file", Arguments: arguments}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	start, err := newToolItemStart(item)
	if err != nil {
		t.Fatalf("newToolItemStart: %v", err)
	}
	if err := start.validate(); err != nil {
		t.Fatalf("validate canonical start: %v", err)
	}

	start.ToolInvocation.Name = "other_tool"
	if err := start.validate(); err == nil {
		t.Fatal("validate accepted a presentation invocation different from the durable Item")
	}
}

func TestItemDeltaValuesRejectImpossibleStates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		construct func() error
	}{
		{name: "content", construct: func() error {
			_, err := newContentItemDelta("")
			return err
		}},
		{name: "reasoning", construct: func() error {
			_, err := newReasoningItemDelta("")
			return err
		}},
		{name: "tool arguments", construct: func() error {
			_, err := newToolArgumentsItemDelta("")
			return err
		}},
		{name: "tool output", construct: func() error {
			_, err := newToolOutputItemDelta("")
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.construct(); err == nil {
				t.Fatal("constructor accepted an empty delta")
			}
		})
	}
}

func TestProjectRejectsMalformedItemChangeBeforePublication(t *testing.T) {
	t.Parallel()

	_, err := newReducer(testReducerConfig()).project([]RunEvent{ItemChanged{ItemID: "item_1"}})
	if !errors.Is(err, errReducerInvariant) {
		t.Fatalf("project error = %v, want reducer invariant violation", err)
	}
}

func TestRunProgressRejectsImpossiblePreviews(t *testing.T) {
	t.Parallel()

	negativeStep, negativeContext := -1, int64(-1)
	invalidCost := -0.01
	for _, test := range []struct {
		name     string
		progress RunProgress
	}{
		{name: "empty"},
		{name: "negative step", progress: RunProgress{Step: &negativeStep}},
		{name: "invalid usage", progress: RunProgress{Usage: &accounting.Usage{
			Total: accounting.Totals{CostUSD: &invalidCost},
		}}},
		{name: "negative context", progress: RunProgress{ContextTokens: &negativeContext}},
		{name: "noncanonical activity", progress: RunProgress{Activity: " calling model "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.progress.validate(); err == nil {
				t.Fatalf("validate accepted %+v", test.progress)
			}
		})
	}

	zeroStep, zeroContext := 0, int64(0)
	valid := RunProgress{Step: &zeroStep, ContextTokens: &zeroContext, Activity: "Calling model"}
	if err := valid.validate(); err != nil {
		t.Fatalf("validate canonical progress: %v", err)
	}
}

func TestPlanSnapshotRejectsNonEventStatesAndOwnsItsFence(t *testing.T) {
	t.Parallel()

	now := time.Unix(7, 0).UTC()
	valid := PlanSnapshot{
		SessionID: "session-1", Revision: 1, UpdatedAt: now,
		Steps: []plan.Step{{Description: "inspect", Status: plan.StatusInProgress}},
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("validate canonical snapshot: %v", err)
	}
	for _, test := range []struct {
		name     string
		snapshot PlanSnapshot
	}{
		{name: "unwritten state", snapshot: PlanSnapshot{SessionID: "session-1"}},
		{name: "noncanonical session", snapshot: PlanSnapshot{SessionID: " session-1", Revision: 1, UpdatedAt: now}},
		{name: "missing time", snapshot: PlanSnapshot{SessionID: "session-1", Revision: 1}},
		{name: "noncanonical time", snapshot: PlanSnapshot{SessionID: "session-1", Revision: 1, UpdatedAt: time.Unix(7, 0)}},
		{name: "invalid steps", snapshot: PlanSnapshot{
			SessionID: "session-1", Revision: 1, UpdatedAt: now,
			Steps: []plan.Step{{Description: "inspect", Status: plan.Status("unknown")}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.snapshot.validate(); err == nil {
				t.Fatalf("validate accepted %+v", test.snapshot)
			}
		})
	}

	owned := valid.clone()
	valid.Steps[0].Description = "mutated"
	if got := owned.Steps[0].Description; got != "inspect" {
		t.Fatalf("cloned snapshot description = %q, want owned value", got)
	}
}

func TestProjectRejectsEveryMalformedEventVariantBeforePublication(t *testing.T) {
	t.Parallel()

	reducer := newReducer(testReducerConfig())
	for _, event := range []RunEvent{
		SegmentStarted{},
		SegmentProgressed{},
		SegmentFinished{},
		ItemStarted{},
		ItemCompleted{},
		PlanSnapshot{SessionID: reducer.cfg.SessionID},
	} {
		if _, err := reducer.project([]RunEvent{event}); !errors.Is(err, errReducerInvariant) {
			t.Fatalf("project %T error = %v, want reducer invariant violation", event, err)
		}
	}
}

func TestEmptyStreamingChunksDoNotCreateAnchors(t *testing.T) {
	t.Parallel()

	reducer := newReducer(testReducerConfig())
	for _, fact := range []ExecutionFact{MessageDelta{}, ReasoningDelta{}} {
		batch, err := reducer.reduce(fact)
		if err != nil {
			t.Fatalf("reduce %T: %v", fact, err)
		}
		if len(batch.events) != 0 {
			t.Fatalf("reduce %T published %d events for an empty chunk", fact, len(batch.events))
		}
	}
}
