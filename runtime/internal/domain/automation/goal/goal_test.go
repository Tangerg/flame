package goal

import (
	"errors"
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
	budget := testBudget(t, BudgetLimits{MaxRuns: intPointer(3)})
	value, err := New(
		"ses_1", "finish the refactor", selection, budget,
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
		budget      Budget
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
		{name: "budget missing", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc", createdAt: now},
		{name: "incarnation missing", sessionID: "ses", objective: "obj", selection: selection, createdAt: now},
		{name: "incarnation whitespace", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc arnation", createdAt: now},
		{name: "incarnation non-printing", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc\u200barnation", createdAt: now},
		{name: "incarnation invalid UTF-8", sessionID: "ses", objective: "obj", selection: selection, incarnation: string([]byte{0xff}), createdAt: now},
		{name: "incarnation oversized", sessionID: "ses", objective: "obj", selection: selection, incarnation: strings.Repeat("界", goalref.MaximumIncarnationCharacters+1), createdAt: now},
		{name: "time missing", sessionID: "ses", objective: "obj", selection: selection, incarnation: "inc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.sessionID, test.objective, test.selection, test.budget, run.Capabilities{}, test.incarnation, test.createdAt); err == nil {
				t.Fatal("New accepted invalid input")
			}
		})
	}
}

func TestGoalCanonicalizesObjectiveCommands(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	created, err := New(
		"ses", " \n objective one \t", testSelection(t), UnlimitedBudget(),
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

func TestBudgetRequiresExplicitConstructionAndPositiveLimits(t *testing.T) {
	if err := (Budget{}).Validate(); err == nil {
		t.Fatal("zero Budget was accepted as an implicit unlimited policy")
	}
	if err := (Budget{initialized: true, maxRuns: -1}).Validate(); err == nil {
		t.Fatal("corrupt negative Budget was accepted")
	}
	unlimited := UnlimitedBudget()
	if err := unlimited.Validate(); err != nil || !unlimited.Unlimited() {
		t.Fatalf("UnlimitedBudget = %+v, error %v", unlimited, err)
	}
	for _, test := range []struct {
		name   string
		limits BudgetLimits
	}{
		{name: "empty"},
		{name: "zero runs", limits: BudgetLimits{MaxRuns: intPointer(0)}},
		{name: "zero cost", limits: BudgetLimits{MaxCostUSD: floatPointer(0)}},
		{name: "zero steps", limits: BudgetLimits{MaxSteps: intPointer(0)}},
		{name: "nan cost", limits: BudgetLimits{MaxCostUSD: floatPointer(math.NaN())}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewBudget(test.limits); err == nil {
				t.Fatal("NewBudget accepted a non-positive or absent limit")
			}
		})
	}
	limited := testBudget(t, BudgetLimits{
		MaxRuns: intPointer(3), MaxCostUSD: floatPointer(1.5), MaxSteps: intPointer(20),
	})
	if value, ok := limited.MaxRuns(); !ok || value != 3 {
		t.Fatalf("MaxRuns = (%d, %t), want (3, true)", value, ok)
	}
	if _, ok := limited.MaxCostUSD(); !ok {
		t.Fatal("MaxCostUSD lost its explicit limit")
	}
	if _, ok := limited.MaxSteps(); !ok {
		t.Fatal("MaxSteps lost its explicit limit")
	}
}

func TestGoalOwnsCapabilityStorage(t *testing.T) {
	input := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question, interrupt.Approval}}
	value := testGoal(t, UnlimitedBudget(), input)
	input.InterruptKinds[0] = interrupt.Approval

	read := value.Capabilities()
	read.InterruptKinds[0] = interrupt.Question
	if !value.Capabilities().Equal(run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question}}) {
		t.Fatalf("Goal shares capability storage: %v", value.Capabilities())
	}
}

func TestRestoreRejectsImpossibleCommittedState(t *testing.T) {
	base := testGoal(t, UnlimitedBudget(), run.Capabilities{}).Snapshot()
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
		{name: "active exhausted budget", mutate: func(s *Snapshot) {
			s.Budget, s.Used.Runs = testBudget(t, BudgetLimits{MaxRuns: intPointer(1)}), 1
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

	value := testGoal(t, UnlimitedBudget(), run.Capabilities{})
	current, err := CurrentOf(value)
	if err != nil {
		t.Fatal(err)
	}
	owned, exists := current.Goal()
	if !exists || current.Version().IsUnwritten() || owned.Version() != current.Version() {
		t.Fatal("committed Current lost Goal identity")
	}

	fresh := testGoalFor(t, "ses_1", "inc_fresh", UnlimitedBudget())
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

func TestLifecycleTransitionsAreImmutableAndMonotonic(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, UnlimitedBudget(), run.Capabilities{}, now)
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
	active := testGoalAt(t, UnlimitedBudget(), run.Capabilities{}, now)
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
	active := testGoalAt(t, testBudget(t, BudgetLimits{MaxRuns: intPointer(1)}), run.Capabilities{}, now)
	record := RunRecord{
		SessionID: "ses_1", IncarnationID: "inc_1", RunID: "run_1",
		Outcome: run.OutcomeCompleted, Cost: goalTestCost(t, 0.25), Steps: 2, CompletedAt: now.Add(time.Second),
	}
	blocked, err := active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status() != StatusBlocked || blocked.Reason().Code() != ReasonRunBudgetReached ||
		blocked.Used() != (Usage{Runs: 1, Cost: goalTestCost(t, 0.25), Steps: 2}) || blocked.Revision() != 2 {
		t.Fatalf("blocked = %+v", blocked.Snapshot())
	}
	if _, err := blocked.Resume(now.Add(2 * time.Second)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("Resume error = %v, want ErrBudgetExhausted", err)
	}

	failedGoal := testGoalAt(t, UnlimitedBudget(), run.Capabilities{}, now)
	record.Outcome, record.RunID = run.OutcomeFailed, "run_2"
	paused, err := failedGoal.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status() != StatusPaused || paused.Reason().Code() != ReasonRunNotCompleted || paused.Reason().Detail() != run.OutcomeFailed.String() {
		t.Fatalf("failed Run state = %+v", paused.Snapshot())
	}
}

func TestRecordRunRequiresPricingForCostLimitedGoal(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	budget := testBudget(t, BudgetLimits{MaxCostUSD: floatPointer(1)})
	record := RunRecord{
		SessionID: "ses_1", IncarnationID: "inc_1", RunID: "run_1",
		Outcome: run.OutcomeCompleted, Steps: 2, CompletedAt: now.Add(time.Second),
	}

	active := testGoalAt(t, budget, run.Capabilities{}, now)
	blocked, err := active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status() != StatusBlocked || blocked.Reason().Code() != ReasonPricingUnavailable ||
		blocked.Used() != (Usage{Runs: 1, Steps: 2}) {
		t.Fatalf("unpriced Run result = %+v", blocked.Snapshot())
	}
	if _, err := blocked.Resume(now.Add(2 * time.Second)); !errors.Is(err, ErrPricingUnavailable) {
		t.Fatalf("Resume unpriced Goal = %v, want ErrPricingUnavailable", err)
	}

	active = testGoalAt(t, budget, run.Capabilities{}, now)
	record.RunID = "run_2"
	record.Cost = goalTestCost(t, 0)
	continued, err := active.RecordRun(record)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Status() != StatusActive || continued.Used() != (Usage{Runs: 1, Cost: goalTestCost(t, 0), Steps: 2}) {
		t.Fatalf("priced-zero Run result = %+v", continued.Snapshot())
	}

	active = testGoalAt(t, budget, run.Capabilities{}, now)
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
	if _, priced := blocked.Used().Cost.USD(); blocked.Status() != StatusBlocked ||
		blocked.Reason().Code() != ReasonPricingUnavailable || priced ||
		blocked.Used().Runs != 2 || blocked.Used().Steps != 4 {
		t.Fatalf("mixed-price Run result = %+v", blocked.Snapshot())
	}
}

func TestRecordRunPreservesPriorModelReport(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	active := testGoalAt(t, testBudget(t, BudgetLimits{MaxRuns: intPointer(1)}), run.Capabilities{}, now)
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
	active := testGoalAt(t, UnlimitedBudget(), run.Capabilities{}, now)
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
	active := testGoalAt(t, testBudget(t, BudgetLimits{MaxRuns: intPointer(4)}), run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}, now)
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
	if revised.Status() != StatusPaused || revised.Reason() != paused.Reason() || revised.Budget() != active.Budget() ||
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

func TestBudgetExceeded(t *testing.T) {
	tests := []struct {
		name     string
		budget   Budget
		used     Usage
		limit    BudgetLimit
		exceeded bool
	}{
		{name: "unbounded", budget: UnlimitedBudget(), used: Usage{Runs: 100, Cost: goalTestCost(t, 999), Steps: 999}},
		{name: "under", budget: testBudget(t, BudgetLimits{MaxRuns: intPointer(5)}), used: Usage{Runs: 4}},
		{name: "runs", budget: testBudget(t, BudgetLimits{MaxRuns: intPointer(5)}), used: Usage{Runs: 5}, limit: BudgetLimitRuns, exceeded: true},
		{name: "cost", budget: testBudget(t, BudgetLimits{MaxCostUSD: floatPointer(1)}), used: Usage{Runs: 1, Cost: goalTestCost(t, 1)}, limit: BudgetLimitCost, exceeded: true},
		{name: "steps", budget: testBudget(t, BudgetLimits{MaxSteps: intPointer(10)}), used: Usage{Steps: 11}, limit: BudgetLimitSteps, exceeded: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limit, exceeded := test.budget.exceeded(test.used)
			if limit != test.limit || exceeded != test.exceeded {
				t.Fatalf("Exceeded = (%s,%t), want (%s,%t)", limit, exceeded, test.limit, test.exceeded)
			}
		})
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

func testBudget(t *testing.T, limits BudgetLimits) Budget {
	t.Helper()
	budget, err := NewBudget(limits)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func intPointer(value int) *int { return &value }

func floatPointer(value float64) *float64 { return &value }

func testGoal(t *testing.T, budget Budget, capabilities run.Capabilities) Goal {
	t.Helper()
	return testGoalAt(t, budget, capabilities, time.Unix(10, 0).UTC())
}

func testGoalAt(t *testing.T, budget Budget, capabilities run.Capabilities, now time.Time) Goal {
	t.Helper()
	value, err := New("ses_1", "objective", testSelection(t), budget, capabilities, "inc_1", now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testGoalFor(t *testing.T, sessionID, incarnationID string, budget Budget) Goal {
	t.Helper()
	value, err := New(sessionID, "objective", testSelection(t), budget, run.Capabilities{}, incarnationID, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return value
}
