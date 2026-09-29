package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTreeCommitContextHasAnIndependentDeadline(t *testing.T) {
	type key struct{}
	caller, abandon := context.WithCancel(context.WithValue(t.Context(), key{}, "request"))
	lifetime := newInteractionLifetime(t.Context())
	t.Cleanup(lifetime.stopRelease)
	t.Cleanup(lifetime.stopExecution)
	t.Cleanup(lifetime.stopReconciling)
	abandon()
	lifetime.stopExecution()

	before := time.Now()
	ctx, cancel := lifetime.treeCommitContext(caller)
	defer cancel()
	deadline, bounded := ctx.Deadline()
	if !bounded || deadline.Before(before) || deadline.After(time.Now().Add(executionTreeCommitTimeout)) {
		t.Fatalf("tree commit has no independent bounded deadline: %v, %t", deadline, bounded)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("abandoning observation canceled persistence: %v", err)
	}
	if ctx.Value(key{}) != "request" {
		t.Fatal("tree commit lost the caller's context values")
	}

	lifetime.stopRelease()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal(ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("owner release did not cancel persistence")
	}
}

func TestTreeCommitConfirmationGetsANewBoundedContext(t *testing.T) {
	lifetime := newInteractionLifetime(t.Context())
	t.Cleanup(lifetime.stopRelease)
	t.Cleanup(lifetime.stopExecution)
	t.Cleanup(lifetime.stopReconciling)
	write, cancelWrite := lifetime.treeCommitContext(t.Context())
	cancelWrite()

	confirmation, cancel := lifetime.treeCommitContext(write)
	defer cancel()
	if err := confirmation.Err(); err != nil {
		t.Fatalf("confirmation inherited the failed write's cancellation: %v", err)
	}
	if _, bounded := confirmation.Deadline(); !bounded {
		t.Fatal("confirmation has no storage deadline")
	}
}

func TestInteractionLifetimeBindSeesAnAlreadyCanceledOwner(t *testing.T) {
	lifetime := newInteractionLifetime(t.Context())
	t.Cleanup(lifetime.stopRelease)
	t.Cleanup(lifetime.stopReconciling)
	lifetime.stopExecution()

	ctx, cancel := lifetime.bind(t.Context())
	defer cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("bind returned an active context for an already canceled owner")
	}
}

func TestTreeCommitContextSeesAnAlreadyReleasedOwner(t *testing.T) {
	lifetime := newInteractionLifetime(t.Context())
	t.Cleanup(lifetime.stopExecution)
	t.Cleanup(lifetime.stopReconciling)
	lifetime.stopRelease()

	ctx, cancel := lifetime.treeCommitContext(t.Context())
	defer cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("tree commit started after owner release")
	}
}
