package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTreeCommitPublicationBelongsToTheReleaseLifetime(t *testing.T) {
	type key struct{}
	caller, abandon := context.WithCancel(context.WithValue(t.Context(), key{}, "request"))
	lifetime := newInteractionLifetime(t.Context())
	t.Cleanup(lifetime.stopRelease)
	t.Cleanup(lifetime.stopExecution)
	t.Cleanup(lifetime.stopReconciling)
	abandon()
	lifetime.stopExecution()

	ctx, cancel := lifetime.publicationContext(caller)
	defer cancel()
	deadline, bounded := ctx.Deadline()
	if bounded {
		t.Fatalf("tree commit acquired an independent expiry: %v", deadline)
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

func TestTreeCommitConfirmationRetainsTheOriginalOwnerLifetime(t *testing.T) {
	lifetime := newInteractionLifetime(t.Context())
	t.Cleanup(lifetime.stopRelease)
	t.Cleanup(lifetime.stopExecution)
	t.Cleanup(lifetime.stopReconciling)
	write, cancelWrite := lifetime.publicationContext(t.Context())
	cancelWrite()

	confirmation, cancel := lifetime.publicationContext(write)
	defer cancel()
	if err := confirmation.Err(); err != nil {
		t.Fatalf("confirmation inherited the failed write's cancellation: %v", err)
	}
	if _, bounded := confirmation.Deadline(); bounded {
		t.Fatal("confirmation acquired an independent expiry")
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

	ctx, cancel := lifetime.publicationContext(t.Context())
	defer cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("tree commit started after owner release")
	}
}
