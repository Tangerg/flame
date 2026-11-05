package goal

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	framevalue "github.com/Tangerg/frame/value"

	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func goalTestCost(t *testing.T, usd float64) accounting.Cost {
	t.Helper()
	cost, err := accounting.NewCost(usd)
	if err != nil {
		t.Fatalf("NewCost(%g): %v", usd, err)
	}
	return cost
}

func TestSnapshotRejectsIncompleteIdentityPolicyAndTime(t *testing.T) {
	selection := testSelection(t)
	now := time.Unix(1, 0).UTC()
	tests := []struct {
		name        string
		sessionID   string
		objective   string
		selection   modelchoice.Selection
		incarnation string
		createdAt   time.Time
	}{
		{name: "session missing", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session whitespace", sessionID: " ses ", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session interior whitespace", sessionID: "ses_ one", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session non-printing", sessionID: "ses_\u200bhidden", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "session oversized", sessionID: strings.Repeat("界", identity.MaximumResourceCharacters+1), objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
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
			if _, err := newFixture(test.sessionID, test.objective, test.selection, run.Capabilities{}, test.incarnation, test.createdAt); err == nil {
				t.Fatal("New accepted invalid input")
			}
		})
	}
}

func TestGoalOwnsCapabilityStorage(t *testing.T) {
	input := run.Capabilities{InterruptTypes: []run.InterruptType{run.InterruptQuestion, run.InterruptApproval}}
	value := testGoal(t, input)
	input.InterruptTypes[0] = run.InterruptApproval

	read := value.Capabilities()
	read.InterruptTypes[0] = run.InterruptQuestion
	if !value.Capabilities().Equal(run.Capabilities{InterruptTypes: []run.InterruptType{run.InterruptApproval, run.InterruptQuestion}}) {
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
			s.ReasonExplanation = "blocked"
		}},
		{name: "blocked without model detail", mutate: func(s *Snapshot) { s.Status, s.ReasonCode = StatusBlocked, ReasonBlockedByModel }},
		{name: "noncanonical capabilities", mutate: func(s *Snapshot) {
			s.Capabilities.InterruptTypes = []run.InterruptType{run.InterruptQuestion, run.InterruptApproval}
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
	paused, err := publishedFixture(value, value.UpdatedAt())
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

func TestUsageOfFoldsTheIncarnationsTerminalRuns(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	priced := goalTestRun(t, "run_1", "inc_1", run.OutcomeCompleted, new(0.25), 2, now)
	zero := goalTestRun(t, "run_2", "inc_1", run.OutcomeCanceled, new(0.0), 3, now)
	unpriced := goalTestRun(t, "run_3", "inc_1", run.OutcomeCompleted, nil, 4, now)

	if used, err := UsageOf("ses_1", "inc_1", nil); err != nil || used != (Usage{}) {
		t.Fatalf("empty usage = %+v, %v", used, err)
	}
	used, err := UsageOf("ses_1", "inc_1", []run.Run{priced, zero})
	if err != nil || used != (Usage{Runs: 2, Cost: goalTestCost(t, 0.25), Steps: 5}) {
		t.Fatalf("priced usage = %+v, %v", used, err)
	}
	used, err = UsageOf("ses_1", "inc_1", []run.Run{priced, unpriced})
	if _, available := used.Cost.USD(); err != nil || available || used.Runs != 2 || used.Steps != 6 {
		t.Fatalf("mixed-price usage = %+v, %v", used, err)
	}
	if _, err := UsageOf("ses_1", "inc_other", []run.Run{priced}); !errors.Is(err, errRunIdentityConflict) {
		t.Fatalf("foreign incarnation error = %v", err)
	}
	if _, err := UsageOf("ses_1", "inc_1", []run.Run{goalTestRunningRun(t, "inc_1", now)}); err == nil {
		t.Fatal("usage folded a Run that has not finished")
	}

	many := make([]run.Run, 1000)
	for index := range many {
		many[index] = goalTestRun(t, fmt.Sprintf("run_%d", index), "inc_1", run.OutcomeCompleted, new(100.0), 10000, now)
	}
	used, err = UsageOf("ses_1", "inc_1", many)
	cost, available := used.Cost.USD()
	if err != nil || used.Runs != 1000 || used.Steps != 10000000 || !available || cost != 100000 {
		t.Fatalf("large usage = %+v, %v", used, err)
	}
}

func TestUsageOfChargesDescendantsWithoutCountingAnotherGoalRun(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	root := goalTestRun(t, "run_root", "inc_1", run.OutcomeCompleted, new(0.25), 2, now)
	childSnapshot := goalTestRun(t, "run_child", "", run.OutcomeCanceled, new(0.5), 3, now).Snapshot()
	childSnapshot.Lineage = testsupport.ChildLineage("itm_delegate", root.ID(), root.ID())
	child, err := run.Restore(childSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	used, err := UsageOf("ses_1", "inc_1", []run.Run{child, root})
	if err != nil || used != (Usage{Runs: 1, Cost: goalTestCost(t, 0.75), Steps: 5}) {
		t.Fatalf("delegated usage = %+v, %v", used, err)
	}
	if _, err := UsageOf("ses_1", "inc_1", []run.Run{child}); !errors.Is(err, errRunIdentityConflict) {
		t.Fatalf("missing root = %v", err)
	}
	if _, err := UsageOf("ses_1", "inc_1", []run.Run{root, child, child}); !errors.Is(err, errRunIdentityConflict) {
		t.Fatalf("duplicate Run = %v", err)
	}
	childSnapshot.SessionID = "ses_other"
	foreign, err := run.Restore(childSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UsageOf("ses_1", "inc_1", []run.Run{root, foreign}); !errors.Is(err, errRunIdentityConflict) {
		t.Fatalf("foreign Session = %v", err)
	}
	childSnapshot.SessionID = root.SessionID()
	childSnapshot.Metrics, err = run.NewMetrics(nil, 3, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	unpriced, err := run.Restore(childSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	used, err = UsageOf("ses_1", "inc_1", []run.Run{root, unpriced})
	if _, available := used.Cost.USD(); err != nil || available || used.Runs != 1 || used.Steps != 5 {
		t.Fatalf("unpriced descendant = %+v, %v", used, err)
	}
}

func testSelection(t *testing.T) modelchoice.Selection {
	t.Helper()
	selection, err := modelchoice.New("provider", "model")
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
	value, err := newFixture("ses_1", "objective", testSelection(t), capabilities, "inc_1", now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testGoalFor(t *testing.T, sessionID, incarnationID string) Goal {
	t.Helper()
	value, err := newFixture(sessionID, "objective", testSelection(t), run.Capabilities{}, incarnationID, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func goalTestRun(t *testing.T, runID, incarnationID string, outcome run.Outcome, costUSD *float64, steps int, finishedAt time.Time) run.Run {
	t.Helper()
	var usage *accounting.Usage
	if costUSD != nil {
		cost, err := accounting.CostFromOptional(costUSD)
		if err != nil {
			t.Fatal(err)
		}
		usage = &accounting.Usage{Total: accounting.Totals{Cost: cost}}
	}
	metrics, err := run.NewMetrics(usage, steps, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(1, 0).UTC()
	value, err := run.Restore(run.Snapshot{
		SessionID: "ses_1", ID: runID, ModelSelection: testSelection(t),
		GoalIncarnationID: incarnationID, State: run.Finished, Ending: testsupport.EndingFor(outcome),
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

func pausedByOutcome(reason Reason, want run.Outcome) bool {
	outcome, ok := reason.RunOutcome()
	return ok && outcome == want
}

func newFixture(sessionID, objective string, selection modelchoice.Selection, capabilities run.Capabilities, incarnation string, now time.Time) (Goal, error) {
	return Restore(Snapshot{Author: extension.ContributionSource{InstallationID: "eeb329cd-c7ce-40c9-bd90-6821fef06d30", Release: framevalue.Hash([]byte("unit")).String(), Contribution: "test"}, SessionID: sessionID, Objective: objective, Status: StatusActive, ModelSelection: selection, Capabilities: capabilities.Normalized(), IncarnationID: incarnation, Revision: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC()})
}
func publishedFixture(value Goal, now time.Time) (Goal, error) {
	snapshot := value.Snapshot()
	return snapshot.Publish(snapshot.WorkflowValue(), snapshot.Author, snapshot.IncarnationID, now)
}
