package ownership

import (
	"context"
	"errors"
	"testing"
	"time"
)

type admissionBackendStub struct {
	sessionErr, treeErr error
	released            int
}

func (s *admissionBackendStub) TrySession(string) (Lease, bool, error) {
	if s.sessionErr != nil {
		return nil, false, s.sessionErr
	}
	return admissionLeaseFunc(func() { s.released++ }), true, nil
}

func (s *admissionBackendStub) TryWorkingTree(string, bool) (Lease, bool, error) {
	if s.treeErr != nil {
		return nil, false, s.treeErr
	}
	return admissionLeaseFunc(func() {}), true, nil
}

func newTestGate(t *testing.T) *Gate {
	t.Helper()
	gate, err := NewGate(&admissionBackendStub{})
	if err != nil {
		t.Fatal(err)
	}
	return gate
}

func TestNewGateRequiresOwnership(t *testing.T) {
	var typedNil *admissionBackendStub
	for _, backend := range []AdmissionBackend{nil, typedNil} {
		if gate, err := NewGate(backend); err == nil || gate != nil {
			t.Fatalf("NewGate(%T) = %#v, %v", backend, gate, err)
		}
	}
}

func TestGateHoldsSessionThroughMaintenance(t *testing.T) {
	gate := newTestGate(t)
	opening, ok, _ := gate.AcquireRun("ses_1", "/repo")
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
	if _, sessionOk, _ := gate.AcquireSession("ses_1"); sessionOk {
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

	first, ok, _ := gate.AcquireRun("ses_1", cwd)
	if !ok {
		t.Fatal("first run admission was rejected")
	}
	second, ok, _ := gate.AcquireRun("ses_2", cwd)
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
	if _, ok, _ := gate.AcquireRun("ses_3", cwd); ok {
		t.Fatal("run admission crossed working-tree mutation")
	}
	releaseMutation()
	if admission, ok, _ := gate.AcquireRun("ses_3", ""); !ok {
		t.Fatal("empty working tree must not require a claim")
	} else {
		admission.Release()
	}
}

func TestWaitRunStartableIncludesTerminalMaintenance(t *testing.T) {
	gate := newTestGate(t)
	opening, ok, _ := gate.AcquireRun("ses_1", "/repo")
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
	opening, ok, _ := gate.AcquireRun("ses_1", "/repo")
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
	release, ok, _ := gate.AcquireSession("ses_1")
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

type admissionLeaseFunc func()

func (f admissionLeaseFunc) Release() { f() }

func TestFailedOwnershipDoesNotLeaveAdmissionHeld(t *testing.T) {
	cause := errors.New("filesystem failed")
	backend := &admissionBackendStub{sessionErr: cause}
	gate, err := NewGate(backend)
	if err != nil {
		t.Fatal(err)
	}
	if release, acquired, err := gate.AcquireSession("ses_1"); release != nil || acquired || !errors.Is(err, cause) {
		t.Fatalf("Session acquisition = (%v, %t, %v)", release == nil, acquired, err)
	}
	backend.sessionErr = nil
	backend.treeErr = cause
	if _, acquired, err := gate.AcquireRun("ses_1", "/repo"); acquired || !errors.Is(err, cause) {
		t.Fatalf("Run acquisition = (%t, %v)", acquired, err)
	}
	if backend.released != 1 || len(gate.ActiveSessions()) != 0 {
		t.Fatal("failed working-tree acquisition retained Session ownership")
	}
	if release, acquired, err := gate.AcquireWorkingTreeMutation("/repo"); release != nil || acquired || !errors.Is(err, cause) {
		t.Fatalf("mutation acquisition = (%v, %t, %v)", release == nil, acquired, err)
	}
	backend.treeErr = nil
	admission, acquired, err := gate.AcquireRun("ses_1", "/repo")
	if err != nil || !acquired {
		t.Fatalf("acquire after repair: %t, %v", acquired, err)
	}
	admission.Release()
}
