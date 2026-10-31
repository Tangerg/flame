package execution

import (
	"context"
	"errors"
	"iter"
	"path/filepath"
	"sync"
	"testing"
	"time"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
)

var testTreeStores sync.Map

type testExecutionTrees struct {
	stores       *persistencefixture.Stores
	mu           sync.Mutex
	publications map[string]runs.ResultPublication
}

func testTrees(t *testing.T) *testExecutionTrees {
	t.Helper()
	if value, found := testTreeStores.Load(t); found {
		return value.(*testExecutionTrees)
	}
	stores, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &testExecutionTrees{stores: stores, publications: make(map[string]runs.ResultPublication)}
	actual, loaded := testTreeStores.LoadOrStore(t, store)
	if loaded {
		if err := stores.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Cleanup(func() {
			testTreeStores.Delete(t)
			if err := stores.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	return actual.(*testExecutionTrees)
}
func newProductInteractionExecutor(t *testing.T, model chat.Model, pricing accounting.Pricing, config InteractionExecutorConfig) *InteractionExecutor {
	t.Helper()
	config.Lifetime = t.Context()
	config.BuildID = interactionTestBuildID
	var counter modeladapter.InputTokenCounter
	if value, ok := model.(modeladapter.InputTokenCounter); ok {
		counter = value
	}
	config.ChatResolver = interactionChatResolver(model, counter)
	executor, err := newProductPricedConfiguredInteractionExecutor(t, config, pricing)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func newProductConfiguredInteractionExecutor(t *testing.T, config InteractionExecutorConfig) (*InteractionExecutor, error) {
	return newProductPricedConfiguredInteractionExecutor(t, config, nil)
}

func newProductPricedConfiguredInteractionExecutor(t *testing.T, config InteractionExecutorConfig, pricing accounting.Pricing) (*InteractionExecutor, error) {
	t.Helper()
	stores := testTrees(t).stores
	resolver, err := modeladapter.NewObservedResolver(config.ChatResolver, stores.ModelInvocations, pricing)
	if err != nil {
		t.Fatal(err)
	}
	config.ChatResolver = resolver
	config.ModelEvidence = stores.ModelInvocations
	config.ExecutionTrees = stores.ExecutorCheckpoints
	return newTestConfiguredInteractionExecutor(t, config)
}

func newTestConfiguredInteractionExecutor(t *testing.T, config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if config.ModelEvidence == nil && config.ChatResolver != nil {
		db := testTrees(t).stores
		journal := db.ModelInvocations
		resolver, err := modeladapter.NewObservedResolver(config.ChatResolver, newSessionEndpointJournal(db), nil)
		if err != nil {
			t.Fatal(err)
		}
		config.ChatResolver = resolver
		config.ModelEvidence = journal
	}
	if config.Admission == nil {
		config.Admission = testTrees(t).stores.Admission
	}
	if config.ExecutionTrees == nil {
		config.ExecutionTrees = testTrees(t)
	}
	executor, err := NewInteractionExecutor(config)
	if err == nil {
		t.Cleanup(func() {
			executor.BeginShutdown()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := executor.AwaitShutdown(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	return executor, err
}

func (s *testExecutionTrees) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	return s.stores.ExecutorCheckpoints.LoadExecutionTree(ctx, sessionID, rootID)
}

func (s *testExecutionTrees) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	return s.stores.ExecutorCheckpoints.SaveExecutionTree(ctx, update)
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

// The execution harness owns Session facts in memory; its separate physical
// evidence database needs the same Session admission before recording a call.
// This Session-only model fixture does not simulate relational Run association;
// that boundary is verified with admitted SQLite Model/Tool owners.
type sessionEndpointJournal struct {
	*persistence.ModelInvocationStore
	db *persistencefixture.Stores
}

func newSessionEndpointJournal(db *persistencefixture.Stores) sessionEndpointJournal {
	return sessionEndpointJournal{ModelInvocationStore: db.ModelInvocations, db: db}
}

func (j sessionEndpointJournal) StartEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation, _ string) error {
	if invocation.SessionID != "" {
		sessions := j.db.Sessions
		if _, err := sessions.Get(ctx, invocation.SessionID); errors.Is(err, session.ErrNotFound) {
			if err := sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{ID: invocation.SessionID})); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return j.ModelInvocationStore.StartEndpointInvocation(ctx, invocation, "")
}

func newPricedObservedTestInteractionExecutor(t *testing.T, model chat.Model, pricing accounting.Pricing, config InteractionExecutorConfig) *InteractionExecutor {
	t.Helper()
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "priced-model-evidence.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	journal := db.ModelInvocations
	var counter modeladapter.InputTokenCounter
	if value, ok := model.(modeladapter.InputTokenCounter); ok {
		counter = value
	}
	resolver, err := modeladapter.NewObservedResolver(interactionChatResolver(model, counter), newSessionEndpointJournal(db), pricing)
	if err != nil {
		t.Fatal(err)
	}
	config.ModelEvidence = journal
	config.ChatResolver = resolver
	config.Lifetime = t.Context()
	config.BuildID = interactionTestBuildID
	executor, err := newTestConfiguredInteractionExecutor(t, config)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}
