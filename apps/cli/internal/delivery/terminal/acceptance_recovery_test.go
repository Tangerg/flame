package terminal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type retiredOpeningProbe struct {
	*runtimefixture.Runtime
	starts        atomic.Int32
	cancellations atomic.Int32
	subscriptions atomic.Int32
	lookups       atomic.Int32
}

type observedCurrentOwner struct {
	Runtime
	subscribed chan struct{}
}

func (r *observedCurrentOwner) SubscribeRun(ctx context.Context, request conversation.SubscribeRun) (conversation.SegmentStream, error) {
	stream, err := r.Runtime.SubscribeRun(ctx, request)
	if err == nil && request.Snapshot {
		select {
		case r.subscribed <- struct{}{}:
		default:
		}
	}
	return stream, err
}

func (r *retiredOpeningProbe) GetRunAcceptance(ctx context.Context, query protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error) {
	r.lookups.Add(1)
	return r.Runtime.GetRunAcceptance(ctx, query)
}

func (r *retiredOpeningProbe) StartRun(ctx context.Context, command prompt.StartRun) (conversation.SegmentStream, error) {
	r.starts.Add(1)
	return r.Runtime.StartRun(ctx, command)
}

func (r *retiredOpeningProbe) CancelRun(ctx context.Context, command conversation.CancelRun) (conversation.RunCancellation, error) {
	r.cancellations.Add(1)
	return r.Runtime.CancelRun(ctx, command)
}

func (r *retiredOpeningProbe) SubscribeRun(ctx context.Context, request conversation.SubscribeRun) (conversation.SegmentStream, error) {
	r.subscriptions.Add(1)
	return r.Runtime.SubscribeRun(ctx, request)
}

func TestRetiredAcceptanceSettlesTheOpeningWithoutObservingOrCancelingMaterial(t *testing.T) {
	for _, canceling := range []bool{false, true} {
		name := "dispatching"
		if canceling {
			name = "canceling"
		}
		t.Run(name, func(t *testing.T) {
			base := runtimefixture.New()
			base.Instant = true
			base.Script = stableCompletedScript
			command := testStartRun("ses_demo_1", "retired opening")
			command.CommandID = "cli_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			opened, err := base.StartRun(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			for _, err := range opened.Events {
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := base.RollbackSession(t.Context(), conversation.RollbackSession{SessionID: command.SessionID, Scope: protocol.RestoreHistory}); err != nil {
				t.Fatal(err)
			}
			stateDirectory := t.TempDir()
			store, err := openTestWorkbench(stateDirectory)
			if err != nil {
				t.Fatal(err)
			}
			stageDispatchingRun(t, store, command)
			if canceling {
				if _, err := store.MarkPendingRunCanceling(command.SessionID, command.CommandID, durableCommandReplayGuard(t)); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			backend := &retiredOpeningProbe{Runtime: base}
			host, stop, workbenchForHost := runUIWithReplayState(t, backend, "/tmp/flame-cli-test", command.SessionID, stateDirectory)
			if !canceling {
				host.Shows(t, "accepted run material has been retired")
			}
			awaitState(t, "retired command receipt settlement", func() bool {
				reopened := workbenchForHost()
				if reopened == nil {
					return false
				}
				return len(reopened.PendingRuns(command.SessionID)) == 0
			})
			stop()
			if backend.lookups.Load() != 1 || backend.starts.Load() != 0 || backend.subscriptions.Load() != 0 || backend.cancellations.Load() != 0 {
				t.Fatalf("retired receipt touched product material: lookups=%d starts=%d subscriptions=%d cancellations=%d", backend.lookups.Load(), backend.starts.Load(), backend.subscriptions.Load(), backend.cancellations.Load())
			}
		})
	}
}

func TestAnotherRunsTerminalEventCannotSettleAnUnconfirmedOpening(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}}}}
	}
	current, err := base.StartRun(t.Context(), testStartRun("ses_demo_1", "current Runtime owner"))
	if err != nil {
		t.Fatal(err)
	}
	command := testStartRun("ses_demo_1", "unconfirmed opening")
	command.CommandID = "cli_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	stateDirectory := t.TempDir()
	store, err := openTestWorkbench(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingRun(workbench.PendingRun{Command: command, State: workbench.PendingRunQueued, Replay: replay.UnprotectedGuard(), CancelReplay: replay.UnprotectedGuard()}); err != nil {
		t.Fatal(err)
	}
	profile := steerReplayTestProfile(t, "/tmp/flame-cli-test")
	guard := protectedCommandReplayGuard(t, profile.Discovery().Capabilities.Limits.Idempotency.Namespace, time.Now().Add(-time.Second))
	if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID, guard, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	observation := &observedCurrentOwner{Runtime: base, subscribed: make(chan struct{}, 1)}
	gate := &gatedRecoveryStartRuntime{Runtime: observation, commandID: command.CommandID, started: make(chan struct{}, 1), release: make(chan struct{})}
	host, stop, workbenchForHost := runUIFromConfig(t, Config{Runtime: gate, RuntimeProfile: &profile, SessionID: command.SessionID, Workspace: "/tmp/flame-cli-test", OpenWorkbench: persistentTestWorkbench(stateDirectory), DetachOnExit: true})
	awaitValue(t, gate.started, "original command lookup")
	awaitValue(t, observation.subscribed, "current owner snapshot and successor tail")
	if _, err := base.CancelRun(t.Context(), conversation.CancelRun{RunID: current.RunID, Reason: "finish the other Runtime owner"}); err != nil {
		t.Fatal(err)
	}
	host.Shows(t, "pending prompt outcome unknown")
	reopened := workbenchForHost()
	pending := reopened.PendingRuns(command.SessionID)
	if len(pending) != 1 || pending[0].Command.CommandID != command.CommandID || pending[0].State != workbench.PendingRunDispatching {
		t.Fatalf("another Run settled the original command: %+v", pending)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	close(gate.release)
	host.Shows(t, "mutation replay guarantee is unavailable")
	stop()
}
