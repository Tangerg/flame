package agentexec

import (
	"testing"
	"time"
)

func TestInteractionLifetimeRegistersReconcilersBeforeAwaiterRuns(t *testing.T) {
	lifetime := newInteractionLifetime(t.Context())
	reconcilerStarted := make(chan struct{}, 2)
	awaiterJoined := make(chan struct{})

	lifetime.start(
		func() {
			lifetime.stopReconciling()
			lifetime.reconcilers.Wait()
			close(awaiterJoined)
		},
		func() {
			reconcilerStarted <- struct{}{}
			<-lifetime.reconciling.Done()
		},
		func() {
			reconcilerStarted <- struct{}{}
			<-lifetime.reconciling.Done()
		},
	)

	select {
	case <-awaiterJoined:
	case <-time.After(time.Second):
		t.Fatal("awaiter did not join the registered reconcilers")
	}
	for range 2 {
		select {
		case <-reconcilerStarted:
		default:
			t.Fatal("awaiter completed before every reconciler started")
		}
	}
	lifetime.workers.Wait()
}
