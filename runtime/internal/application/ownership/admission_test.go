package ownership

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func newTestGate(t *testing.T) *Gate {
	t.Helper()
	gate, err := NewGate(GateConfig{ResolveTree: func(path string) (string, error) { return path, nil }, RetirementPending: noResourceRetirements})
	if err != nil {
		t.Fatal(err)
	}
	return gate
}

func TestNewGateRequiresPhysicalIdentityResolver(t *testing.T) {
	if gate, err := NewGate(GateConfig{}); err == nil || gate != nil {
		t.Fatalf("NewGate(GateConfig{}) = %#v, %v", gate, err)
	}
}

func TestGateHoldsSessionThroughMaintenance(t *testing.T) {
	gate := newTestGate(t)
	opening, ok, _ := gate.AcquireRun(t.Context(), "ses_1", "/repo")
	if !ok {
		t.Fatal("opening admission was rejected")
	}
	if !opening.Admit("run_1") {
		t.Fatal("opening admission did not become live")
	}
	if _, mutationOk, _ := gate.AcquireWorkingTreeMutation("/repo"); mutationOk {
		t.Fatal("live run did not block a working-tree mutation")
	}

	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("terminal maintenance did not acquire the run")
	}
	if !gate.ActiveSessions()["ses_1"] {
		t.Fatal("maintenance release erased the session claim")
	}
	if _, sessionOk, _ := gate.AcquireSession(t.Context(), "ses_1"); sessionOk {
		t.Fatal("new admission crossed the maintenance boundary")
	}
	if _, mutationOk, _ := gate.AcquireWorkingTreeMutation("/repo"); mutationOk {
		t.Fatal("terminal maintenance did not retain the working tree")
	}

	releaseMaintenance()
	if gate.ActiveSessions()["ses_1"] {
		t.Fatal("maintenance release left the session active")
	}
	mutationRelease, ok, _ := gate.AcquireWorkingTreeMutation("/repo")
	if !ok {
		t.Fatal("maintenance release left the working tree busy")
	}
	mutationRelease()
}

func TestGateExcludesWorkingTreeRunAdmissionsAndMutations(t *testing.T) {
	gate := newTestGate(t)
	const cwd = "/repo"

	first, ok, _ := gate.AcquireRun(t.Context(), "ses_1", cwd)
	if !ok {
		t.Fatal("first run admission was rejected")
	}
	second, ok, _ := gate.AcquireRun(t.Context(), "ses_2", cwd)
	if !ok {
		t.Fatal("second run admission was rejected")
	}
	if _, mutationOk, _ := gate.AcquireWorkingTreeMutation(cwd); mutationOk {
		t.Fatal("mutation admission crossed pending run admissions")
	}

	first.Release()
	first.Release()
	if _, mutationOk, _ := gate.AcquireWorkingTreeMutation(cwd); mutationOk {
		t.Fatal("duplicate release consumed another run's admission")
	}
	second.Release()

	releaseMutation, ok, _ := gate.AcquireWorkingTreeMutation(cwd)
	if !ok {
		t.Fatal("mutation admission was rejected after run admissions released")
	}
	if _, ok, _ := gate.AcquireRun(t.Context(), "ses_3", cwd); ok {
		t.Fatal("run admission crossed working-tree mutation")
	}
	releaseMutation()
	if admission, ok, _ := gate.AcquireRun(t.Context(), "ses_3", ""); !ok {
		t.Fatal("empty working tree must not require a claim")
	} else {
		admission.Release()
	}
}

func TestLiveRunHoldsItsSessionAgainstEveryWriter(t *testing.T) {
	gate := newTestGate(t)
	opening, ok, _ := gate.AcquireRun(t.Context(), "ses_1", "/repo")
	if !ok || !opening.Admit("run_1") {
		t.Fatal("admit run")
	}

	// A different working tree so only the single-writer rule can refuse.
	if _, ok, _ := gate.AcquireRun(t.Context(), "ses_1", "/elsewhere"); ok {
		t.Fatal("run admission crossed a live run on the same session")
	}
	if _, ok, _ := gate.AcquireSession(t.Context(), "ses_1"); ok {
		t.Fatal("session admission crossed a live run")
	}
	if !gate.ActiveSessions()["ses_1"] {
		t.Fatal("a live run left its session idle")
	}

	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("begin maintenance")
	}
	releaseMaintenance()
	admission, ok, _ := gate.AcquireRun(t.Context(), "ses_1", "/repo")
	if !ok {
		t.Fatal("a finished run left its session busy")
	}
	admission.Release()
}

func TestWaitRunStartableIncludesTerminalMaintenance(t *testing.T) {
	gate := newTestGate(t)
	opening, ok, _ := gate.AcquireRun(t.Context(), "ses_1", "/repo")
	if !ok || !opening.Admit("run_1") {
		t.Fatal("admit run")
	}
	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("begin maintenance")
	}

	done := make(chan error, 1)
	go func() { done <- gate.WaitRunStartable(t.Context(), "ses_1", "/repo") }()
	select {
	case err := <-done:
		t.Fatalf("WaitRunStartable returned inside maintenance: %v", err)
	default:
	}
	releaseMaintenance()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitRunStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitRunStartable did not observe maintenance release")
	}
}

func TestWaitRunStartableIncludesPendingRun(t *testing.T) {
	gate := newTestGate(t)
	opening, ok, _ := gate.AcquireRun(t.Context(), "ses_1", "/repo")
	if !ok {
		t.Fatal("acquire pending Run")
	}

	done := make(chan error, 1)
	go func() { done <- gate.WaitRunStartable(t.Context(), "ses_1", "/repo") }()
	select {
	case err := <-done:
		t.Fatalf("WaitRunStartable returned while Run was pending: %v", err)
	default:
	}
	opening.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitRunStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitRunStartable did not observe pending Run release")
	}
}

func TestWaitRunStartableIncludesWorkingTreeMutation(t *testing.T) {
	gate := newTestGate(t)
	release, ok, _ := gate.AcquireWorkingTreeMutation("/repo")
	if !ok {
		t.Fatal("acquire working-tree mutation")
	}

	done := make(chan error, 1)
	go func() { done <- gate.WaitRunStartable(t.Context(), "ses_1", "/repo") }()
	select {
	case err := <-done:
		t.Fatalf("WaitRunStartable returned inside working-tree mutation: %v", err)
	default:
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitRunStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitRunStartable did not observe working-tree mutation release")
	}
}

func TestWaitRunStartableIsContextBounded(t *testing.T) {
	gate := newTestGate(t)
	release, ok, _ := gate.AcquireSession(t.Context(), "ses_1")
	if !ok {
		t.Fatal("acquire session")
	}
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := gate.WaitRunStartable(ctx, "ses_1", "/repo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitRunStartable error = %v, want context canceled", err)
	}
}

func TestFailedIdentityResolutionDoesNotLeaveAdmissionHeld(t *testing.T) {
	cause := errors.New("filesystem failed")
	var resolveErr error = cause
	gate, err := NewGate(GateConfig{ResolveTree: func(path string) (string, error) { return path, resolveErr }, RetirementPending: noResourceRetirements})
	if err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := gate.AcquireRun(t.Context(), "ses_1", "/repo"); acquired || !errors.Is(err, cause) {
		t.Fatalf("Run acquisition = (%t, %v)", acquired, err)
	}
	if len(gate.ActiveSessions()) != 0 {
		t.Fatal("failed working-tree resolution retained Session ownership")
	}
	if release, acquired, err := gate.AcquireWorkingTreeMutation("/repo"); release != nil || acquired || !errors.Is(err, cause) {
		t.Fatalf("mutation acquisition = (%v, %t, %v)", release == nil, acquired, err)
	}
	if err := gate.WaitRunStartable(t.Context(), "ses_1", "/repo"); !errors.Is(err, cause) {
		t.Fatalf("startability = %v, want filesystem cause", err)
	}
	resolveErr = nil
	admission, acquired, err := gate.AcquireRun(t.Context(), "ses_1", "/repo")
	if err != nil || !acquired {
		t.Fatalf("acquire after repair: %t, %v", acquired, err)
	}
	admission.Release()
}

func TestForegroundAdmissionCanCancelWhileRecoveryHoldsSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := newTestGate(t)
		release, ok, err := gate.AcquireRecoverySession("ses_1")
		if err != nil || !ok {
			t.Fatalf("recovery admission = %t, %v", ok, err)
		}
		defer release()
		for _, acquire := range []func(context.Context) error{
			func(ctx context.Context) error {
				slot, _, err := gate.AcquireRun(ctx, "ses_1", "/repo")
				slot.Release()
				return err
			},
			func(ctx context.Context) error {
				release, _, err := gate.AcquireSession(ctx, "ses_1")
				if release != nil {
					release()
				}
				return err
			},
		} {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			cause := errors.New("caller stopped waiting")
			result := make(chan error, 1)
			go func() { result <- acquire(ctx) }()
			synctest.Wait()
			select {
			case err := <-result:
				t.Fatalf("foreground command did not wait for recovery: %v", err)
			default:
			}
			cancel(cause)
			synctest.Wait()
			if err := <-result; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("admission error = %v, want cancellation and cause", err)
			}
		}
		release()
		slot, ok, err := gate.AcquireRun(t.Context(), "ses_1", "/repo")
		defer slot.Release()
		if !ok || err != nil {
			t.Fatalf("canceled waiter retained admission: %t, %v", ok, err)
		}
	})
}

func TestRepeatedSessionReleaseCannotRetireItsSuccessor(t *testing.T) {
	gate := newTestGate(t)
	first, acquired, err := gate.AcquireSession(t.Context(), "ses_1")
	if err != nil || !acquired {
		t.Fatalf("first acquisition = %t, %v", acquired, err)
	}
	first()
	second, acquired, err := gate.AcquireSession(t.Context(), "ses_1")
	if err != nil || !acquired {
		t.Fatalf("second acquisition = %t, %v", acquired, err)
	}
	defer second()
	first()
	if release, acquired, err := gate.AcquireRecoverySession("ses_1"); release != nil || acquired || err != nil {
		t.Fatalf("repeated release retired the successor = %t, %v", acquired, err)
	}
	second()
	if gate.ActiveSessions()["ses_1"] {
		t.Fatal("finished owner retained the Session")
	}
}

func TestAdmissionChangesRetainReleaseOrdering(t *testing.T) {
	gate := newTestGate(t)
	first, ok, err := gate.AcquireSession(t.Context(), "ses_first")
	if err != nil || !ok {
		t.Fatalf("first admission = %t, %v", ok, err)
	}
	observed := gate.Changes()
	first()
	select {
	case <-observed:
	default:
		t.Fatal("release did not close the captured version")
	}
	next := gate.Changes()
	second, ok, err := gate.AcquireSession(t.Context(), "ses_second")
	if err != nil || !ok {
		t.Fatalf("second admission = %t, %v", ok, err)
	}
	first()
	select {
	case <-next:
		t.Fatal("repeated release consumed the next admission version")
	default:
	}
	second()
	select {
	case <-next:
	default:
		t.Fatal("successor release did not close its captured version")
	}
}

func TestRunAdmissionCannotReplaceAnotherSessionsLiveIdentity(t *testing.T) {
	gate := newTestGate(t)
	first, ok, err := gate.AcquireRun(t.Context(), "ses_first", "/first")
	if err != nil || !ok || !first.Admit("run_shared") {
		t.Fatalf("first admission: %t, %v", ok, err)
	}
	second, ok, err := gate.AcquireRun(t.Context(), "ses_second", "/second")
	if err != nil || !ok {
		t.Fatalf("second admission: %t, %v", ok, err)
	}
	if second.Admit("run_shared") {
		t.Fatal("a duplicate Run identity replaced another Session's reservation")
	}
	if sessions := gate.ActiveSessions(); !sessions["ses_first"] || !sessions["ses_second"] {
		t.Fatalf("refused admission lost an owner: %v", sessions)
	}
	second.Release()
	if sessions := gate.ActiveSessions(); !sessions["ses_first"] || sessions["ses_second"] {
		t.Fatalf("refused admission could not release its own pending reservation: %v", sessions)
	}
	release, ok := gate.BeginMaintenance("run_shared")
	if !ok {
		t.Fatal("duplicate admission retired the original live Run")
	}
	release()
	if sessions := gate.ActiveSessions(); len(sessions) != 0 {
		t.Fatalf("completed admissions retained ownership: %v", sessions)
	}
}

func noResourceRetirements(context.Context, string) (bool, error) { return false, nil }
