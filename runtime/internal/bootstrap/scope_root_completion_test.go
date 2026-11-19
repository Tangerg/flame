package bootstrap

import (
	"context"
	"database/sql/driver"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

const (
	rootCompletionCrashPath = "FLAME_TEST_ROOT_COMPLETION_CRASH_PATH"
	rootCompletionCrashCut  = "FLAME_TEST_ROOT_COMPLETION_CRASH_CUT"
	rootCompletionTx        = "transaction"
	rootCompletionMaintain  = "maintenance"
	rootCompletionCancel    = "cancellation-reserved"
	rootCompletionLateClaim = "cancellation-late-reservation"
	rootCompletionReply     = "Original completed reply."
)

type crashAtRootMaintenance struct{}

func (crashAtRootMaintenance) Maintain(context.Context, executionadapter.RunMaintenanceInput) executionadapter.RunMaintenanceResult {
	os.Exit(commandCrashExit)
	return executionadapter.RunMaintenanceResult{}
}

type crashAfterLateCancellationClaim struct {
	operation.ReplayStore
	entered chan<- struct{}
	proceed <-chan struct{}
}

func (g crashAfterLateCancellationClaim) Claim(ctx context.Context, claim operation.ReplayClaim) (operation.ReplayRecord, bool, error) {
	close(g.entered)
	<-g.proceed
	record, claimed, err := g.ReplayStore.Claim(ctx, claim)
	if err == nil {
		os.Exit(commandCrashExit)
	}
	return record, claimed, err
}

func TestScopeRootCompletionCrashHelper(t *testing.T) {
	path := os.Getenv(rootCompletionCrashPath)
	if path == "" {
		return
	}
	cut := os.Getenv(rootCompletionCrashCut)
	if cut == rootCompletionTx {
		err := modernsqlite.RegisterScalarFunction("test_exit_during_root_completion", 1,
			func(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
				payload, ok := args[0].([]byte)
				if !ok {
					return nil, errors.New("the Run transaction had no Scope head")
				}
				log, err := waitingChildScopeLog(payload)
				if err != nil || !scopeRootCompleted(log) {
					return nil, errors.Join(err, errors.New("the Run transaction did not see the original Scope completion"))
				}
				os.Exit(commandCrashExit)
				return nil, nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}

	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	reply := newReplyStub(rootCompletionReply)
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		select {
		case <-release:
			return reply.Call(ctx, request)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})}
	cfg := protocolRuntimeConfig(t, stores, model)
	if cut == rootCompletionMaintain || cut == rootCompletionCancel {
		cfg.maintenance = crashAtRootMaintenance{}
	}
	host := assembleTestRuntime(t, cfg)
	defer host.Close()
	api, err := protocolHandler(host, home)
	if err != nil {
		t.Fatal(err)
	}
	var replayStore operation.ReplayStore = stores.Idempotency
	reserved, submit := make(chan struct{}), make(chan struct{})
	releaseSubmit := sync.OnceFunc(func() { close(submit) })
	defer releaseSubmit()
	switch cut {
	case rootCompletionCancel:
		replayStore = cancellationReservationGate{stores.Idempotency, activeRootCancelKey, reserved, submit}
	case rootCompletionLateClaim:
		replayStore = crashAfterLateCancellationClaim{stores.Idempotency, reserved, submit}
	}
	if err := startTestRuntimeWithReplayStore(t, host, api, stores, replayStore); err != nil {
		t.Fatal(err)
	}
	if cut == rootCompletionTx {
		questionCrashDatabase(t, home, `CREATE TRIGGER crash_root_completion BEFORE UPDATE OF outcome ON runs
			WHEN NEW.outcome = 'completed'
			BEGIN SELECT test_exit_during_root_completion(
				(SELECT payload FROM execution_trees WHERE session_id = NEW.session_id)
			); END`)
	}
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "root completion crash",
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Complete this Run."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	cancelReply := make(chan protocol.OperationResult, 1)
	if cut == rootCompletionCancel || cut == rootCompletionLateClaim {
		request := protocol.CancelRunRequest{RunID: opened.RunID, Reason: activeRootCancelReason}
		go func() {
			cancelReply <- host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, protocol.CallMetadata{IdempotencyKey: activeRootCancelKey})
		}()
		waitForSignal(t, reserved, "the original cancellation reservation cut")
	}
	close(release)
	for _, err := range events {
		if err != nil {
			t.Fatal(err)
		}
	}
	if cut == rootCompletionLateClaim {
		releaseSubmit()
		select {
		case reply := <-cancelReply:
			t.Fatalf("the late cancellation did not reach its commit cut: %+v", reply)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	t.Fatal("the Run never reached the completion crash cut")
}

func TestScopeRootCompletionIsAtomicAfterProcessExit(t *testing.T) {
	for _, cut := range []string{rootCompletionTx, rootCompletionMaintain, rootCompletionCancel, rootCompletionLateClaim} {
		t.Run(cut, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("FLAME_HOME", home)
			path := filepath.Join(t.TempDir(), "original-run.json")
			crashCtx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestScopeRootCompletionCrashHelper$")
			child.Env = append(os.Environ(), rootCompletionCrashPath+"="+path, rootCompletionCrashCut+"="+cut)
			out, err := child.CombinedOutput()
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
				t.Fatalf("the original Run did not reach its completion cut: %v\n%s", err, out)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var original protocol.StartRunResponse
			if err := json.Unmarshal(raw, &original); err != nil {
				t.Fatal(err)
			}
			want := protocol.OutcomeCompleted
			if cut == rootCompletionTx {
				questionCrashDatabase(t, home, "DROP TRIGGER crash_root_completion")
				want = protocol.OutcomeLost
			}
			originalCall := assertRootCompletionAtCrash(t, home, original, cut)
			var dispatched atomic.Int32
			model := delegateRestartModel{chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				dispatched.Add(1)
				return nil, errors.New("recovery must not dispatch a new model call")
			})}
			host, api := openProtocolRuntime(t, model)
			defer host.Close()
			ctx := protocolLifecycleContext(t.Context())
			run, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: original.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != protocol.RunStatusFinished || run.Outcome == nil || run.Outcome.Type != want {
				t.Errorf("recovered original Run: status = %s, outcome = %+v, want %s", run.Status, run.Outcome, want)
			}
			calls, err := api.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: original.RunID})
			if err != nil || len(calls.Data) != 1 || calls.Data[0].CallID != originalCall.CallID ||
				calls.Data[0].SegmentID != original.SegmentID || calls.Data[0].State != protocol.ModelInvocationCompleted ||
				!calls.Data[0].StartedAt.Equal(originalCall.StartedAt) || !calls.Data[0].SettledAt.Equal(originalCall.FinishedAt) {
				t.Fatalf("recovery changed the original completed model call: %+v, %v", calls, err)
			}
			items, err := api.ListItems(ctx, protocol.ListItemsRequest{
				Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: original.RunID},
			})
			if err != nil {
				t.Fatal(err)
			}
			var answers int
			for _, item := range items.Data {
				if item.Type != protocol.ItemTypeAgentMessage {
					continue
				}
				answers++
				if item.Status != protocol.ItemStatusCompleted || len(item.Content) != 1 || item.Content[0].Text != rootCompletionReply {
					t.Errorf("original assistant output changed: %+v", item)
				}
			}
			if answers != 1 || dispatched.Load() != 0 {
				t.Fatalf("recovery left %d assistant answers and dispatched %d model calls", answers, dispatched.Load())
			}
			if cut == rootCompletionCancel || cut == rootCompletionLateClaim {
				history := readWaitingChildScopeLog(t, home)
				if controls := waitingChildHostControls(history, history.Root.String()); controls != 0 {
					t.Fatalf("the natural completion received %d cancellation controls", controls)
				}
				if err := api.DeleteSession(ctx, run.SessionID); err != nil {
					t.Fatal(err)
				}
				request := protocol.CancelRunRequest{RunID: original.RunID, Reason: activeRootCancelReason}
				result := host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, protocol.CallMetadata{IdempotencyKey: activeRootCancelKey})
				if !errors.Is(result.Failure, protocol.ErrRunFinished) {
					t.Fatalf("the root completion lost the original cancellation refusal: %+v", result)
				}
			}
		})
	}
}

func assertRootCompletionAtCrash(t *testing.T, home string, original protocol.StartRunResponse, cut string) runs.ModelInvocationCommit {
	t.Helper()
	committed := cut != rootCompletionTx
	if got := scopeRootCompleted(readWaitingChildScopeLog(t, home)); got != committed {
		t.Errorf("original Scope completion committed = %t at %s, want %t", got, cut, committed)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	defer stores.Close()
	run, found, err := stores.Runs.Run(t.Context(), original.RunID)
	if err != nil || !found {
		t.Fatalf("read original Run at %s: found = %t, %v", cut, found, err)
	}
	ending, _ := run.Ending()
	_, completed := ending.(rundomain.Completed)
	if completed != committed || run.State().IsTerminal() != committed {
		t.Errorf("original Run at %s: state = %s, ending = %+v, Scope completion should be committed = %t", cut, run.State(), ending, committed)
	}
	calls, err := stores.ModelInvocations.PageModelInvocations(t.Context(), original.RunID, 0, "", 2)
	if err != nil || len(calls) != 1 || calls[0].State != modelref.InvocationCompleted {
		t.Fatalf("original completed model call at %s: %+v, %v", cut, calls, err)
	}
	return calls[0]
}

func scopeRootCompleted(log scoperun.Log) bool {
	for _, entry := range log.Entries {
		if exit, ok := entry.Record.(scoperun.Exited); ok && exit.Process == log.Root &&
			exit.Termination.Cause() == scoperun.TerminationCauseCompletion {
			return true
		}
	}
	return false
}
