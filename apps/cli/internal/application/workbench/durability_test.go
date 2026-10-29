package workbench_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/statefile"
	agentrun "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	agentsession "github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// Physical publication faults are exercised by statefile. This port withholds
// confirmation of a real write so consumers must honor the reported boundary.
type uncertainPersistence struct {
	*statefile.Store
	remaining int
	state     filesystem.CommitState
	cause     error
}

func (p *uncertainPersistence) result(result filesystem.CommitResult) filesystem.CommitResult {
	if p.remaining > 0 {
		p.remaining--
		if p.remaining == 0 && result.Committed() {
			result.State = p.state
			result.Err = errors.Join(result.Err, p.cause)
		}
	}
	return result
}

func (p *uncertainPersistence) Replace(name string, body []byte) filesystem.CommitResult {
	return p.result(p.Store.Replace(name, body))
}

func (p *uncertainPersistence) Remove(name string) filesystem.CommitResult {
	return p.result(p.Store.Remove(name))
}

func openUncertainWorkbench(t *testing.T) (*workbench.Store, *uncertainPersistence, string) {
	t.Helper()
	directory := t.TempDir()
	storage, err := statefile.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	uncertain := &uncertainPersistence{Store: storage, state: filesystem.CommitVisible, cause: errors.New("directory confirmation failed")}
	store, err := workbench.Open(uncertain, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store, uncertain, directory
}

func reopenWorkbench(t *testing.T, store *workbench.Store, directory string) *workbench.Store {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	storage, err := statefile.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := workbench.Open(storage, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := reopened.DispatchReady(); err != nil {
		t.Fatal(err)
	}
	return reopened
}

func requireWriteOutcome(t *testing.T, err, cause error, committed bool) {
	t.Helper()
	write, ok := errors.AsType[*workbench.WriteError](err)
	if !ok || !errors.Is(err, cause) || write.Result.Committed() != committed {
		t.Fatalf("write outcome = %v, want committed=%t and original cause %v", err, committed, cause)
	}
}

func requireFenced(t *testing.T, err, cause error) {
	t.Helper()
	if !errors.Is(err, cause) || !mutation.OutcomeUnknown(err) {
		t.Fatalf("unconfirmed identity was not fenced: %v", err)
	}
}

func TestVisibleWritePublishesMemoryAndReopenConfirmsIt(t *testing.T) {
	store, storage, directory := openUncertainWorkbench(t)
	if err := store.SaveDraft("session", authoring.Message{Text: "before"}); err != nil {
		t.Fatal(err)
	}
	storage.remaining = 1
	after := authoring.Message{Text: "after"}
	requireWriteOutcome(t, store.SaveDraft("session", after), storage.cause, true)
	if draft, _ := store.Draft("session"); !draft.Equal(after) {
		t.Fatalf("visible draft = %+v", draft)
	}
	requireFenced(t, store.DispatchReady(), storage.cause)
	requireWriteOutcome(t, store.SaveDraft("session", authoring.Message{Text: "not attempted"}), storage.cause, false)
	reopened := reopenWorkbench(t, store, directory)
	if draft, _ := reopened.Draft("session"); !draft.Equal(after) {
		t.Fatalf("reopened draft = %+v", draft)
	}
	if err := reopened.SaveDraft("session", authoring.Message{Text: "confirmed"}); err != nil {
		t.Fatal(err)
	}
}

func TestQueueAdoptsVisibleWriteButRestoresAnUnattemptedSuccessor(t *testing.T) {
	store, storage, directory := openUncertainWorkbench(t)
	queue, err := workbench.NewQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	original := mutation.NewCommandID()
	storage.remaining = 1
	_, err = queue.EnqueueCommand(original, "session", authoring.Message{Text: "original"}, authoring.RunOptions{})
	requireWriteOutcome(t, err, storage.cause, true)
	_, err = queue.EnqueueCommand(mutation.NewCommandID(), "session", authoring.Message{Text: "not attempted"}, authoring.RunOptions{})
	requireWriteOutcome(t, err, storage.cause, false)
	if entries := queue.Snapshot("session").Entries; len(entries) != 1 || entries[0].CommandID != original {
		t.Fatalf("FIFO disagrees with visible owner: %+v", entries)
	}
	reopened := reopenWorkbench(t, store, directory)
	if pending := reopened.PendingRuns("session"); len(pending) != 1 || pending[0].Command.CommandID != original {
		t.Fatalf("reopen changed command identity: %+v", pending)
	}
}

func TestQueueSettlementAdoptsOnlyThePublishedOutboxTransition(t *testing.T) {
	for _, operation := range []string{"history", "retirement", "requeue"} {
		t.Run(operation, func(t *testing.T) {
			store, storage, _ := openUncertainWorkbench(t)
			queue, err := workbench.NewQueue(store)
			if err != nil {
				t.Fatal(err)
			}
			original := mutation.NewCommandID()
			if _, err := queue.EnqueueCommand(original, "session", authoring.Message{Text: "original"}, authoring.RunOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, ok := queue.BeginDispatch("session"); !ok {
				t.Fatal("missing opening reservation")
			}
			if err := store.MarkPendingRunDispatching("session", original, authoring.UnprotectedGuard(), nil); err != nil {
				t.Fatal(err)
			}
			storage.remaining = 1
			if operation == "retirement" {
				storage.remaining = 2
			}
			if operation == "requeue" {
				err = queue.RequeueDispatch("session", original)
			} else {
				_, _, err = queue.SettleDispatch("session", original)
			}
			requireWriteOutcome(t, err, storage.cause, true)
			pending, entries := store.PendingRuns("session"), queue.Snapshot("session").Entries
			if len(pending) != len(entries) {
				t.Fatalf("published outbox=%+v FIFO=%+v", pending, entries)
			}
			if len(pending) > 0 && pending[0].Command.CommandID != entries[0].CommandID {
				t.Fatal("FIFO retained superseded identity")
			}
			_, reserved := queue.Dispatching("session")
			if reserved != (operation == "history") {
				t.Fatalf("reservation=%t after %s", reserved, operation)
			}
		})
	}
}

type durabilityRuntime struct {
	*runtimefixture.Runtime
	receipt                                     protocol.RunAcceptance
	queries                                     []protocol.GetRunAcceptanceRequest
	starts, resumes, steers, deletes, rollbacks int
}

func (r *durabilityRuntime) GetRunAcceptance(_ context.Context, query protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error) {
	r.queries = append(r.queries, query)
	return r.receipt, nil
}
func (r *durabilityRuntime) StartRun(context.Context, authoring.StartRun) (conversation.SegmentStream, error) {
	r.starts++
	return conversation.SegmentStream{}, nil
}
func (r *durabilityRuntime) ResumeRun(context.Context, conversation.ResumeRun) (conversation.SegmentStream, error) {
	r.resumes++
	return conversation.SegmentStream{}, nil
}
func (r *durabilityRuntime) SteerRun(context.Context, authoring.SteerRun) (protocol.SteerRunResponse, error) {
	r.steers++
	return protocol.SteerRunResponse{}, nil
}
func (r *durabilityRuntime) DeleteSession(context.Context, conversation.DeleteSession) error {
	r.deletes++
	return nil
}
func (r *durabilityRuntime) RollbackSession(context.Context, conversation.RollbackSession) (conversation.RollbackResult, error) {
	r.rollbacks++
	return conversation.RollbackResult{}, nil
}

func TestUnconfirmedOpeningAllowsReceiptReadsButFencesDelivery(t *testing.T) {
	for _, operation := range []protocol.RunCommandOperation{protocol.RunCommandStart, protocol.RunCommandResume} {
		t.Run(string(operation), func(t *testing.T) {
			store, storage, directory := openUncertainWorkbench(t)
			now := time.Now()
			replay := policy(t, "runtime", &now)
			guard, err := replay.NewGuard()
			if err != nil {
				t.Fatal(err)
			}
			id := mutation.NewCommandID()
			runtime := &durabilityRuntime{receipt: protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}}
			var deliver func(*workbench.Store) (conversation.SegmentStream, error)
			if operation == protocol.RunCommandStart {
				if err := store.StagePendingRun(workbench.PendingRun{
					State: workbench.PendingRunQueued, Replay: authoring.UnprotectedGuard(), CancelReplay: authoring.UnprotectedGuard(),
					Command: authoring.StartRun{CommandID: id, SessionID: "session", Message: authoring.Message{Text: "original"}},
				}); err != nil {
					t.Fatal(err)
				}
				storage.remaining = 1
				requireWriteOutcome(t, store.MarkPendingRunDispatching("session", id, guard, nil), storage.cause, true)
				deliver = func(current *workbench.Store) (conversation.SegmentStream, error) {
					return agentrun.OpenPendingStart(t.Context(), runtime, current, current.PendingRuns("session")[0], replay)
				}
			} else {
				approval := conversation.Approval{
					RunID: "run_original", ItemID: "approval", Title: "Approve",
					Tool: &conversation.ToolCall{Kind: protocol.ToolCategoryRead, Name: "read", Status: conversation.ToolRunning},
				}
				pending := workbench.PendingResume{
					Command: conversation.ResumeRun{
						CommandID: id, RunID: approval.RunID,
						Answers: []conversation.InterruptAnswer{{ItemID: approval.ItemID, Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny}}},
					},
					Interrupts: []conversation.Interrupt{approval}, Replay: guard,
				}
				storage.remaining = 1
				requireWriteOutcome(t, store.StagePendingResume("session", pending, nil), storage.cause, true)
				deliver = func(current *workbench.Store) (conversation.SegmentStream, error) {
					pending, _ := current.PendingResume("session")
					return agentrun.OpenPendingResume(t.Context(), runtime, current, pending, replay)
				}
			}
			for range 2 {
				_, err := deliver(store)
				requireFenced(t, err, storage.cause)
			}
			runtime.receipt = protocol.RunAcceptance{Type: protocol.RunAcceptanceAccepted, SessionID: "session", RunID: "run_original", SegmentID: "seg_original"}
			if operation == protocol.RunCommandStart {
				runtime.receipt.UserItemID = "item_original"
			}
			if stream, err := deliver(store); err != nil || stream.RunID != "run_original" {
				t.Fatalf("read-only reconciliation: %+v %v", stream, err)
			}
			if runtime.starts != 0 || runtime.resumes != 0 {
				t.Fatal("unconfirmed record crossed first-send boundary")
			}
			for _, query := range runtime.queries {
				if query.Key != string(id) || query.Namespace != guard.Namespace() || query.Operation != operation {
					t.Fatalf("reconciliation changed identity: %+v", query)
				}
			}
			reopened := reopenWorkbench(t, store, directory)
			runtime.receipt = protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}
			if _, err := deliver(reopened); err != nil {
				t.Fatal(err)
			}
			if runtime.starts+runtime.resumes != 1 {
				t.Fatal("confirmed command did not dispatch")
			}
		})
	}
}

func TestUnconfirmedSteerAndCancellationRetainTheirIdentity(t *testing.T) {
	t.Run("steer", func(t *testing.T) {
		store, storage, _ := openUncertainWorkbench(t)
		now := time.Now()
		replay := policy(t, "runtime", &now)
		source := authoring.Message{Text: "/steer original"}
		if err := store.SaveDraft("session", source); err != nil {
			t.Fatal(err)
		}
		command := authoring.SteerRun{CommandID: mutation.NewCommandID(), RunID: "run_original", SegmentID: "seg_original", Message: authoring.Message{Text: "original"}}
		storage.remaining = 1
		_, err := agentrun.StageSteer(store, "session", command, source, replay, nil)
		requireWriteOutcome(t, err, storage.cause, true)
		runtime := new(durabilityRuntime)
		for range 2 {
			_, err := agentrun.RecoverSteers(t.Context(), runtime, store, replay, mutation.AcknowledgementBackoff())
			requireFenced(t, err, storage.cause)
		}
		pending, found := store.PendingSteer("session")
		if !found || pending.CommandID() != command.CommandID || runtime.steers != 0 {
			t.Fatal("steer changed identity or dispatched")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		store, storage, _ := openUncertainWorkbench(t)
		id := mutation.NewCommandID()
		if err := store.StagePendingRun(workbench.PendingRun{
			State: workbench.PendingRunQueued, Replay: authoring.UnprotectedGuard(), CancelReplay: authoring.UnprotectedGuard(),
			Command: authoring.StartRun{CommandID: id, SessionID: "session", Message: authoring.Message{Text: "original"}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.MarkPendingRunDispatching("session", id, authoring.UnprotectedGuard(), nil); err != nil {
			t.Fatal(err)
		}
		storage.remaining = 1
		_, err := store.MarkPendingRunCanceling("session", id, authoring.UnprotectedGuard())
		requireWriteOutcome(t, err, storage.cause, true)
		retained := store.PendingRuns("session")[0].CancelCommandID
		retried, err := store.MarkPendingRunCanceling("session", id, authoring.UnprotectedGuard())
		if err != nil || retained == "" || retained != retried {
			t.Fatalf("cancellation identity changed: %q %q %v", retained, retried, err)
		}
		requireFenced(t, store.CancellationReady(retained), storage.cause)
		if err := store.CancellationReady(mutation.NewCommandID()); err != nil {
			t.Fatal("independent live cancellation was blocked")
		}
	})
}

func TestUnconfirmedSessionCommandsCannotBypassStagingOnRetry(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		store, storage, _ := openUncertainWorkbench(t)
		now := time.Now()
		replay := policy(t, "runtime", &now)
		runtime := &durabilityRuntime{Runtime: runtimefixture.New()}
		storage.remaining = 1
		_, err := agentsession.Delete(t.Context(), runtime, store, "ses_demo_1", replay, mutation.AcknowledgementBackoff())
		requireWriteOutcome(t, err, storage.cause, true)
		original, found := store.PendingSessionDeletion("ses_demo_1")
		if !found {
			t.Fatal("visible deletion intent was lost")
		}
		result, err := agentsession.Delete(t.Context(), runtime, store, "ses_demo_1", replay, mutation.AcknowledgementBackoff())
		requireFenced(t, err, storage.cause)
		if result.Outcome != mutation.Unknown || result.Request.CommandID != original.CommandID || runtime.deletes != 0 {
			t.Fatal("deletion retried with unconfirmed identity")
		}
	})
	for _, scope := range []protocol.RestoreType{protocol.RestoreHistory, protocol.RestoreFiles} {
		t.Run("rollback_"+string(scope), func(t *testing.T) {
			store, storage, _ := openUncertainWorkbench(t)
			now := time.Now()
			replay := policy(t, "runtime", &now)
			runtime := &durabilityRuntime{Runtime: runtimefixture.New()}
			snapshot, err := runtime.GetSession(t.Context(), "ses_demo_1")
			if err != nil {
				t.Fatal(err)
			}
			preview, err := agentsession.PreviewRollback(snapshot, conversation.RollbackSession{SessionID: snapshot.Session.ID, ToRunID: snapshot.Runs[0].ID, Scope: scope})
			if err != nil {
				t.Fatal(err)
			}
			storage.remaining = 1
			_, err = agentsession.Rollback(t.Context(), runtime, store, preview, replay, mutation.AcknowledgementBackoff())
			requireWriteOutcome(t, err, storage.cause, true)
			original := store.PendingSessionRollbacks()[0].CommandID
			err = agentsession.RecoverRollbacks(t.Context(), runtime, store, replay, mutation.AcknowledgementBackoff())
			requireFenced(t, err, storage.cause)
			if pending := store.PendingSessionRollbacks(); len(pending) != 1 || pending[0].CommandID != original || runtime.rollbacks != 0 {
				t.Fatal("rollback lost original intent or dispatched")
			}
		})
	}
}

func TestInstalledCommandWaitsForReopenWithoutPreparingAnotherIntent(t *testing.T) {
	store, storage, directory := openUncertainWorkbench(t)
	now := time.Now()
	replay := policy(t, "runtime", &now)
	preparations, calls := 0, 0
	prepare := func(context.Context) (protocol.CallPluginRequest, error) { preparations++; return request(t), nil }
	var delivered authoring.CommandID
	caller := callerFunc(func(_ context.Context, _ protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		calls++
		delivered = id
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	})
	storage.remaining = 1
	requireWriteOutcome(t, execute(t.Context(), caller, store, "delete", replay, prepare), storage.cause, true)
	original, found := store.PendingInstalledCommand("delete")
	if !found {
		t.Fatal("visible installed intent was lost")
	}
	requireFenced(t, execute(t.Context(), caller, store, "delete", replay, prepare), storage.cause)
	if preparations != 1 || calls != 0 {
		t.Fatalf("preparations=%d calls=%d", preparations, calls)
	}
	reopened := reopenWorkbench(t, store, directory)
	if err := execute(t.Context(), caller, reopened, "delete", replay, prepare); err != nil {
		t.Fatal(err)
	}
	if delivered != original.CommandID || preparations != 1 || calls != 1 {
		t.Fatal("reopen changed the original installed command")
	}
}

func TestStashPostCommitFailureNeverCompensatesVisibleProgress(t *testing.T) {
	for _, state := range []filesystem.CommitState{filesystem.CommitVisible, filesystem.CommitDurable} {
		for _, cut := range []int{1, 2, 3, 4} {
			t.Run(fmt.Sprintf("state_%d/cut_%d", state, cut), func(t *testing.T) {
				store, storage, directory := openUncertainWorkbench(t)
				message := authoring.Message{Text: "original"}
				if err := store.SaveDraft("session", message); err != nil {
					t.Fatal(err)
				}
				storage.state, storage.remaining = state, cut
				_, err := store.StashDraft("session", message)
				requireWriteOutcome(t, err, storage.cause, true)
				if cut >= 2 && len(store.Stashes()) != 1 {
					t.Fatal("committed stash was compensated")
				}
				if _, present := store.Draft("session"); cut >= 3 && present {
					t.Fatal("committed source retirement was reversed")
				}
				reopened := reopenWorkbench(t, store, directory)
				if _, present := reopened.Draft("session"); present {
					t.Fatal("recovery restored transferred draft")
				}
				if stashes := reopened.Stashes(); len(stashes) != 1 || !stashes[0].Message.Equal(message) {
					t.Fatalf("recovered stashes = %+v", stashes)
				}
			})
		}
	}
}

func TestSessionRetirementNeverHidesPostCommitCleanupFailures(t *testing.T) {
	for _, cut := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			store, storage, directory := openUncertainWorkbench(t)
			if err := store.SaveDraft("session", authoring.Message{Text: "retire me"}); err != nil {
				t.Fatal(err)
			}
			storage.remaining = cut
			requireWriteOutcome(t, store.RetireSessionState("session"), storage.cause, true)
			if _, present := store.Draft("session"); present {
				t.Fatal("confirmed tombstone left the old draft live")
			}
			requireFenced(t, store.DispatchReady(), storage.cause)
			reopened := reopenWorkbench(t, store, directory)
			if _, present := reopened.Draft("session"); present {
				t.Fatal("retirement recovery revived old draft")
			}
			if len(reopened.PendingSessionDeletions()) != 0 {
				t.Fatal("confirmed cleanup retained its tombstone")
			}
		})
	}
}
