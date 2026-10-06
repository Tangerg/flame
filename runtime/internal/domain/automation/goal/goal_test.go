package goal

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func goalTestCost(t *testing.T, usd float64) accounting.Cost {
	t.Helper()
	cost, err := accounting.NewCost(usd)
	if err != nil {
		t.Fatalf("NewCost(%g): %v", usd, err)
	}
	return cost
}

func TestNewBuildsCommittedActiveGoal(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.FixedZone("offset", 8*60*60))
	selection := testSelection(t)
	value, err := New(
		"ses_1", "finish the refactor", selection,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question, interrupt.Approval}},
		"inc_1", now,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if value.Status() != StatusActive || value.Revision() != firstRevision {
		t.Fatalf("new Goal = status %q revision %d", value.Status(), value.Revision())
	}
	if !value.CreatedAt().Equal(now) || value.CreatedAt().Location() != time.UTC {
		t.Fatalf("created at = %v, want canonical UTC %v", value.CreatedAt(), now)
	}
	wantCapabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question}}
	if !value.Capabilities().Equal(wantCapabilities) {
		t.Fatalf("capabilities = %v, want %v", value.Capabilities(), wantCapabilities)
	}
}

func TestNewRejectsIncompleteIdentityPolicyAndTime(t *testing.T) {
	selection := testSelection(t)
	now := time.Unix(1, 0).UTC()
	tests := []struct {
		name        string
		sessionID   string
		objective   string
		selection   modelref.Selection
		incarnation string
		createdAt   time.Time
	}{
		{name: "session missing", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session whitespace", sessionID: " ses ", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session interior whitespace", sessionID: "ses_ one", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session non-printing", sessionID: "ses_\u200bhidden", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session oversized", sessionID: strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1), objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "objective missing", sessionID: "ses", selection: selection, incarnation: "inc", createdAt: now},
		{name: "objective blank", sessionID: "ses", objective: " \t ", selection: selection, incarnation: "inc", createdAt: now},
		{name: "incarnation missing", sessionID: "ses", objective: "obj", selection: selection, createdAt: now},
		{name: "incarnation whitespace", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc arnation", createdAt: now},
		{name: "incarnation non-printing", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc\u200barnation", createdAt: now},
		{name: "incarnation invalid UTF-8", sessionID: "ses", objective: "obj", selection: selection, incarnation: string([]byte{0xff}), createdAt: now},
		{name: "incarnation oversized", sessionID: "ses", objective: "obj", selection: selection, incarnation: strings.Repeat("界", goalref.MaximumIncarnationCharacters+1), createdAt: now},
		{name: "time missing", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.sessionID, test.objective, test.selection, run.Capabilities{}, test.incarnation, test.createdAt); err == nil {
				t.Fatal("New accepted invalid input")
			}
		})
	}
}

func TestGoalCanonicalizesObjectiveCommands(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	created, err := New(
		"ses", " \n objective one \t", testSelection(t),
		run.Capabilities{}, "inc_1", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Objective() != "objective one" {
		t.Fatalf("created objective = %q", created.Objective())
	}

	revised, err := created.ReviseObjective("  objective two\n", "inc_2", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if revised.Objective() != "objective two" {
		t.Fatalf("revised objective = %q", revised.Objective())
	}
}

func TestGoalOwnsCapabilityStorage(t *testing.T) {
	input := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question, interrupt.Approval}}
	value := testGoal(t, input)
	input.InterruptKinds[0] = interrupt.Approval

	read := value.Capabilities()
	read.InterruptKinds[0] = interrupt.Question
	if !value.Capabilities().Equal(run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question}}) {
		t.Fatalf("Goal shares capability storage: %v", value.Capabilities())
	}
}

func TestRestoreRejectsImpossibleCommittedState(t *testing.T) {
	base := testGoal(t, run.Capabilities{}).Snapshot()
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{name: "revision missing", mutate: func(s *Snapshot) { s.Revision = 0 }},
		{name: "noncanonical objective", mutate: func(s *Snapshot) { s.Objective = " objective " }},
		{name: "update before create", mutate: func(s *Snapshot) { s.UpdatedAt = s.CreatedAt.Add(-time.Nanosecond) }},
		{name: "active with reason", mutate: func(s *Snapshot) { s.ReasonCode = ReasonStoppedByUser }},
		{name: "paused without reason", mutate: func(s *Snapshot) { s.Status = StatusPaused }},
		{name: "paused with blocked reason", mutate: func(s *Snapshot) {
			s.Status, s.ReasonCode = StatusPaused, ReasonBlockedByModel
			s.ReasonDetail = "blocked"
		}},
		{name: "blocked without model detail", mutate: func(s *Snapshot) { s.Status, s.ReasonCode = StatusBlocked, ReasonBlockedByModel }},
		{name: "noncanonical capabilities", mutate: func(s *Snapshot) {
			s.Capabilities.InterruptKinds = []interrupt.Kind{interrupt.Question, interrupt.Approval}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := base
			snapshot.Capabilities = base.Capabilities.Clone()
			test.mutate(&snapshot)
			if _, err := Restore(snapshot); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Restore error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCurrentAndVersionDistinguishAbsenceFromCommittedState(t *testing.T) {
	unwritten, err := Unwritten("ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := unwritten.Goal(); exists || !unwritten.Version().IsUnwritten() {
		t.Fatal("unwritten Current became committed")
	}

	value := testGoal(t, run.Capabilities{})
	current, err := CurrentOf(value)
	if err != nil {
		t.Fatal(err)
	}
	owned, exists := current.Goal()
	if !exists || current.Version().IsUnwritten() || owned.Version() != current.Version() {
		t.Fatal("committed Current lost Goal identity")
	}

	fresh := testGoalFor(t, "ses_1", "inc_fresh")
	if err := unwritten.Version().AdvancesTo(fresh); err != nil {
		t.Fatalf("unwritten advance: %v", err)
	}
	paused, err := value.Pause(ReasonStoppedByUser, "", value.UpdatedAt())
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Version().AdvancesTo(paused); err != nil {
		t.Fatalf("same-incarnation advance: %v", err)
	}
}

func TestCurrentConstructionOwnsSessionIdentity(t *testing.T) {
	if _, err := Unwritten(""); err == nil {
		t.Fatal("Unwritten accepted a missing Session")
	}
	if _, err := CurrentOf(Goal{}); err == nil {
		t.Fatal("CurrentOf accepted an unconstructed Goal")
	}
	value := testGoalAt(t, run.Capabilities{}, time.Unix(10, 0).UTC())
	current, err := CurrentOf(value)
	if err != nil {
		t.Fatal(err)
	}
	stored, found := current.Goal()
	if !found || stored.SessionID() != value.SessionID() || current.Version() != value.Version() {
		t.Fatal("Current changed the committed Goal identity")
	}
}

func TestLifecycleTransitionsAreImmutableAndMonotonic(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	paused, err := active.Pause(ReasonStoppedByUser, "", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if active.Status() != StatusActive || active.Revision() != 1 {
		t.Fatalf("Pause mutated source = %q@%d", active.Status(), active.Revision())
	}
	if paused.Status() != StatusPaused || paused.Reason().Code() != ReasonStoppedByUser || paused.Revision() != 2 {
		t.Fatalf("paused = %q/%q@%d", paused.Status(), paused.Reason().Code(), paused.Revision())
	}
	resumed, err := paused.Resume(now.Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status() != StatusActive || !resumed.Reason().IsNone() || resumed.Revision() != 3 {
		t.Fatalf("resumed = %q/%q@%d", resumed.Status(), resumed.Reason().Code(), resumed.Revision())
	}
	complete, err := resumed.Complete(now.Add(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if complete.Status() != StatusComplete || complete.Revision() != 4 {
		t.Fatalf("complete = %q@%d", complete.Status(), complete.Revision())
	}
	if _, err := complete.Resume(now.Add(4 * time.Second)); !errors.Is(err, ErrNotResumable) {
		t.Fatalf("complete Resume error = %v", err)
	}
}

func TestTransitionRejectsInvalidReasonTimeAndState(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	if _, err := active.Pause(ReasonNone, "", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Pause no reason = %v", err)
	}
	if _, err := active.Pause(ReasonBlockedByModel, "blocked", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Pause blocked reason = %v", err)
	}
	if _, err := active.Block(ReasonBlockedByModel, "", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Block no detail = %v", err)
	}
	if _, err := active.Complete(now.Add(-time.Nanosecond)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("time travel = %v", err)
	}
	paused, err := active.Pause(ReasonStoppedByUser, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := paused.Pause(ReasonStoppedByUser, "", now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("double Pause = %v", err)
	}
}

func TestRecordRunPausesAnActiveGoalAfterAnUnfinishedRun(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	completed := goalTestRun(t, "run_1", "inc_1", run.OutcomeCompleted, new(0.25), 2, now.Add(time.Second))
	unchanged, changed, err := active.RecordRun(completed)
	if err != nil {
		t.Fatal(err)
	}
	if changed || unchanged.Revision() != active.Revision() || unchanged.Status() != StatusActive {
		t.Fatalf("completed Run changed the Goal: changed=%t %+v", changed, unchanged.Snapshot())
	}

	failed := goalTestRun(t, "run_2", "inc_1", run.OutcomeCanceled, nil, 1, now.Add(time.Second))
	paused, changed, err := active.RecordRun(failed)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || paused.Status() != StatusPaused || paused.Reason().Code() != ReasonRunNotCompleted ||
		paused.Reason().Detail() != string(run.OutcomeCanceled) || paused.Revision() != active.Revision()+1 {
		t.Fatalf("failed Run state = %+v", paused.Snapshot())
	}
}

func TestRecordRunPreservesPriorModelReport(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	blocked, err := active.Block(ReasonBlockedByModel, "need a credential", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	failed := goalTestRun(t, "run_1", "inc_1", run.OutcomeCanceled, nil, 2, now.Add(2*time.Second))
	accounted, changed, err := blocked.RecordRun(failed)
	if err != nil {
		t.Fatal(err)
	}
	if changed || accounted.Status() != StatusBlocked || accounted.Reason() != blocked.Reason() {
		t.Fatalf("accounted = %+v", accounted.Snapshot())
	}
}

func TestRecordRunRejectsAForeignOrUnfinishedRun(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	foreign := goalTestRun(t, "run_1", "inc_other", run.OutcomeCompleted, nil, 0, now)
	if _, _, err := active.RecordRun(foreign); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("foreign Run error = %v", err)
	}
	if _, _, err := active.RecordRun(goalTestRunningRun(t, "inc_1", now)); err == nil {
		t.Fatal("a Run that has not finished was applied to its Goal")
	}
}

func TestUsageOfFoldsTheIncarnationsTerminalRuns(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	priced := goalTestRun(t, "run_1", "inc_1", run.OutcomeCompleted, new(0.25), 2, now)
	zero := goalTestRun(t, "run_2", "inc_1", run.OutcomeCanceled, new(0.0), 3, now)
	unpriced := goalTestRun(t, "run_3", "inc_1", run.OutcomeCompleted, nil, 4, now)

	if used, err := UsageOf("inc_1", nil); err != nil || used != (Usage{}) {
		t.Fatalf("empty usage = %+v, %v", used, err)
	}
	used, err := UsageOf("inc_1", []run.Run{priced, zero})
	if err != nil || used != (Usage{Runs: 2, Cost: goalTestCost(t, 0.25), Steps: 5}) {
		t.Fatalf("priced usage = %+v, %v", used, err)
	}
	used, err = UsageOf("inc_1", []run.Run{priced, unpriced})
	if _, available := used.Cost.USD(); err != nil || available || used.Runs != 2 || used.Steps != 6 {
		t.Fatalf("mixed-price usage = %+v, %v", used, err)
	}
	if _, err := UsageOf("inc_other", []run.Run{priced}); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("foreign incarnation error = %v", err)
	}
	if _, err := UsageOf("inc_1", []run.Run{goalTestRunningRun(t, "inc_1", now)}); err == nil {
		t.Fatal("usage folded a Run that has not finished")
	}

	many := make([]run.Run, 1000)
	for index := range many {
		many[index] = goalTestRun(t, fmt.Sprintf("run_%d", index), "inc_1", run.OutcomeCompleted, new(100.0), 10000, now)
	}
	used, err = UsageOf("inc_1", many)
	cost, available := used.Cost.USD()
	if err != nil || used.Runs != 1000 || used.Steps != 10000000 || !available || cost != 100000 {
		t.Fatalf("large usage = %+v, %v", used, err)
	}
}

func TestReviseObjectiveStartsFreshVersionAndPreservesFacts(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}, now)
	paused, err := active.Pause(ReasonStoppedByUser, "", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	revised, err := paused.ReviseObjective("second", "inc_2", now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if revised.Objective() != "second" || revised.IncarnationID() != "inc_2" || revised.Revision() != firstRevision {
		t.Fatalf("revised identity = %s/%s@%d", revised.Objective(), revised.IncarnationID(), revised.Revision())
	}
	if revised.Status() != StatusPaused || revised.Reason() != paused.Reason() ||
		!revised.CreatedAt().Equal(active.CreatedAt()) {
		t.Fatalf("revised facts = %+v", revised.Snapshot())
	}
	if err := paused.Version().AdvancesTo(revised); err != nil {
		t.Fatalf("fresh incarnation advance: %v", err)
	}

	activeRevision, err := paused.ReviseObjectiveAndResume("second", "inc_3", now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if activeRevision.Status() != StatusActive || activeRevision.Revision() != firstRevision {
		t.Fatalf("revised+resumed = %+v", activeRevision.Snapshot())
	}
}

func testSelection(t *testing.T) modelref.Selection {
	t.Helper()
	selection, err := modelref.New("provider", "model")
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func testGoal(t *testing.T, capabilities run.Capabilities) Goal {
	t.Helper()
	return testGoalAt(t, capabilities, time.Unix(10, 0).UTC())
}

func testGoalAt(t *testing.T, capabilities run.Capabilities, now time.Time) Goal {
	t.Helper()
	value, err := New("ses_1", "objective", testSelection(t), capabilities, "inc_1", now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testGoalFor(t *testing.T, sessionID, incarnationID string) Goal {
	t.Helper()
	value, err := New(sessionID, "objective", testSelection(t), run.Capabilities{}, incarnationID, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func goalTestRun(t *testing.T, runID, incarnationID string, outcome run.Outcome, costUSD *float64, steps int, finishedAt time.Time) run.Run {
	t.Helper()
	var usage *accounting.Usage
	if costUSD != nil {
		usage = &accounting.Usage{Total: accounting.Totals{CostUSD: costUSD}}
	}
	metrics, err := run.NewMetrics(usage, steps, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(1, 0).UTC()
	value, err := run.Restore(run.Snapshot{
		SessionID: "ses_1", ID: runID, ModelSelection: testSelection(t),
		GoalIncarnationID: incarnationID, State: run.State(outcome), Outcome: &outcome,
		Metrics: metrics, CreatedAt: createdAt, FinishedAt: finishedAt, UpdatedAt: finishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func goalTestRunningRun(t *testing.T, incarnationID string, now time.Time) run.Run {
	t.Helper()
	createdAt := time.Unix(1, 0).UTC()
	value, err := run.Restore(run.Snapshot{
		SessionID: "ses_1", ID: "run_running", ModelSelection: testSelection(t),
		GoalIncarnationID: incarnationID, State: run.Running, ActiveSegmentID: "segment_1",
		CreatedAt: createdAt, UpdatedAt: now, MessageMark: run.UnknownMessageMark(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
