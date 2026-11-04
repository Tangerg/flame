package execution

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
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
		return fmt.Errorf("test: persisted batch identity differs: got (%q, %q), want (%q, %q)",
			update.Head.CommitID, update.Head.CommitDigest, expected.id, expected.digest)
	}
	return s.ExecutionTreeStore.SaveExecutionTree(ctx, update)
}

func (d *sqliteScopeStore) Append(ctx context.Context, batch scoperun.Batch) error {
	payload, err := json.Marshal(batch, json.Deterministic(true))
	if err != nil {
		return err
	}
	digest := scoperun.ComputeDigest(payload).String()
	ctx = context.WithValue(ctx, treeCommitIdentityExpectation{}, treeCommitIdentityExpectation{
		id: digest, digest: digest,
	})
	return d.interactionSession.Append(ctx, batch)
}

func TestScopeStoreRejectsInvalidBatchBeforeStorage(t *testing.T) {
	session := &interactionSession{}
	if err := session.Append(t.Context(), scoperun.Batch{}); err == nil {
		t.Fatal("invalid Scope batch reached the host's storage lifecycle")
	}
}
