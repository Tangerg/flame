package runs

import (
	"context"
	"testing"
	"time"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type gatedRecoveryCatalog struct {
	*recoveryStoreStub
	reads   int
	claimed chan struct{}
	release chan struct{}
}

func (s *gatedRecoveryCatalog) ListNonTerminalRuns(ctx context.Context) ([]rundomain.Run, error) {
	s.reads++
	if s.reads == 1 {
		return s.runs, nil
	}
	close(s.claimed)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return nil, nil
	}
}

func TestNewRunWaitsForRecoveryToReleaseItsSessionProbe(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store := &gatedRecoveryCatalog{
		recoveryStoreStub: &recoveryStoreStub{runs: []rundomain.Run{testsupport.MustRestoreRun(rundomain.Snapshot{
			ID: "run_previous", SessionID: "ses_1", State: rundomain.Running, ActiveSegmentID: "seg_previous",
			CreatedAt: time.Now().UTC(), MessageMark: rundomain.UnknownMessageMark,
		})}}, claimed: make(chan struct{}), release: make(chan struct{}),
	}
	gate := testsupport.NewAdmissionGate()
	recovery, err := NewRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) { return true, nil }), gate, nil)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := recovery.Reconcile(ctx); finished <- err }()
	select {
	case <-store.claimed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	admitted := make(chan bool, 1)
	go func() {
		slot, ok, err := gate.AcquireRun(t.Context(), "ses_1", "/repo")
		if ok {
			slot.Release()
		}
		admitted <- ok && err == nil
	}()
	select {
	case ok := <-admitted:
		t.Errorf("foreground admission returned before recovery released its probe: acquired=%t", ok)
	case <-time.After(20 * time.Millisecond):
	}
	close(store.release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !t.Failed() {
		select {
		case ok := <-admitted:
			if !ok {
				t.Fatal("foreground run rejected after recovery")
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
