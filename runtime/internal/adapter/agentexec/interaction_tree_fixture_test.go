package agentexec

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

var testTreeStores sync.Map

type testExecutionTrees struct {
	mu           sync.Mutex
	heads        map[string]runs.ExecutionTreeHead
	publications map[string]runs.ResultPublication
}

func testTrees(t *testing.T) *testExecutionTrees {
	t.Helper()
	if value, found := testTreeStores.Load(t); found {
		return value.(*testExecutionTrees)
	}
	store := &testExecutionTrees{heads: make(map[string]runs.ExecutionTreeHead), publications: make(map[string]runs.ResultPublication)}
	actual, loaded := testTreeStores.LoadOrStore(t, store)
	if !loaded {
		t.Cleanup(func() { testTreeStores.Delete(t) })
	}
	return actual.(*testExecutionTrees)
}
func newTestConfiguredInteractionExecutor(t *testing.T, config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if config.ExecutionTrees == nil {
		config.ExecutionTrees = testTrees(t)
	}
	return NewInteractionExecutor(interactionTestCapabilities(t, config))
}
func (s *testExecutionTrees) LoadExecutionTree(_ context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, found := s.heads[rootID]
	if found && h.SessionID != sessionID {
		return runs.ExecutionTreeHead{}, false, errors.New("foreign tree")
	}
	return h, found, nil
}
func (s *testExecutionTrees) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.heads[update.Head.RootID]
	if h.Writer == update.Head.Writer && h.Digest == update.Head.Digest {
		return nil
	}
	if h.Writer != update.PreviousWriter || h.Digest != update.PreviousDigest {
		return errors.New("stale tree writer")
	}
	s.heads[update.Head.RootID] = update.Head
	return nil
}
func (s *testExecutionTrees) ExecutionResultCommitted(_ context.Context, _ string, publication runs.ResultPublication) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, found := s.publications[publication.ID]
	if found && stored != publication {
		return false, errors.New("result conflict")
	}
	return found, nil
}
func (s *testExecutionTrees) record(publication runs.ResultPublication) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publications[publication.ID] = publication
}

// Adapter-only harnesses expand one tree transaction into their existing fact
// assertions. Application/SQLite tests exercise the actual atomic transaction.
func observeTestInteraction(t *testing.T, executor *InteractionExecutor, ctx context.Context, ref runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	sequence, err := executor.Observe(ctx, ref)
	if err != nil {
		return nil, err
	}
	return func(yield func(runs.ExecutorEvent) bool) {
		for event := range sequence {
			commit, ok := event.Payload.(runs.ExecutionFactCommit)
			if !ok {
				if !yield(event) {
					return
				}
				continue
			}
			tree, ok := commit.Fact().(runs.ExecutionTreeSettled)
			if !ok {
				if !yield(event) {
					return
				}
				continue
			}
			var commitErr error
			for _, projected := range tree.Facts {
				request, receipt, err := runs.NewExecutionFactCommit(projected.Payload.(runs.ExecutionFact))
				if err != nil {
					commitErr = err
					break
				}
				if !yield(runs.ExecutorEvent{Member: projected.Member, Payload: request}) {
					commit.Complete(context.Canceled)
					return
				}
				if err := receipt.Await(ctx); err != nil {
					commitErr = err
					break
				}
			}
			if commitErr == nil {
				commitErr = executor.config.ExecutionTrees.SaveExecutionTree(ctx, tree.Update)
				if commitErr == nil {
					for _, projected := range tree.Facts {
						if results, ok := projected.Payload.(runs.ToolResultsCommitted); ok {
							testTrees(t).record(results.Publication)
						}
					}
				}
			}
			commit.Complete(commitErr)
		}
	}, nil
}
