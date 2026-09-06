package goals_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// memStore is an in-memory goals.Store.
type memStore struct {
	mu                sync.Mutex
	goals             map[string]goal.Goal
	changed           chan struct{}
	failSave          func(goal.Goal) error
	runs              map[string]struct{}
	afterContextCheck func() // test hook: called before the context-aware lock
}

var (
	goalTraceOnce     sync.Once
	goalTraceExporter *notifyingSpanExporter
)

func installGoalTraceCapture(t *testing.T) *notifyingSpanExporter {
	t.Helper()
	goalTraceOnce.Do(func() {
		goalTraceExporter = &notifyingSpanExporter{
			InMemoryExporter: tracetest.NewInMemoryExporter(),
		}
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(goalTraceExporter))
		otel.SetTracerProvider(tp)
	})
	goalTraceExporter.Reset()
	goalTraceExporter.exported = make(chan struct{}, 1)
	t.Cleanup(goalTraceExporter.Reset)
	return goalTraceExporter
}

type faultingGoalStore struct {
	*memStore
	gets   atomic.Int32
	failAt int32
	err    error
	failed chan struct{}
}

func (f *faultingGoalStore) Get(ctx context.Context, sessionID string) (goal.Current, error) {
	if f.gets.Add(1) == f.failAt {
		close(f.failed)
		return goal.Current{}, f.err
	}
	return f.memStore.Get(ctx, sessionID)
}

type pauseCompletionRaceStore struct {
	*memStore
	won atomic.Bool
}

type conflictingReconcileStore struct {
	goals.Store
	rejectSave  bool
	rejectClear bool
}

type boundaryGoalStore struct {
	goals.Store
	get  func(context.Context, string) (goal.Current, error)
	list func(context.Context) ([]goal.Goal, error)
}

func (b boundaryGoalStore) Get(ctx context.Context, sessionID string) (goal.Current, error) {
	if b.get != nil {
		return b.get(ctx, sessionID)
	}
	return b.Store.Get(ctx, sessionID)
}

func (b boundaryGoalStore) List(ctx context.Context) ([]goal.Goal, error) {
	if b.list != nil {
		return b.list(ctx)
	}
	return b.Store.List(ctx)
}

func (c conflictingReconcileStore) Save(
	context.Context,
	goal.Replacement,
) (bool, error) {
	if c.rejectSave {
		return false, nil
	}
	panic("unexpected Save")
}

func (c conflictingReconcileStore) ClearIf(
	context.Context,
	string,
	goal.Version,
) (bool, error) {
	if c.rejectClear {
		return false, nil
	}
	panic("unexpected ClearIf")
}

func (p *pauseCompletionRaceStore) Save(
	ctx context.Context,
	replacement goal.Replacement,
) (bool, error) {
	candidate := replacement.State()
	expected := replacement.ExpectedVersion()
	if candidate.Reason().Code() != goal.ReasonRunStartFailed || !p.won.CompareAndSwap(false, true) {
		return p.memStore.Save(ctx, replacement)
	}
	if err := p.lock(ctx); err != nil {
		return false, err
	}
	defer p.mu.Unlock()
	current, ok := p.goals[candidate.SessionID()]
	if !ok || current.Version() != expected {
		return false, nil
	}
	completed, err := current.Complete(time.Now())
	if err != nil {
		return false, err
	}
	p.goals[completed.SessionID()] = completed
	p.notifyLocked()
	return false, nil
}

func newMemStore() *memStore {
	return &memStore{
		goals:   map[string]goal.Goal{},
		runs:    map[string]struct{}{},
		changed: make(chan struct{}),
	}
}

// lock observes cancellation before and after waiting for the fake's mutex. The
// second check matters: a drive can be canceled while blocked on a concurrent
// store operation, and must not mutate state after it finally acquires the lock.
func (m *memStore) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.afterContextCheck != nil {
		m.afterContextCheck()
	}
	m.mu.Lock()
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return err
	}
	return nil
}

// The store methods honor ctx cancellation to model the production sqlite store.
// This is load-bearing for the Goal drive: a superseded straggler whose ctx was
// canceled by Stop bails at its next store operation instead of racing recovery.
func (m *memStore) Get(ctx context.Context, id string) (goal.Current, error) {
	if err := m.lock(ctx); err != nil {
		return goal.Current{}, err
	}
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok {
		return goal.Unwritten(id)
	}
	return goal.CurrentOf(g)
}
func (m *memStore) Save(ctx context.Context, replacement goal.Replacement) (bool, error) {
	if err := m.lock(ctx); err != nil {
		return false, err
	}
	defer m.mu.Unlock()
	g := replacement.State()
	expected := replacement.ExpectedVersion()
	if m.failSave != nil {
		if err := m.failSave(g); err != nil {
			m.failSave = nil
			return false, err
		}
	}
	if err := replacement.Validate(); err != nil {
		return false, err
	}
	cur, ok := m.goals[g.SessionID()]
	switch {
	case expected.IsUnwritten():
		if ok {
			return false, nil
		}
	case !ok || cur.Version() != expected:
		return false, nil
	}
	m.goals[g.SessionID()] = g
	m.notifyLocked()
	return true, nil
}

func (m *memStore) failNextStopSave(err error) {
	m.mu.Lock()
	m.failSave = func(g goal.Goal) error {
		if g.Reason().Code() == goal.ReasonStoppedByUser {
			return err
		}
		return nil
	}
	m.mu.Unlock()
}
func (m *memStore) Clear(ctx context.Context, id string) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	delete(m.goals, id)
	m.notifyLocked()
	return nil
}
func (m *memStore) ClearIf(ctx context.Context, id string, expected goal.Version) (bool, error) {
	if err := m.lock(ctx); err != nil {
		return false, err
	}
	defer m.mu.Unlock()
	cur, ok := m.goals[id]
	if !ok || cur.Version() != expected {
		return false, nil
	}
	delete(m.goals, id)
	m.notifyLocked()
	return true, nil
}
func (m *memStore) List(ctx context.Context) ([]goal.Goal, error) {
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	out := make([]goal.Goal, 0, len(m.goals))
	for _, g := range m.goals {
		out = append(out, g)
	}
	return out, nil
}

// RecordRun models the terminal Run transaction: its idempotency identity and
// the Goal aggregate mutation appear together before the drive sees the event.
func (m *memStore) RecordRun(ctx context.Context, record goal.RunRecord) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if _, exists := m.runs[record.RunID]; exists {
		return nil
	}
	m.runs[record.RunID] = struct{}{}
	g, exists := m.goals[record.SessionID]
	if !exists || g.IncarnationID() != record.IncarnationID {
		return nil
	}
	replacement, err := g.RecordRun(record)
	if err != nil {
		return err
	}
	m.goals[replacement.SessionID()] = replacement
	m.notifyLocked()
	return nil
}

func (m *memStore) observe(id string) (goal.Goal, bool, <-chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	return g, ok, m.changed
}

func (m *memStore) notifyLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

// Run scripts one autonomous Run's outcome. setStatus simulates the model
// reporting a terminal goal outcome mid-Run (the driver re-reads the store
// after the run).
type scriptedRun struct {
	setStatus      goal.Status
	reason         string
	outcome        run.Outcome
	missingOutcome bool
	waiting        bool
	boundaryRunID  string
	cost           float64
	steps          int
	park           bool // violate the stream contract by omitting the root boundary
}

type fakeRuns struct {
	t             *testing.T
	store         *memStore
	script        []scriptedRun
	hold          chan struct{} // when non-nil, a run holds its terminal until this closes
	started       chan struct{}
	startErr      error
	startErrs     []error
	cancelStarted chan struct{}
	cancelRelease <-chan struct{}
	idleWaitStart chan struct{}
	idleRelease   <-chan struct{}
	mu            sync.Mutex
	calls         int
	startedRuns   int
	cancels       map[string]chan struct{}
	runDone       map[string]chan struct{}
	runGoals      map[string]goal.RunRecord
	canceled      int
	commands      []runs.StartCommand
}

func (f *fakeRuns) WaitSessionStartable(ctx context.Context, _ string) error {
	if f.idleWaitStart != nil {
		select {
		case f.idleWaitStart <- struct{}{}:
		default:
		}
	}
	if f.idleRelease == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.idleRelease:
		return nil
	}
}

func (*fakeRuns) AdmitSelection(modelref.Selection) error { return nil }

// eventSequence preserves the fake's hold-then-yield-terminal timing and the
// production subscription's caller-cancellation behavior behind its iter.Seq
// contract.
func eventSequence(ctx context.Context, events <-chan runs.Event) iter.Seq[runs.Event] {
	return func(yield func(runs.Event) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-events:
				if !open || !yield(event) {
					return
				}
			}
		}
	}
}

func (f *fakeRuns) nextScriptedRun() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	attempt := f.calls
	f.calls++
	var startErr error
	if attempt < len(f.startErrs) {
		startErr = f.startErrs[attempt]
	} else {
		startErr = f.startErr
	}
	runIndex := f.startedRuns
	if startErr == nil {
		f.startedRuns++
	}
	return runIndex, startErr
}

func (f *fakeRuns) notifyRunStarted() {
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
}

func (f *fakeRuns) applyScriptedGoalStatus(
	ctx context.Context,
	cmd runs.StartCommand,
	script scriptedRun,
) {
	if script.setStatus != "" {
		// Simulate the model reporting a terminal goal outcome mid-Run: a CAS on
		// the current version while retaining the Goal incarnation.
		current, _ := f.store.Get(ctx, cmd.SessionID)
		g, exists := current.Goal()
		if !exists {
			return
		}
		expected := g.Version()
		var replacement goal.Goal
		var err error
		switch script.setStatus {
		case goal.StatusComplete:
			replacement, err = g.Complete(time.Now())
		case goal.StatusBlocked:
			replacement, err = g.Block(goal.ReasonBlockedByModel, script.reason, time.Now())
		}
		if err == nil {
			change, replacementErr := goal.NewReplacement(expected, replacement)
			if replacementErr == nil {
				_, _ = f.store.Save(ctx, change)
			}
		}
	}
}

func (f *fakeRuns) registerScriptedRun(
	cmd runs.StartCommand,
	runID string,
) (cancelRequested chan struct{}, runFinished chan struct{}) {
	cancelRequested = make(chan struct{})
	runFinished = make(chan struct{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancels == nil {
		f.cancels = map[string]chan struct{}{}
		f.runDone = map[string]chan struct{}{}
		f.runGoals = map[string]goal.RunRecord{}
	}
	f.cancels[runID] = cancelRequested
	f.runDone[runID] = runFinished
	f.runGoals[runID] = goal.RunRecord{
		SessionID:     cmd.SessionID,
		IncarnationID: cmd.GoalIncarnationID,
		RunID:         runID,
	}
	return cancelRequested, runFinished
}

func (f *fakeRuns) emitScriptedRun(
	ctx context.Context,
	cmd runs.StartCommand,
	runID string,
	runIndex int,
	script scriptedRun,
	cancelRequested <-chan struct{},
	runFinished chan<- struct{},
	events chan<- runs.Event,
) {
	go func() {
		defer close(events)
		defer close(runFinished)
		defer func() {
			f.mu.Lock()
			delete(f.cancels, runID)
			delete(f.runDone, runID)
			delete(f.runGoals, runID)
			f.mu.Unlock()
		}()
		if f.hold != nil && runIndex == 0 {
			select {
			case <-f.hold:
			case <-cancelRequested:
				return
			}
		}
		if !script.park {
			cost := script.cost
			var outcome *run.Outcome
			if !script.missingOutcome && !script.waiting {
				outcome = &script.outcome
			}
			boundaryRunID := runID
			if script.boundaryRunID != "" {
				boundaryRunID = script.boundaryRunID
			}
			boundaryState := run.Running
			if script.waiting {
				boundaryState = run.Waiting
			}
			finishedRun := testsupport.MustRestoreRun(run.Snapshot{SessionID: cmd.SessionID,
				ID:      boundaryRunID,
				State:   boundaryState,
				Outcome: outcome,
				Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: script.steps, Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &cost}}})})

			if outcome != nil && cmd.GoalIncarnationID != "" {
				if err := f.store.RecordRun(context.WithoutCancel(ctx), goal.RunRecord{
					SessionID: cmd.SessionID, IncarnationID: cmd.GoalIncarnationID, RunID: runID,
					Outcome: *outcome, Cost: accountedGoalCost(cost), Steps: script.steps, CompletedAt: time.Now(),
				}); err != nil {
					f.t.Errorf("record terminal Goal Run: %v", err)
				}
			}
			events <- runs.Event{RunID: boundaryRunID, Payload: runs.SegmentFinished{Run: finishedRun}}
		}
	}()
}

func (f *fakeRuns) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	runIndex, startErr := f.nextScriptedRun()
	if startErr != nil {
		return runs.StartResult{}, startErr
	}
	cmd.Capabilities = cmd.Capabilities.Clone()
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	f.mu.Unlock()
	f.notifyRunStarted()

	events := make(chan runs.Event, 2)
	if runIndex >= len(f.script) {
		f.t.Errorf("unexpected extra run (call %d, script has %d)", runIndex, len(f.script))
		close(events)
		return runs.StartResult{SessionID: cmd.SessionID, Events: eventSequence(ctx, events)}, nil
	}
	script := f.script[runIndex]
	f.applyScriptedGoalStatus(ctx, cmd, script)
	runID := "run_" + strconv.Itoa(runIndex)
	cancelRequested, runFinished := f.registerScriptedRun(cmd, runID)
	f.emitScriptedRun(ctx, cmd, runID, runIndex, script, cancelRequested, runFinished, events)
	return runs.StartResult{RunID: runID, SessionID: cmd.SessionID, Events: eventSequence(ctx, events)}, nil
}

func (f *fakeRuns) Cancel(_ context.Context, cmd runs.CancelCommand) (runs.CancelResult, error) {
	if f.cancelStarted != nil {
		select {
		case f.cancelStarted <- struct{}{}:
		default:
		}
	}
	if f.cancelRelease != nil {
		<-f.cancelRelease
	}
	f.mu.Lock()
	cancel := f.cancels[cmd.RunID]
	done := f.runDone[cmd.RunID]
	record := f.runGoals[cmd.RunID]
	if cancel != nil {
		delete(f.cancels, cmd.RunID)
		close(cancel)
		f.canceled++
	}
	f.mu.Unlock()
	if cancel == nil {
		return runs.CancelResult{}, runs.ErrRunNotFound
	}
	<-done
	record.Outcome = run.OutcomeCanceled
	record.CompletedAt = time.Now()
	if err := f.store.RecordRun(context.Background(), record); err != nil {
		return runs.CancelResult{}, err
	}
	outcome := run.OutcomeCanceled
	return runs.CancelResult{Run: testsupport.MustRestoreRun(run.Snapshot{ID: cmd.RunID, State: run.Canceled, Outcome: &outcome})}, nil
}

// fakeSessions is the driver's session-existence check; sessions exist unless
// listed in deleted (nil map = all exist).
type fakeSessions struct {
	deleted   map[string]bool
	selection modelref.Selection
}

func (f *fakeSessions) ModelSelection(_ context.Context, id string) (modelref.Selection, bool, error) {
	if f.deleted[id] {
		return modelref.Selection{}, false, nil
	}
	if f.selection.Configured() {
		return f.selection, true, nil
	}
	return testGoalModelSelection(), true, nil
}

type terminalRaceRuns struct {
	store       *memStore
	started     chan struct{}
	session     string
	incarnation string
}

func (*terminalRaceRuns) WaitSessionStartable(context.Context, string) error { return nil }
func (*terminalRaceRuns) AdmitSelection(modelref.Selection) error            { return nil }

func (t *terminalRaceRuns) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	t.session = cmd.SessionID
	t.incarnation = cmd.GoalIncarnationID
	close(t.started)
	return runs.StartResult{
		RunID: "run_terminal_race", SessionID: cmd.SessionID,
		Events: func(func(runs.Event) bool) { <-ctx.Done() },
	}, nil
}

func (t *terminalRaceRuns) Cancel(context.Context, runs.CancelCommand) (runs.CancelResult, error) {
	err := t.store.RecordRun(context.Background(), goal.RunRecord{
		SessionID: t.session, IncarnationID: t.incarnation, RunID: "run_terminal_race",
		Outcome: run.OutcomeCompleted, CompletedAt: time.Now(),
	})
	return runs.CancelResult{}, err
}

func mustDriver(
	t testing.TB,
	store goals.Store,
	autonomousRuns goals.AutonomousRuns,
	sessions goals.SessionPolicyReader,
	mutations *goals.SessionMutations,
	ownership goals.DriveOwnership,
	instructions goals.RunInstructionBuilder,
) *goals.Driver {
	t.Helper()
	driver, err := goals.NewDriver(store, autonomousRuns, sessions, mutations, ownership, instructions)
	if err != nil {
		t.Fatal(err)
	}
	return driver
}

func newDriver(t *testing.T, store *memStore, script ...scriptedRun) *goals.Driver {
	t.Helper()
	d := mustDriver(t, store, &fakeRuns{t: t, store: store, script: script}, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	return d
}

func TestNewDriverRejectsIncompleteComposition(t *testing.T) {
	store := newMemStore()
	autonomousRuns := &fakeRuns{t: t, store: store}
	sessions := &fakeSessions{}
	mutations := goals.NewSessionMutations()
	ownership := &selectiveDriveOwnership{}
	var typedNilOwnership *selectiveDriveOwnership
	tests := []struct {
		name         string
		store        goals.Store
		runs         goals.AutonomousRuns
		sessions     goals.SessionPolicyReader
		mutations    *goals.SessionMutations
		ownership    goals.DriveOwnership
		instructions goals.RunInstructionBuilder
	}{
		{name: "store", runs: autonomousRuns, sessions: sessions, mutations: mutations, ownership: ownership, instructions: testPrompt},
		{name: "runs", store: store, sessions: sessions, mutations: mutations, ownership: ownership, instructions: testPrompt},
		{name: "sessions", store: store, runs: autonomousRuns, mutations: mutations, ownership: ownership, instructions: testPrompt},
		{name: "mutations", store: store, runs: autonomousRuns, sessions: sessions, ownership: ownership, instructions: testPrompt},
		{name: "ownership", store: store, runs: autonomousRuns, sessions: sessions, mutations: mutations, instructions: testPrompt},
		{name: "typed nil ownership", store: store, runs: autonomousRuns, sessions: sessions, mutations: mutations, ownership: typedNilOwnership, instructions: testPrompt},
		{name: "instructions", store: store, runs: autonomousRuns, sessions: sessions, mutations: mutations, ownership: ownership},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver, err := goals.NewDriver(
				test.store, test.runs, test.sessions, test.mutations, test.ownership, test.instructions,
			)
			if err == nil || driver != nil {
				t.Fatalf("NewDriver incomplete composition = (%v, %v)", driver, err)
			}
		})
	}
}

func shutdownDriver(d *goals.Driver) error {
	d.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return d.AwaitShutdown(ctx)
}

func cleanupDriver(t testing.TB, d *goals.Driver) {
	t.Helper()
	t.Cleanup(func() {
		if err := shutdownDriver(d); err != nil {
			t.Errorf("shutdown goal driver: %v", err)
		}
	})
}

func testPrompt(input goals.RunInstructionInput) string { return input.Objective }

func testGoalModelSelection() modelref.Selection {
	selection, err := modelref.New("p", "m")
	if err != nil {
		panic(err)
	}
	return selection
}

func TestDriverCurrentRejectsInvalidOrMismatchedStoreValue(t *testing.T) {
	mismatched, err := goal.Unwritten("other-session")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		current goal.Current
	}{
		{name: "invalid current"},
		{name: "mismatched session", current: mismatched},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := newMemStore()
			store := boundaryGoalStore{
				Store: base,
				get: func(context.Context, string) (goal.Current, error) {
					return test.current, nil
				},
			}
			driver := mustDriver(
				t,
				store,
				&fakeRuns{t: t, store: base},
				&fakeSessions{},
				goals.NewSessionMutations(),
				uncontendedDriveOwnership{},
				testPrompt,
			)
			cleanupDriver(t, driver)

			if _, _, err := driver.Current(t.Context(), "requested-session"); err == nil {
				t.Fatal("Current accepted an invalid persistence result")
			}
		})
	}
}

func TestReaderRejectsInvalidOrMismatchedStoreValue(t *testing.T) {
	mismatched, err := goal.Unwritten("other-session")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		current goal.Current
	}{
		{name: "invalid current"},
		{name: "mismatched session", current: mismatched},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := boundaryGoalStore{
				Store: newMemStore(),
				get: func(context.Context, string) (goal.Current, error) {
					return test.current, nil
				},
			}
			reader, err := goals.NewReader(store)
			if err != nil {
				t.Fatal(err)
			}

			if _, _, err := reader.Current(t.Context(), "requested-session"); err == nil {
				t.Fatal("Current accepted an invalid persistence result")
			}
			if _, err := reader.Active(t.Context(), "requested-session"); err == nil {
				t.Fatal("Active accepted an invalid persistence result")
			}
		})
	}
}

func unwrittenGoalVersion(t *testing.T, sessionID string) goal.Version {
	t.Helper()
	current, err := goal.Unwritten(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return current.Version()
}

func goalReplacement(t *testing.T, state goal.Goal, expected goal.Version) goal.Replacement {
	t.Helper()
	replacement, err := goal.NewReplacement(expected, state)
	if err != nil {
		t.Fatalf("prepare Goal replacement: %v", err)
	}
	return replacement
}

func loadStoredGoal(ctx context.Context, store goals.Store, sessionID string) (goal.Goal, bool, error) {
	current, err := store.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, false, err
	}
	value, exists := current.Goal()
	return value, exists, nil
}

func seedStoredGoal(t *testing.T, store *memStore, value goal.Goal) goal.Goal {
	t.Helper()
	applied, err := store.Save(t.Context(), goalReplacement(t, value, unwrittenGoalVersion(t, value.SessionID())))
	if err != nil || !applied {
		t.Fatalf("seed Goal: applied=%t err=%v", applied, err)
	}
	return value
}

func replaceStoredGoal(t *testing.T, store *memStore, replacement goal.Goal, expected goal.Version) goal.Goal {
	t.Helper()
	applied, err := store.Save(t.Context(), goalReplacement(t, replacement, expected))
	if err != nil || !applied {
		t.Fatalf("replace Goal: applied=%t err=%v", applied, err)
	}
	return replacement
}

// waitTestSessionGoal blocks until the test session's goal satisfies cond.
func waitTestSessionGoal(t *testing.T, store *memStore, cond func(goal.Goal, bool) bool) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		g, ok, changed := store.observe("s1")
		if cond(g, ok) {
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			t.Fatalf("goal never reached the expected state: %+v (present=%v)", g, ok)
		}
	}
}

func TestDriverCompletesAndClears(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok }) // completed → cleared
}

func TestDriverFreezesTheSessionExactSelectionWhenGoalHasNoOverride(t *testing.T) {
	store := newMemStore()
	runUseCases := &fakeRuns{
		t: t, store: store,
		script: []scriptedRun{{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted}},
	}
	selection, err := modelref.NewWithReasoningEffort("openai", "gpt-5.6-sol", "xhigh")
	if err != nil {
		t.Fatal(err)
	}
	d := mustDriver(t,
		store,
		runUseCases,
		&fakeSessions{selection: selection},
		goals.NewSessionMutations(),
		uncontendedDriveOwnership{},
		testPrompt,
	)
	cleanupDriver(t, d)
	started, err := d.Start(t.Context(), "s1", "do it", modelref.Selection{}, goal.UnlimitedBudget(), run.Capabilities{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started.ModelSelection() != selection {
		t.Fatalf("Goal selection = %+v, want Session selection %+v", started.ModelSelection(), selection)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
	runUseCases.mu.Lock()
	defer runUseCases.mu.Unlock()
	if len(runUseCases.commands) != 1 || runUseCases.commands[0].ModelSelection != selection {
		t.Fatalf("autonomous Run selections = %+v, want exact Goal selection", runUseCases.commands)
	}
}

func TestDriverCarriesFrozenGoalCapabilitiesIntoAutonomousRuns(t *testing.T) {
	store := newMemStore()
	runUseCases := &fakeRuns{
		t: t, store: store,
		script: []scriptedRun{{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted}},
	}
	d := mustDriver(t, store, runUseCases, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	want := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	started, err := d.Start(
		context.Background(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), want,
	)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !started.Capabilities().Equal(want) {
		t.Fatalf("Goal capabilities = %+v, want %+v", started.Capabilities(), want)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
	runUseCases.mu.Lock()
	defer runUseCases.mu.Unlock()
	if len(runUseCases.commands) != 1 || !runUseCases.commands[0].Capabilities.Equal(want) {
		t.Fatalf("autonomous Run commands = %+v, want one with %+v", runUseCases.commands, want)
	}
}

func TestDriverResumeRequiresTheFrozenGoalCapabilities(t *testing.T) {
	store := newMemStore()
	want := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	g, err := goal.New(
		"s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), want,
		"incarnation-capabilities", time.Unix(0, 0),
	)
	if err != nil {
		t.Fatalf("new Goal: %v", err)
	}
	g = seedStoredGoal(t, store, g)
	expected := g.Version()
	g, err = g.Pause(goal.ReasonAwaitingInput, "", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	_ = replaceStoredGoal(t, store, g, expected)
	runUseCases := &fakeRuns{
		t: t, store: store,
		script: []scriptedRun{{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted}},
	}
	d := mustDriver(t, store, runUseCases, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Resume(t.Context(), "s1", run.Capabilities{}); !errors.Is(err, goals.ErrInsufficientCapabilities) {
		t.Fatalf("Resume without Question capability = %v", err)
	}
	stillPaused, _, _ := loadStoredGoal(t.Context(), store, "s1")
	if stillPaused.Status() != goal.StatusPaused {
		t.Fatalf("rejected Resume changed Goal to %q", stillPaused.Status())
	}
	if _, err := d.Resume(t.Context(), "s1", want); err != nil {
		t.Fatalf("Resume with frozen capabilities: %v", err)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestDriverWaitsForCurrentSessionRunBeforeFirstGoalRun(t *testing.T) {
	store := newMemStore()
	release := make(chan struct{})
	waiting := make(chan struct{}, 1)
	runUseCases := &fakeRuns{
		t: t, store: store,
		script:        []scriptedRun{{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted}},
		idleWaitStart: waiting,
		idleRelease:   release,
	}
	d := mustDriver(t, store, runUseCases, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Start(t.Context(), "s1", "do it", modelref.Selection{}, goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("Goal driver did not wait for session idle")
	}
	runUseCases.mu.Lock()
	callsBeforeRelease := runUseCases.calls
	runUseCases.mu.Unlock()
	if callsBeforeRelease != 0 {
		t.Fatalf("Run Start calls before session idle = %d", callsBeforeRelease)
	}
	close(release)
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestDriverBlocksOnRunBudget(t *testing.T) {
	store := newMemStore()
	// Two completed Runs; MaxRuns=2 blocks after the second.
	d := newDriver(t, store, scriptedRun{outcome: run.OutcomeCompleted}, scriptedRun{outcome: run.OutcomeCompleted})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), limitedGoalBudget(t, goal.BudgetLimits{MaxRuns: goalIntLimit(2)}), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status() == goal.StatusBlocked })
	g, _, _ := loadStoredGoal(context.Background(), store, "s1")
	if g.Used().Runs != 2 {
		t.Fatalf("used Runs = %d, want 2", g.Used().Runs)
	}
}

func TestDriverAccountsModelBlockedTerminalRun(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{
		setStatus: goal.StatusBlocked,
		reason:    "needs credentials",
		outcome:   run.OutcomeCompleted,
		cost:      0.75,
		steps:     2,
	})
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
		return ok && g.Status() == goal.StatusBlocked && g.Used() == (goal.Usage{Runs: 1, Cost: accountedGoalCost(0.75), Steps: 2})
	})

	g, _, _ := loadStoredGoal(t.Context(), store, "s1")
	if g.Reason().Code() != goal.ReasonBlockedByModel || g.Reason().Detail() != "needs credentials" {
		t.Fatalf("blocked goal reason = %+v", g.Reason())
	}
}

func TestDriverPausesOnRunError(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{outcome: run.OutcomeFailed})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status() == goal.StatusPaused })
}

func TestDriverPausesOnMalformedTerminal(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{missingOutcome: true})
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
		return ok && g.Status() == goal.StatusPaused
	})

	g, _, _ := loadStoredGoal(t.Context(), store, "s1")
	if g.Reason().Code() != goal.ReasonTerminalOutcomeMissing {
		t.Fatalf("pause reason = %+v", g.Reason())
	}
	if g.Used().Runs != 0 {
		t.Fatalf("malformed terminal recorded %d Runs, want 0", g.Used().Runs)
	}
}

func TestDriverPausesOnWaitingRootBoundary(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store, scriptedRun{waiting: true})
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
		return ok && g.Status() == goal.StatusPaused
	})

	g, _, _ := loadStoredGoal(t.Context(), store, "s1")
	if g.Reason().Code() != goal.ReasonAwaitingInput {
		t.Fatalf("waiting root pause reason = %+v, want awaitingInput", g.Reason())
	}
	if g.Used().Runs != 0 {
		t.Fatalf("waiting root recorded %d completed Runs, want 0", g.Used().Runs)
	}
}

func TestResumeKeepsOutstandingGoalRunInSameIncarnation(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	g, err := goal.New(
		"s1",
		"do it",
		testGoalModelSelection(),
		limitedGoalBudget(t, goal.BudgetLimits{MaxRuns: goalIntLimit(1)}),
		run.Capabilities{},
		"incarnation-waiting",
		now,
	)
	if err != nil {
		t.Fatalf("new Goal: %v", err)
	}
	g = seedStoredGoal(t, store, g)
	expected := g.Version()
	g, err = g.Pause(goal.ReasonAwaitingInput, "", now)
	if err != nil {
		t.Fatal(err)
	}
	g = replaceStoredGoal(t, store, g, expected)

	waitStarted := make(chan struct{}, 1)
	releaseSession := make(chan struct{})
	fake := &fakeRuns{
		t:             t,
		store:         store,
		idleWaitStart: waitStarted,
		idleRelease:   releaseSession,
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	resumed, err := d.Resume(t.Context(), "s1", run.Capabilities{})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.IncarnationID() != g.IncarnationID() {
		t.Fatalf(
			"Resume incarnation = %q, want outstanding Run incarnation %q",
			resumed.IncarnationID(),
			g.IncarnationID(),
		)
	}
	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("resumed drive did not wait for the outstanding Goal Run")
	}

	// The parked Run resumes and terminalizes under the incarnation that admitted
	// it. Its one-Run budget charge must block the Goal before the waiting drive
	// can admit another Run.
	if err := store.RecordRun(t.Context(), goal.RunRecord{
		SessionID:     "s1",
		IncarnationID: g.IncarnationID(),
		RunID:         "run_waiting",
		Outcome:       run.OutcomeCompleted,
		CompletedAt:   time.Now().Add(time.Second),
	}); err != nil {
		t.Fatalf("record resumed Run: %v", err)
	}
	close(releaseSession)

	waitTestSessionGoal(t, store, func(current goal.Goal, ok bool) bool {
		return ok && current.Status() == goal.StatusBlocked && current.Used().Runs == 1
	})
	fake.mu.Lock()
	starts := fake.calls
	fake.mu.Unlock()
	if starts != 0 {
		t.Fatalf("Resume admitted %d extra Runs after the prior Run spent the budget", starts)
	}
}

func TestResumeObservesOutstandingGoalRunTerminalReport(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	g, err := goal.New(
		"s1",
		"do it",
		testGoalModelSelection(),
		goal.UnlimitedBudget(),
		run.Capabilities{},
		"incarnation-waiting",
		now,
	)
	if err != nil {
		t.Fatalf("new Goal: %v", err)
	}
	g = seedStoredGoal(t, store, g)
	expected := g.Version()
	g, err = g.Pause(goal.ReasonAwaitingInput, "", now)
	if err != nil {
		t.Fatal(err)
	}
	g = replaceStoredGoal(t, store, g, expected)

	waitStarted := make(chan struct{}, 1)
	releaseSession := make(chan struct{})
	fake := &fakeRuns{
		t:             t,
		store:         store,
		idleWaitStart: waitStarted,
		idleRelease:   releaseSession,
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, resumeErr := d.Resume(t.Context(), "s1", run.Capabilities{}); resumeErr != nil {
		t.Fatalf("Resume: %v", resumeErr)
	}
	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("resumed drive did not wait for the outstanding Goal Run")
	}

	// The same parked Run may report the objective complete before its terminal
	// transaction releases the Session. Resume must preserve that Run's
	// incarnation so the report applies, then the waiting drive must settle it
	// without launching another Run from its pre-wait snapshot.
	reporter, err := goals.NewOutcomeReporter(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reporter.Report(t.Context(), goals.ReportCommand{
		SessionID:     "s1",
		IncarnationID: g.IncarnationID(),
		Outcome:       goal.StatusComplete,
	})
	if err != nil || result != goals.ReportApplied {
		t.Fatalf("report outstanding Run outcome: result=%v err=%v", result, err)
	}
	if err := store.RecordRun(t.Context(), goal.RunRecord{
		SessionID:     "s1",
		IncarnationID: g.IncarnationID(),
		RunID:         "run_waiting",
		Outcome:       run.OutcomeCompleted,
		CompletedAt:   time.Now().Add(time.Second),
	}); err != nil {
		t.Fatalf("record outstanding Run terminal: %v", err)
	}
	close(releaseSession)

	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	starts := fake.calls
	fake.mu.Unlock()
	if starts != 0 {
		t.Fatalf("Resume admitted %d extra Runs after the prior Run completed the Goal", starts)
	}
}

func TestDriverTreatsMissingRootBoundaryAsContractFailure(t *testing.T) {
	for name, script := range map[string]scriptedRun{
		"empty stream":       {park: true},
		"child waiting only": {waiting: true, boundaryRunID: "run_child"},
	} {
		t.Run(name, func(t *testing.T) {
			store := newMemStore()
			d := newDriver(t, store, script)
			if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
				t.Fatalf("Start: %v", err)
			}
			waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
				return ok && g.Status() == goal.StatusPaused
			})

			g, _, _ := loadStoredGoal(t.Context(), store, "s1")
			if g.Reason().Code() != goal.ReasonTerminalOutcomeMissing {
				t.Fatalf("missing root pause reason = %+v, want terminalOutcomeMissing", g.Reason())
			}
		})
	}
}

func TestPauseCASUsesAuthoritativeCompleteOutcome(t *testing.T) {
	base := newMemStore()
	store := &pauseCompletionRaceStore{memStore: base}
	fake := &fakeRuns{t: t, store: base, startErr: runs.ErrSessionBusy}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, base, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 1 {
		t.Fatalf("run start calls = %d, want one", calls)
	}
}

func TestDriverRetriesSessionBusyAdmissionRace(t *testing.T) {
	store := newMemStore()
	fake := &fakeRuns{
		t:         t,
		store:     store,
		startErrs: []error{runs.ErrRunAdmissionBusy},
		script: []scriptedRun{{
			setStatus: goal.StatusComplete,
			outcome:   run.OutcomeCompleted,
		}},
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 2 {
		t.Fatalf("run start calls = %d, want one lost race plus one accepted run", calls)
	}
}

func TestDriverAccumulatesCostBudget(t *testing.T) {
	store := newMemStore()
	// Each Run costs 0.5; MaxCostUSD 1.0 blocks after the second (used 1.0).
	d := newDriver(t, store,
		scriptedRun{outcome: run.OutcomeCompleted, cost: 0.5},
		scriptedRun{outcome: run.OutcomeCompleted, cost: 0.5})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), limitedGoalBudget(t, goal.BudgetLimits{MaxCostUSD: goalCostLimit(1)}), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status() == goal.StatusBlocked })
	g, _, _ := loadStoredGoal(context.Background(), store, "s1")
	if cost, available := g.Used().Cost.USD(); !available || cost != 1.0 {
		t.Fatalf("used cost = %v, available %t; want 1.0", cost, available)
	}
}

func TestDriverRefusesConcurrentStart(t *testing.T) {
	store := newMemStore()
	g, _ := goal.New("s1", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-active", time.Unix(0, 0))
	_, _ = store.Save(context.Background(), goalReplacement(t, g, unwrittenGoalVersion(t, "s1")))
	// Refusing also restores the in-process drive for the active row it found, so
	// the fake has to have a Run to serve. Scripting none asserted the opposite
	// — that adoption never happens — and passed only when the assertion outran
	// the drive the refusal had just launched.
	started := make(chan struct{}, 1)
	fake := &fakeRuns{
		t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeCompleted}},
		hold: make(chan struct{}), started: started,
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "s1", "obj2", modelref.Selection{}, goal.UnlimitedBudget(), run.Capabilities{}); err != goals.ErrGoalActive {
		t.Fatalf("Start on active goal = %v, want ErrGoalActive", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Start on an active goal did not restore its drive")
	}
}

// TestDriverStopPausesRunningGoal stops a goal while a run is in flight and
// asserts it settles on paused without launching another run — the drive must
// honor the pause, never re-affirm active over it (the checkpoint-vs-Stop race).
func TestDriverStopPausesRunningGoal(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeCompleted}}, hold: hold, started: started}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started: // the drive launched the run and is draining its terminal
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	if _, err := d.Stop(context.Background(), "s1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	fake.mu.Lock()
	canceled := fake.canceled
	fake.mu.Unlock()
	if canceled != 1 {
		t.Fatalf("Stop canceled %d owned runs, want 1", canceled)
	}
	if err := shutdownDriver(d); err != nil { // join proves no checkpoint remains in flight
		t.Fatalf("Close: %v", err)
	}
	if g, _, _ := loadStoredGoal(context.Background(), store, "s1"); g.Status() != goal.StatusPaused {
		t.Fatalf("goal not stably paused after stop: %q", g.Status())
	}
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 1 {
		t.Fatalf("stopped goal launched %d runs, want 1", calls)
	}
}

func TestDriverUpdateObjectiveQuiescesAndContinuesTheActiveGoal(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 2)
	fake := &fakeRuns{
		t: t, store: store, hold: hold, started: started,
		script: []scriptedRun{
			{outcome: run.OutcomeCompleted},
			{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted},
		},
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	first, err := d.Start(t.Context(), "s1", "first", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first Goal Run did not start")
	}

	updated, err := d.UpdateObjective(t.Context(), "s1", "second", run.Capabilities{})
	if err != nil {
		t.Fatalf("UpdateObjective: %v", err)
	}
	if updated.Objective() != "second" || updated.Status() != goal.StatusActive {
		t.Fatalf("updated Goal = %+v", updated)
	}
	if updated.IncarnationID() == first.IncarnationID() || updated.Used().Runs != 1 {
		t.Fatalf("updated provenance/accounting = incarnation %q used %+v", updated.IncarnationID(), updated.Used())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("revised Goal did not continue with a new Run")
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestDriverClearQuiescesAndRemovesTheActiveGoal(t *testing.T) {
	store := newMemStore()
	started := make(chan struct{}, 1)
	fake := &fakeRuns{
		t: t, store: store, hold: make(chan struct{}), started: started,
		script: []scriptedRun{{outcome: run.OutcomeCompleted}},
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "clear me", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Goal Run did not start")
	}
	if err := d.Clear(t.Context(), "s1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, present, err := loadStoredGoal(t.Context(), store, "s1"); err != nil || present {
		t.Fatalf("Goal after Clear: present=%t err=%v", present, err)
	}
	if err := d.Clear(t.Context(), "s1"); err != nil {
		t.Fatalf("idempotent Clear: %v", err)
	}
}

func TestDriverStopFoldsTerminalRaceBeforePausing(t *testing.T) {
	store := newMemStore()
	fake := &terminalRaceRuns{store: store, started: make(chan struct{})}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatal("goal run did not start")
	}

	stopped, err := d.Stop(t.Context(), "s1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped.Status() != goal.StatusPaused || stopped.Reason().Code() != goal.ReasonStoppedByUser {
		t.Fatalf("stopped goal = status %q reason %+v", stopped.Status(), stopped.Reason())
	}
	if stopped.Used().Runs != 1 {
		t.Fatalf("terminal race accounting = %d Runs, want 1", stopped.Used().Runs)
	}
}

func TestDriverFreshStartReplacesStoppedGoal(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, hold: hold, started: started, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
		{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted},
	}}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "first", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	<-started
	stopped, err := d.Stop(t.Context(), "s1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	fresh, err := d.Start(t.Context(), "s1", "second", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{})
	if err != nil {
		t.Fatalf("fresh Start: %v", err)
	}
	if fresh.Objective() != "second" || fresh.Revision() != 1 || fresh.IncarnationID() == stopped.IncarnationID() {
		t.Fatalf("fresh goal = %+v, stopped = %+v", fresh, stopped)
	}
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })
}

func TestDriverStoreFailureRemainsAddressableUntilStop(t *testing.T) {
	base := newMemStore()
	storeErr := errors.New("goal store read failed")
	store := &faultingGoalStore{
		memStore: base,
		failAt:   4,
		err:      storeErr,
		failed:   make(chan struct{}),
	}
	fake := &fakeRuns{t: t, store: base, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
		{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted},
	}}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "old objective", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("start old goal: %v", err)
	}
	select {
	case <-store.failed:
	case <-time.After(2 * time.Second):
		t.Fatal("goal supervisor did not encounter the store failure")
	}

	// The store signal marks the failed read, not the later goalDrive completion
	// publication. A Start racing that publication may still observe the drive as
	// active; once published, the failure must remain addressable until Stop.
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := d.Start(t.Context(), "s1", "replacement", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{})
		if errors.Is(err, storeErr) {
			break
		}
		if !errors.Is(err, goals.ErrGoalActive) {
			t.Fatalf("Start while supervisor is faulted = %v, want active transition or store error", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("goal supervisor failure did not become addressable")
		}
		time.Sleep(time.Millisecond)
	}
	stopped, err := d.Stop(t.Context(), "s1")
	if !errors.Is(err, storeErr) {
		t.Fatalf("Stop faulted supervisor = %v, want store error", err)
	}
	if stopped.Status() != goal.StatusPaused {
		t.Fatalf("stopped status = %s, want paused", stopped.Status())
	}
	if _, err := d.Resume(t.Context(), "s1", run.Capabilities{}); err != nil {
		t.Fatalf("Resume after observing supervisor fault: %v", err)
	}
	waitTestSessionGoal(t, base, func(_ goal.Goal, ok bool) bool { return !ok })
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 2 {
		t.Fatalf("run starts = %d, want one before and one after recovery", calls)
	}
}

func TestDriverStopSaveFailureDoesNotPublishUserStop(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
	}, hold: hold, started: started}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	stopErr := errors.New("goal store unavailable")
	store.failNextStopSave(stopErr)
	if _, err := d.Stop(t.Context(), "s1"); !errors.Is(err, stopErr) {
		t.Fatalf("Stop error = %v, want %v", err, stopErr)
	}
	// The Run cancellation may already have produced its own durable pause, but
	// the failed user-stop write must not be reported as committed.
	got, ok, err := loadStoredGoal(t.Context(), store, "s1")
	if err != nil || !ok {
		t.Fatalf("goal after failed Stop = present=%v err=%v", ok, err)
	}
	if got.Status() == goal.StatusActive {
		t.Fatal("failed Stop left an active goal without a driver")
	}
	if got.Reason().Code() == goal.ReasonStoppedByUser {
		t.Fatal("failed Stop published the uncommitted user-stop reason")
	}
}

func TestDriverContinuesAfterTerminalAccounting(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{
		{outcome: run.OutcomeCompleted},
		{setStatus: goal.StatusComplete, outcome: run.OutcomeCompleted},
	}, hold: hold, started: started}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}

	close(hold)
	waitTestSessionGoal(t, store, func(_ goal.Goal, ok bool) bool { return !ok })

	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 2 {
		t.Fatalf("run starts = %d, want one supervisor to start 2 Runs", calls)
	}
}

func TestDriverRejectsCommandsAfterShutdown(t *testing.T) {
	store := newMemStore()
	d := newDriver(t, store)
	d.BeginShutdown()

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); !errors.Is(err, goals.ErrClosed) {
		t.Fatalf("Start after shutdown = %v, want ErrClosed", err)
	}
	if _, ok, err := loadStoredGoal(t.Context(), store, "s1"); err != nil || ok {
		t.Fatalf("shutdown start persisted goal: present=%v err=%v", ok, err)
	}
}

func TestDriverShutdownJoinsRunCancellation(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	cancelStarted := make(chan struct{}, 1)
	cancelRelease := make(chan struct{})
	fake := &fakeRuns{
		t:             t,
		store:         store,
		script:        []scriptedRun{{outcome: run.OutcomeCompleted}},
		hold:          hold,
		started:       started,
		cancelStarted: cancelStarted,
		cancelRelease: cancelRelease,
	}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)

	if _, err := d.Start(t.Context(), "s1", "do it", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started
	d.BeginShutdown()
	<-cancelStarted

	waitCtx, cancelWait := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancelWait()
	if err := d.AwaitShutdown(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AwaitShutdown before Run cancellation completed = %v, want deadline", err)
	}

	close(cancelRelease)
	if err := d.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("AwaitShutdown after Run cancellation completed: %v", err)
	}
}

func TestMemStoreRejectsCancellationWhileWaitingForLock(t *testing.T) {
	store := newMemStore()
	checked := make(chan struct{})
	continueLock := make(chan struct{})
	store.afterContextCheck = func() {
		close(checked)
		<-continueLock
	}

	store.mu.Lock()
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := store.Save(ctx, goal.Replacement{})
		result <- err
	}()
	<-checked // the first cancellation check has passed; Save is about to wait
	cancel()
	close(continueLock)
	store.mu.Unlock()

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
}

// TestDriverEmitsRunSpan proves the observability is real (not just no-op): a
// goal.run span carries the session, Run ordinal, and the run's outcome/usage.
func TestDriverEmitsRunSpan(t *testing.T) {
	exporter := installGoalTraceCapture(t)

	store := newMemStore()
	// One completed Run; MaxRuns=1 blocks after it, so the span has run.outcome.
	d := newDriver(t, store, scriptedRun{outcome: run.OutcomeCompleted, cost: 0.3, steps: 2})
	if _, err := d.Start(context.Background(), "s1", "do it", testGoalModelSelection(), limitedGoalBudget(t, goal.BudgetLimits{MaxRuns: goalIntLimit(1)}), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool { return ok && g.Status() == goal.StatusBlocked })
	select {
	case <-exporter.exported:
	case <-time.After(2 * time.Second):
		t.Fatal("goal.run span did not finish")
	}
	if err := shutdownDriver(d); err != nil { // goal.run ends before its span is inspected
		t.Fatalf("Close: %v", err)
	}

	var span *tracetest.SpanStub
	for _, s := range exporter.GetSpans() {
		if s.Name == "goal.run" {
			stub := s
			span = &stub
			break
		}
	}
	if span == nil {
		t.Fatal("no goal.run span was emitted")
	}
	attrs := map[string]string{}
	for _, a := range span.Attributes {
		attrs[string(a.Key)] = a.Value.String()
	}
	if attrs["goal.session"] != "s1" || attrs["goal.run_ordinal"] != "1" || attrs["run.outcome"] != "completed" {
		t.Fatalf("goal.run span attributes = %v, want session s1 / Run ordinal 1 / outcome completed", attrs)
	}
}

type notifyingSpanExporter struct {
	*tracetest.InMemoryExporter
	exported chan struct{}
}

func (n *notifyingSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := n.InMemoryExporter.ExportSpans(ctx, spans)
	select {
	case n.exported <- struct{}{}:
	default:
	}
	return err
}

func TestReconcileDegradesActiveAndClearsComplete(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	active, _ := goal.New("live", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-live", now)
	done, _ := goal.New("done", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-done", now)
	paused, _ := goal.New("held", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-held", now)
	_ = seedStoredGoal(t, store, active)
	done = seedStoredGoal(t, store, done)
	doneReplacement, _ := done.Complete(now)
	_ = replaceStoredGoal(t, store, doneReplacement, done.Version())
	_ = seedStoredGoal(t, store, paused)
	pausedReplacement, _ := paused.Pause(goal.ReasonAwaitingInput, "", now)
	_ = replaceStoredGoal(t, store, pausedReplacement, paused.Version())

	d := newDriver(t, store)
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if g, _, _ := loadStoredGoal(context.Background(), store, "live"); g.Status() != goal.StatusPaused {
		t.Fatalf("active goal not degraded: %q", g.Status())
	}
	if _, ok, _ := loadStoredGoal(context.Background(), store, "done"); ok {
		t.Fatal("complete goal not cleared")
	}
	if g, _, _ := loadStoredGoal(context.Background(), store, "held"); g.Status() != goal.StatusPaused || g.Reason().Code() != goal.ReasonAwaitingInput {
		t.Fatalf("paused goal was disturbed: %+v", g)
	}
}

type testDriveLease struct{ release func() }

func (t testDriveLease) Release() { t.release() }

type uncontendedDriveOwnership struct{}

func (uncontendedDriveOwnership) TryGoalDrive(string) (goals.DriveLease, bool, error) {
	return testDriveLease{release: func() {}}, true, nil
}

type selectiveDriveOwnership struct {
	busy     map[string]bool
	released map[string]int
	calls    int
	err      error
}

func (s *selectiveDriveOwnership) TryGoalDrive(sessionID string) (goals.DriveLease, bool, error) {
	s.calls++
	if s.err != nil {
		return nil, false, s.err
	}
	if s.busy[sessionID] {
		return nil, false, nil
	}
	return testDriveLease{release: func() { s.released[sessionID]++ }}, true, nil
}

func TestReconcilePreservesOwnershipFailureAndActiveGoal(t *testing.T) {
	store := newMemStore()
	active, err := goal.New("session", "objective", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "incarnation", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	seedStoredGoal(t, store, active)
	cause := errors.New("lease filesystem unavailable")
	driver := mustDriver(t, store, &fakeRuns{t: t, store: store}, &fakeSessions{}, goals.NewSessionMutations(), &selectiveDriveOwnership{err: cause}, testPrompt)
	cleanupDriver(t, driver)
	if err := driver.Reconcile(t.Context()); !errors.Is(err, cause) {
		t.Fatalf("Reconcile error = %v, want ownership cause", err)
	}
	if got, _, err := loadStoredGoal(t.Context(), store, active.SessionID()); err != nil || got.Status() != goal.StatusActive {
		t.Fatalf("Goal after failed ownership = %+v, %v", got, err)
	}
}

func TestReconcileValidatesCompleteCatalogBeforeAcquiringOwnership(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	active, err := goal.New(
		"session", "objective", testGoalModelSelection(), goal.UnlimitedBudget(),
		run.Capabilities{}, "incarnation", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		listed []goal.Goal
	}{
		{name: "invalid item", listed: []goal.Goal{{}}},
		{name: "duplicate session", listed: []goal.Goal{active, active}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := newMemStore()
			store := boundaryGoalStore{
				Store: base,
				list: func(context.Context) ([]goal.Goal, error) {
					return test.listed, nil
				},
			}
			ownership := &selectiveDriveOwnership{
				busy: map[string]bool{}, released: map[string]int{},
			}
			driver := mustDriver(
				t,
				store,
				&fakeRuns{t: t, store: base},
				&fakeSessions{},
				goals.NewSessionMutations(),
				ownership,
				testPrompt,
			)
			cleanupDriver(t, driver)

			if err := driver.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an invalid persistence catalog")
			}
			if ownership.calls != 0 {
				t.Fatalf("drive ownership acquired %d times before catalog validation", ownership.calls)
			}
		})
	}
}

type activateOnSecondReadStore struct {
	goals.Store
	base  *memStore
	reads atomic.Int32
	now   time.Time
}

func (a *activateOnSecondReadStore) Get(
	ctx context.Context,
	sessionID string,
) (goal.Current, error) {
	if sessionID == "s1" && a.reads.Add(1) == 2 {
		current, err := a.base.Get(ctx, sessionID)
		if err != nil {
			return goal.Current{}, err
		}
		paused, found := current.Goal()
		if !found {
			return goal.Current{}, errors.New("test Goal disappeared before foreign resume")
		}
		active, err := paused.Resume(a.now)
		if err != nil {
			return goal.Current{}, err
		}
		change, replacementErr := goal.NewReplacement(paused.Version(), active)
		if replacementErr != nil {
			return goal.Current{}, replacementErr
		}
		if applied, err := a.base.Save(ctx, change); err != nil || !applied {
			return goal.Current{}, fmt.Errorf("foreign resume: applied=%t err=%v", applied, err)
		}
	}
	return a.base.Get(ctx, sessionID)
}

func TestDriverStopRejectsGoalThatBecameForeignOwnedAfterInitialRead(t *testing.T) {
	base := newMemStore()
	now := time.Unix(10, 0).UTC()
	active, err := goal.New(
		"s1", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{},
		"inc-stop-ownership", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	active = seedStoredGoal(t, base, active)
	paused, err := active.Pause(goal.ReasonAwaitingInput, "", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	replaceStoredGoal(t, base, paused, active.Version())
	store := &activateOnSecondReadStore{
		Store: base,
		base:  base,
		now:   now.Add(2 * time.Second),
	}
	ownership := &selectiveDriveOwnership{
		busy: map[string]bool{"s1": true}, released: map[string]int{},
	}
	driver := mustDriver(t,
		store,
		&fakeRuns{t: t, store: base},
		&fakeSessions{},
		goals.NewSessionMutations(),
		ownership,
		testPrompt,
	)
	cleanupDriver(t, driver)

	if _, err := driver.Stop(t.Context(), "s1"); !errors.Is(err, goals.ErrGoalOwned) {
		t.Fatalf("Stop error = %v, want ErrGoalOwned", err)
	}
	current, found, err := loadStoredGoal(t.Context(), store, "s1")
	if err != nil || !found {
		t.Fatalf("foreign Goal = %+v, found=%t err=%v", current, found, err)
	}
	if current.Status() != goal.StatusActive || current.Reason().Code() == goal.ReasonStoppedByUser {
		t.Fatalf("foreign-owned Goal was mutated by Stop: %+v", current)
	}
}

func TestReconcileSkipsGoalDriveOwnedByAnotherRuntime(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	foreign, _ := goal.New("foreign", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "inc-foreign", now)
	abandoned, _ := goal.New("abandoned", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "inc-abandoned", now)
	for _, value := range []goal.Goal{foreign, abandoned} {
		if applied, err := store.Save(t.Context(), goalReplacement(t, value, unwrittenGoalVersion(t, value.SessionID()))); err != nil || !applied {
			t.Fatalf("seed Goal %q: applied=%t err=%v", value.SessionID(), applied, err)
		}
	}
	ownership := &selectiveDriveOwnership{
		busy: map[string]bool{foreign.SessionID(): true}, released: map[string]int{},
	}
	d := mustDriver(t,
		store,
		&fakeRuns{t: t, store: store},
		&fakeSessions{},
		goals.NewSessionMutations(),
		ownership,
		testPrompt,
	)
	cleanupDriver(t, d)
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, _, _ := loadStoredGoal(t.Context(), store, foreign.SessionID()); got.Status() != goal.StatusActive {
		t.Fatalf("foreign-owned Goal changed to %q", got.Status())
	}
	if got, _, _ := loadStoredGoal(t.Context(), store, abandoned.SessionID()); got.Status() != goal.StatusPaused ||
		got.Reason().Code() != goal.ReasonRuntimeRestarted {
		t.Fatalf("abandoned Goal was not paused: %+v", got)
	}
	if ownership.released[abandoned.SessionID()] != 1 || ownership.released[foreign.SessionID()] != 0 {
		t.Fatalf("drive releases = %+v", ownership.released)
	}
}

func TestReconcileFailsClosedWhenARecoveryCASDoesNotLand(t *testing.T) {
	for _, test := range []struct {
		name        string
		status      goal.Status
		rejectSave  bool
		rejectClear bool
	}{
		{name: "active pause", status: goal.StatusActive, rejectSave: true},
		{name: "complete clear", status: goal.StatusComplete, rejectClear: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newMemStore()
			now := time.Unix(0, 0)
			seed, err := goal.New("session", "objective", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "incarnation", now)
			if err != nil {
				t.Fatal(err)
			}
			if test.status == goal.StatusComplete {
				seed = seedStoredGoal(t, store, seed)
				expected := seed.Version()
				seed, err = seed.Complete(now)
				if err != nil {
					t.Fatal(err)
				}
				seed = replaceStoredGoal(t, store, seed, expected)
			} else {
				seed = seedStoredGoal(t, store, seed)
			}
			conflicting := conflictingReconcileStore{
				Store: store, rejectSave: test.rejectSave, rejectClear: test.rejectClear,
			}
			d := mustDriver(t, conflicting, &fakeRuns{t: t, store: store}, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
			cleanupDriver(t, d)

			if reconcileErr := d.Reconcile(t.Context()); !errors.Is(reconcileErr, goals.ErrGoalConflict) {
				t.Fatalf("Reconcile conflict error = %v, want %v", reconcileErr, goals.ErrGoalConflict)
			}
			current, present, err := loadStoredGoal(t.Context(), store, seed.SessionID())
			if err != nil || !present || current.Version() != seed.Version() || current.Status() != test.status {
				t.Fatalf("conflicting recovery changed Goal: present=%t current=%+v err=%v", present, current, err)
			}
		})
	}
}

func TestStartRefusesMissingSession(t *testing.T) {
	store := newMemStore()
	fake := &fakeRuns{t: t, store: store}
	d := mustDriver(t, store, fake, &fakeSessions{deleted: map[string]bool{"ghost": true}}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "ghost", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != goals.ErrNoSession {
		t.Fatalf("Start(missing session) = %v, want ErrNoSession", err)
	}
	if _, ok, _ := loadStoredGoal(context.Background(), store, "ghost"); ok {
		t.Fatal("a goal was created for a nonexistent session")
	}
	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	if calls != 0 {
		t.Fatalf("launched %d runs for a missing session, want 0", calls)
	}
}

func TestReconcileSweepsOrphanGoal(t *testing.T) {
	store := newMemStore()
	now := time.Unix(0, 0)
	orphan, _ := goal.New("gone", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-gone", now) // session deleted while down
	_, _ = store.Save(context.Background(), goalReplacement(t, orphan, unwrittenGoalVersion(t, orphan.SessionID())))
	kept, _ := goal.New("live", "obj", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-live", now)
	kept = seedStoredGoal(t, store, kept)
	expected := kept.Version()
	kept, _ = kept.Pause(goal.ReasonAwaitingInput, "", now)
	_ = replaceStoredGoal(t, store, kept, expected)

	d := mustDriver(t, store, &fakeRuns{t: t, store: store}, &fakeSessions{deleted: map[string]bool{"gone": true}}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, ok, _ := loadStoredGoal(context.Background(), store, "gone"); ok {
		t.Fatal("orphan goal for a deleted session was not swept")
	}
	if _, ok, _ := loadStoredGoal(context.Background(), store, "live"); !ok {
		t.Fatal("a goal for a live session was wrongly swept")
	}
}

// TestStopThenStartRejectsStragglerWrite is the race-#4 keystone: a run whose
// goal was stopped and replaced by a fresh Start (a new incarnation) must not
// clobber the new Goal when its straggler drive finally drains. The drive's
// incarnation no longer matches, so it stops without writing.
func TestStopThenStartRejectsStragglerWrite(t *testing.T) {
	store := newMemStore()
	hold := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &fakeRuns{t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeFailed}}, hold: hold, started: started}
	d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)
	cleanupDriver(t, d)

	if _, err := d.Start(context.Background(), "s1", "objective one", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goal driver did not launch its first run")
	}
	// User stops the Goal (paused, drive canceled) then starts a fresh objective.
	// Save the new Goal with a new incarnation exactly as Start would
	// (no second drive launched, to keep the straggler the only writer under test).
	stopped, err := d.Stop(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	newGoal, _ := goal.New("s1", "objective two", testGoalModelSelection(), goal.UnlimitedBudget(), run.Capabilities{}, "lease-replacement", time.Unix(0, 0))
	if applied, err := store.Save(context.Background(), goalReplacement(t, newGoal, stopped.Version())); err != nil || !applied {
		t.Fatalf("seed replacement goal: applied=%v err=%v", applied, err)
	}

	close(hold) // release the straggler Run; its drive drains and re-reads the Goal
	if err := shutdownDriver(d); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, ok, _ := loadStoredGoal(context.Background(), store, "s1")
	if !ok || got.IncarnationID() != "lease-replacement" || got.Status() != goal.StatusActive || got.Objective() != "objective two" {
		t.Fatalf("straggler clobbered the replacement goal: %+v", got)
	}
}

// TestStopResumeRaceNeverWedgesActive runs Stop and Resume concurrently on a
// paused goal. The command mutex must serialize them so the goal never ends up
// active with no drive owning it — a wedge would leave it active forever, and
// waitGoal would time out. Run under -race to also catch memory races.
func TestStopResumeRaceNeverWedgesActive(t *testing.T) {
	for i := 0; i < 50; i++ {
		store := newMemStore()
		g, _ := goal.New("s1", "obj", testGoalModelSelection(), limitedGoalBudget(t, goal.BudgetLimits{MaxRuns: goalIntLimit(1)}), run.Capabilities{}, "lease-seed", time.Unix(0, 0))
		g = seedStoredGoal(t, store, g)
		expected := g.Version()
		g, _ = g.Pause(goal.ReasonAwaitingInput, "", time.Unix(0, 0))
		_ = replaceStoredGoal(t, store, g, expected)
		fake := &fakeRuns{t: t, store: store, script: []scriptedRun{{outcome: run.OutcomeCompleted}}}
		d := mustDriver(t, store, fake, &fakeSessions{}, goals.NewSessionMutations(), uncontendedDriveOwnership{}, testPrompt)

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = d.Stop(context.Background(), "s1") })
		wg.Go(func() { _, _ = d.Resume(context.Background(), "s1", run.Capabilities{}) })
		wg.Wait()

		// Settles non-active: paused (Stop won) or blocked (Resume's drive ran its one
		// budgeted Run). Active-with-no-drive would never leave active.
		waitTestSessionGoal(t, store, func(g goal.Goal, ok bool) bool {
			return ok && g.Status() != goal.StatusActive
		})

		if err := shutdownDriver(d); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}

func limitedGoalBudget(t *testing.T, limits goal.BudgetLimits) goal.Budget {
	t.Helper()
	budget, err := goal.NewBudget(limits)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func goalIntLimit(value int) *int { return &value }

func goalCostLimit(value float64) *float64 { return &value }

func accountedGoalCost(value float64) accounting.Cost {
	cost, err := accounting.NewCost(value)
	if err != nil {
		panic(err)
	}
	return cost
}
