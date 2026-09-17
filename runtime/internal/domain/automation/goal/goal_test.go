package goal

import (
	"errors"
	"fmt"
	"math"
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
	if err := value.ValidateSnapshot(); err != nil {
		t.Fatalf("ValidateSnapshot: %v", err)
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
		{name: "selection missing", sessionID: "ses", objective: "obj", incarnation: "inc", createdAt: now},
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

func TestCurrentValidatesExactSessionIdentity(t *testing.T) {
	unwritten, err := Unwritten("ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := unwritten.ValidateFor("ses_1"); err != nil {
		t.Fatalf("ValidateFor exact unwritten Current: %v", err)
	}
	if err := unwritten.ValidateFor("ses_2"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateFor mismatched Current error = %v, want ErrInvalid", err)
	}
	if err := (Current{}).ValidateFor("ses_1"); err == nil {
		t.Fatal("ValidateFor accepted invalid Current")
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

func TestRecordRunOwnsAccountingAndDerivedLifecycle(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	record := RunRecord{
		SessionID: "ses_1", IncarnationID: "inc_1", RunID: "run_1",
		Outcome: run.OutcomeCompleted, Cost: goalTestCost(t, 0.25), Steps: 2, CompletedAt: now.Add(time.Second),
	}
	blocked, err := active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status() != StatusActive || !blocked.Reason().IsNone() ||
		blocked.Used() != (Usage{Runs: 1, Cost: goalTestCost(t, 0.25), Steps: 2}) || blocked.Revision() != 2 {
		t.Fatalf("blocked = %+v", blocked.Snapshot())
	}

	failedGoal := testGoalAt(t, run.Capabilities{}, now)
	record.Outcome, record.RunID = run.OutcomeFailed, "run_2"
	paused, err := failedGoal.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status() != StatusPaused || paused.Reason().Code() != ReasonRunNotCompleted || paused.Reason().Detail() != string(run.OutcomeFailed) {
		t.Fatalf("failed Run state = %+v", paused.Snapshot())
	}
}

func TestRecordRunContinuesWithUnavailablePricing(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	record := RunRecord{
		SessionID: "ses_1", IncarnationID: "inc_1", RunID: "run_1",
		Outcome: run.OutcomeCompleted, Steps: 2, CompletedAt: now.Add(time.Second),
	}

	active := testGoalAt(t, run.Capabilities{}, now)
	blocked, err := active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status() != StatusActive || !blocked.Reason().IsNone() ||
		blocked.Used() != (Usage{Runs: 1, Steps: 2}) {
		t.Fatalf("unpriced Run result = %+v", blocked.Snapshot())
	}

	active = testGoalAt(t, run.Capabilities{}, now)
	record.RunID = "run_2"
	record.Cost = goalTestCost(t, 0)
	continued, err := active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Status() != StatusActive || continued.Used() != (Usage{Runs: 1, Cost: goalTestCost(t, 0), Steps: 2}) {
		t.Fatalf("priced-zero Run result = %+v", continued.Snapshot())
	}

	active = testGoalAt(t, run.Capabilities{}, now)
	record.RunID, record.Cost = "run_3", goalTestCost(t, 0.25)
	continued, err = active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	record.RunID, record.Cost, record.CompletedAt = "run_4", accounting.Cost{}, now.Add(2*time.Second)
	blocked, err = continued.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, priced := blocked.Used().Cost.USD(); blocked.Status() != StatusActive ||
		!blocked.Reason().IsNone() || priced ||
		blocked.Used().Runs != 2 || blocked.Used().Steps != 4 {
		t.Fatalf("mixed-price Run result = %+v", blocked.Snapshot())
	}
}

func TestRecordRunPreservesPriorModelReport(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	blocked, err := active.Block(ReasonBlockedByModel, "need a credential", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	accounted, err := blocked.RecordRun(RunRecord{
		SessionID: "ses_1", IncarnationID: "inc_1", RunID: "run_1",
		Outcome: run.OutcomeCompleted, Cost: goalTestCost(t, 0.25), Steps: 2, CompletedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if accounted.Status() != StatusBlocked || accounted.Reason() != blocked.Reason() || accounted.Used().Runs != 1 {
		t.Fatalf("accounted = %+v", accounted.Snapshot())
	}
}

func TestRecordRunRejectsForeignIdentityAndOverflow(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, run.Capabilities{}, now)
	record := RunRecord{
		SessionID: "other", IncarnationID: "inc_1", RunID: "run_1",
		Outcome: run.OutcomeCompleted, CompletedAt: now,
	}
	if _, err := active.RecordRun(record); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("foreign Run error = %v", err)
	}

	overflowSnapshot := active.Snapshot()
	overflowSnapshot.Used.Runs = math.MaxInt
	overflow, err := Restore(overflowSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	record.SessionID = "ses_1"
	if _, err := overflow.RecordRun(record); !errors.Is(err, ErrInvalid) && err == nil {
		t.Fatalf("overflow error = %v", err)
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

// TestRunRecordDescribesItsTerminalRun pins the rule three write-sets used to
// spell out for themselves: a Goal charge is exactly the terminal Run it names.
// Every field is a fact the record copied, so changing any one of them means
// the charge no longer describes the Run it is charged for.
func TestRunRecordDescribesItsTerminalRun(t *testing.T) {
	completed := run.OutcomeCompleted
	usd := 1.25
	metrics, err := run.NewMetrics(&accounting.Usage{Total: accounting.Totals{CostUSD: &usd}}, 3, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(1, 0).UTC()
	finishedAt := time.Unix(2, 0).UTC()
	selection, err := modelref.New("provider", "model")
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := run.Restore(run.Snapshot{
		SessionID: "session_1", ID: "run_1", ModelSelection: selection,
		GoalIncarnationID: "incarnation_1", State: run.Completed, Outcome: &completed,
		Metrics: metrics, CreatedAt: createdAt, FinishedAt: finishedAt, UpdatedAt: finishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	cost, err := terminal.Metrics().Cost()
	if err != nil {
		t.Fatal(err)
	}
	matching := RunRecord{
		SessionID: "session_1", IncarnationID: "incarnation_1", RunID: "run_1",
		Outcome: completed, Cost: cost, Steps: 3, CompletedAt: finishedAt,
	}
	if err := matching.Describes(terminal); err != nil {
		t.Fatalf("a record copied from the Run does not describe it: %v", err)
	}

	otherCost, err := accounting.NewCost(2.50)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		differ func(*RunRecord)
	}{
		{name: "session", differ: func(r *RunRecord) { r.SessionID = "session_other" }},
		{name: "incarnation", differ: func(r *RunRecord) { r.IncarnationID = "incarnation_other" }},
		{name: "run", differ: func(r *RunRecord) { r.RunID = "run_other" }},
		{name: "outcome", differ: func(r *RunRecord) { r.Outcome = run.OutcomeCanceled }},
		{name: "cost", differ: func(r *RunRecord) { r.Cost = otherCost }},
		{name: "steps", differ: func(r *RunRecord) { r.Steps = 4 }},
		{name: "completion time", differ: func(r *RunRecord) { r.CompletedAt = finishedAt.Add(time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := matching
			test.differ(&record)
			if err := record.Describes(terminal); err == nil {
				t.Fatalf("a record whose %s differs still describes the Run", test.name)
			}
		})
	}

	running, err := run.Restore(run.Snapshot{
		SessionID: "session_1", ID: "run_1", ModelSelection: selection,
		GoalIncarnationID: "incarnation_1", State: run.Running, ActiveSegmentID: "segment_1",
		Metrics: metrics, CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := matching.Describes(running); err == nil {
		t.Fatal("a charge described a Run that has not finished")
	}

	// Whether a charge belongs at all is the other half, and every writer that
	// files one asks it before asking whether the charge matches.
	if err := ValidateCharge(terminal, &matching); err != nil {
		t.Fatalf("a Goal-owned Run with its own charge was refused: %v", err)
	}
	if err := ValidateCharge(terminal, nil); err == nil {
		t.Fatal("a Goal-owned Run with no charge was accepted")
	}
	outsideSnapshot := terminal.Snapshot()
	outsideSnapshot.GoalIncarnationID = ""
	outside, err := run.Restore(outsideSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCharge(outside, nil); err != nil {
		t.Fatalf("a Run outside every Goal was refused for carrying no charge: %v", err)
	}
	if err := ValidateCharge(outside, &matching); err == nil {
		t.Fatal("a Run outside every Goal accepted a charge")
	}
}

func TestGoalKeepsRunningWithLargeAccumulatedUsageAcrossRestore(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	value := testGoalAt(t, run.Capabilities{}, now)
	for index := range 1000 {
		var err error
		value, err = value.RecordRun(RunRecord{
			SessionID: value.SessionID(), IncarnationID: value.IncarnationID(), RunID: fmt.Sprintf("run_%d", index),
			Outcome: run.OutcomeCompleted, Cost: goalTestCost(t, 100), Steps: 10000,
			CompletedAt: now.Add(time.Duration(index+1) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		value, err = Restore(value.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		if value.Status() != StatusActive || !value.Reason().IsNone() {
			t.Fatalf("stopped after %d Runs: %+v", index+1, value.Snapshot())
		}
	}
	cost, available := value.Used().Cost.USD()
	if value.Used().Runs != 1000 || value.Used().Steps != 10000000 || !available || cost != 100000 {
		t.Fatalf("usage lost: %+v", value.Used())
	}
	paused, err := value.Pause(ReasonStoppedByUser, "", value.UpdatedAt())
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := paused.Resume(paused.UpdatedAt())
	if err != nil || resumed.Status() != StatusActive || resumed.Used() != value.Used() {
		t.Fatalf("resume = %+v, %v", resumed.Snapshot(), err)
	}
}
