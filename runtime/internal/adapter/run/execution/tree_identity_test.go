package execution

import (
	"context"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/scope/agent"
)

type treeCommitIdentityExpectation struct {
	id     string
	digest string
}

type scopeCommitCheckingStore struct{ runs.ExecutionTreeStore }

func (s *scopeCommitCheckingStore) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	expected, ok := ctx.Value(treeCommitIdentityExpectation{}).(treeCommitIdentityExpectation)
	if !ok {
		return fmt.Errorf("test: missing Scope commit expectation")
	}
	if update.Head.CommitID != expected.id || update.Head.CommitDigest != expected.digest {
		return fmt.Errorf("test: Runtime derived a second Scope commit identity: got (%q, %q), want (%q, %q)",
			update.Head.CommitID, update.Head.CommitDigest, expected.id, expected.digest)
	}
	return s.ExecutionTreeStore.SaveExecutionTree(ctx, update)
}

func (d *sqliteTreeCommitterDriver) ActivateTree(ctx context.Context, activation agent.TreeActivation) error {
	digest, err := activation.ContentDigest()
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, treeCommitIdentityExpectation{}, treeCommitIdentityExpectation{
		id: activation.Identity(), digest: digest.String(),
	})
	return d.interactionSession.ActivateTree(ctx, activation)
}

func (d *sqliteTreeCommitterDriver) CommitEffect(ctx context.Context, boundary agent.EffectBoundary) error {
	digest, err := boundary.ContentDigest()
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, treeCommitIdentityExpectation{}, treeCommitIdentityExpectation{
		id: boundary.Identity(), digest: digest.String(),
	})
	return d.interactionSession.CommitEffect(ctx, boundary)
}

func (d *sqliteTreeCommitterDriver) CommitCheckpoint(ctx context.Context, checkpoint agent.TreeCheckpoint) error {
	digest, err := checkpoint.ContentDigest()
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, treeCommitIdentityExpectation{}, treeCommitIdentityExpectation{
		id: checkpoint.Identity(), digest: digest.String(),
	})
	return d.interactionSession.CommitCheckpoint(ctx, checkpoint)
}

func TestScopeTreeCommitterRejectsInvalidBoundaryBeforeStorage(t *testing.T) {
	session := &interactionSession{}
	for _, test := range []struct {
		name   string
		commit func() error
	}{
		{"activation", func() error { return session.ActivateTree(t.Context(), agent.TreeActivation{}) }},
		{"effect", func() error { return session.CommitEffect(t.Context(), agent.EffectBoundary{}) }},
		{"checkpoint", func() error { return session.CommitCheckpoint(t.Context(), agent.TreeCheckpoint{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.commit(); err == nil {
				t.Fatal("invalid Scope boundary reached the host's storage lifecycle")
			}
		})
	}
}
