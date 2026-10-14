package execution

import (
	"context"
	"database/sql"
	"errors"
	"iter"
	"path/filepath"
	"sync"
	"testing"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

var testTreeStores sync.Map

type testExecutionTrees struct {
	mu           sync.Mutex
	heads        map[string]runs.ExecutionTreeHead
	publications map[string]runs.ResultPublication
	commits      map[string]string
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
	if config.ModelEvidence == nil && config.ChatResolver != nil {
		db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "model-evidence.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		journal := sqlite.NewModelInvocationStore(db)
		resolver, err := modeladapter.NewObservedResolver(config.ChatResolver, fixtureEndpointJournal{ModelInvocationStore: journal, db: db}, nil)
		if err != nil {
			t.Fatal(err)
		}
		config.ChatResolver = resolver
		config.ModelEvidence = journal
	}
	if config.Admission == nil {
		config.Admission = plugins.NewAdmission()
	}
	if config.InstallationCommands == nil {
		config.InstallationCommands = emptyInstallationCommands{}
	}
	if config.ExecutionTrees == nil {
		config.ExecutionTrees = testTrees(t)
	}
	return NewInteractionExecutor(config)
}

type emptyInstallationCommands struct{}

func (emptyInstallationCommands) PendingCommandDependencies(context.Context) ([]plugin.Dependency, error) {
	return nil, nil
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
	if err := update.Validate(); err != nil {
		return err
	}
	h := s.heads[update.Head.RootID]
	if s.commits == nil {
		s.commits = make(map[string]string)
	}
	key := update.Head.RootID + "|" + update.Head.CommitID
	if h.SameCommit(update.Head) {
		return nil
	}
	if _, exists := s.commits[key]; exists {
		return errors.New("historical commit")
	}
	if h.Writer == "" && update.Head.Sequence != 1 || h.Writer != "" && h.Writer != update.Head.Writer && update.Head.Sequence != 0 || h.Writer == update.Head.Writer && update.Head.Sequence != h.Sequence+1 {
		return errors.New("invalid commit sequence")
	}
	if h.Writer != update.PreviousWriter || h.Digest != update.PreviousDigest {
		return errors.New("stale tree writer")
	}
	s.heads[update.Head.RootID] = update.Head
	s.commits[key] = update.Head.CommitDigest
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

// The execution harness owns Session facts in memory; its separate physical
// evidence database needs the same Session admission before recording a call.
// This Session-only model fixture does not simulate relational Run association;
// that boundary is verified with admitted SQLite Model/Tool owners.
type fixtureEndpointJournal struct {
	*sqlite.ModelInvocationStore
	db *sql.DB
}

func (j fixtureEndpointJournal) StartEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation, _ string) error {
	if invocation.SessionID != "" {
		sessions := sqlite.NewSessionStore(j.db)
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
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "priced-model-evidence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	journal := sqlite.NewModelInvocationStore(db)
	var counter modeladapter.InputTokenCounter
	if value, ok := model.(modeladapter.InputTokenCounter); ok {
		counter = value
	}
	resolver, err := modeladapter.NewObservedResolver(interactionChatResolver(model, counter), fixtureEndpointJournal{ModelInvocationStore: journal, db: db}, pricing)
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
