package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/workbenchstate"
	"github.com/Tangerg/flame/cli/internal/application/agent/promptqueue"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"
)

func TestStartConflictAndLocalPreparationFailuresKeepOriginalDispatchIdentity(t *testing.T) {
	for _, cause := range []error{agent.ErrCommandConflict, agent.ErrCommandInputUnavailable, errors.Join(agent.ErrCommandNotDispatched, errors.New("local failure"))} {
		t.Run(cause.Error(), func(t *testing.T) {
			store, err := workbench.OpenMemory(workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			command := testStartRun("ses_1", "original command")
			command.CommandID = "cli_11111111111111111111111111111111"
			stageDispatchingRun(t, store, command)
			queue := promptqueue.New()
			if err := queue.Restore(command.SessionID, []agent.StartRun{command}, command.CommandID); err != nil {
				t.Fatal(err)
			}
			application := &app{workbench: store, queue: queue}
			if err := application.requeueDefinitivelyRefusedStart(command, &startRunCallError{err: cause}); err != nil {
				t.Fatal(err)
			}
			pending := store.PendingRuns(command.SessionID)
			if len(pending) != 1 || pending[0].State != workbench.PendingRunDispatching || pending[0].Command.CommandID != command.CommandID {
				t.Fatal("uncertain start was requeued under a new identity")
			}
			if dispatch, found := queue.Dispatching(command.SessionID); !found || dispatch.CommandID != command.CommandID {
				t.Fatal("uncertain start lost its queue reservation")
			}
		})
	}
}

type unavailableAttachmentRuntime struct{ *steeringRuntime }

func (r unavailableAttachmentRuntime) PrepareInput(ctx context.Context, message agent.Message) ([]protocol.ContentBlock, error) {
	if len(message.Attachments) != 0 {
		return nil, os.ErrNotExist
	}
	return r.Runtime.PrepareInput(ctx, message)
}

func TestSteerPreparationFailurePreservesTheEditableInstructionAndAttachments(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := unavailableAttachmentRuntime{steeringRuntime: &steeringRuntime{Runtime: base}}
	workspace, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, workspace, "", state)
	host.Shows(t, "Ask flame")
	sessionID := firstRuntimeSession(t, base)
	host.Type("start long work")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/attach notes.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached notes.txt")
	host.Type("/steer preserve this instruction")
	host.Press(input.Enter)
	host.Shows(t, "prepare steer input")
	stop()
	store, err := workbenchstate.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	draft, found := store.Draft(sessionID)
	if !found || draft.Text != "/steer preserve this instruction" || len(draft.Attachments) != 1 {
		t.Fatalf("preparation failure lost the editable draft: %+v, %t", draft, found)
	}
	if _, found := store.PendingSteer(sessionID); found || backend.lastSteer().CommandID != "" {
		t.Fatal("preparation failure staged or dispatched a steer")
	}
}

type pausedInputRuntime struct {
	*steeringRuntime
	entered  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (r *pausedInputRuntime) PrepareInput(ctx context.Context, message agent.Message) ([]protocol.ContentBlock, error) {
	if len(message.Attachments) == 0 {
		return r.Runtime.PrepareInput(ctx, message)
	}
	close(r.entered)
	select {
	case <-ctx.Done():
		close(r.canceled)
		return nil, context.Cause(ctx)
	case <-r.release:
		return r.Runtime.PrepareInput(ctx, message)
	}
}

func TestInputPreparationLeavesTheUIResponsiveAndUnsentInputEditable(t *testing.T) {
	for _, command := range []string{"start", "steer"} {
		for _, stopPreparation := range []string{"cancel", "close"} {
			t.Run(command+"/"+stopPreparation, func(t *testing.T) {
				base := runtimefixture.New()
				base.Script = func(string) runtimefixture.Script {
					return runtimefixture.Script{Prelude: []runtimefixture.Step{
						{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
						{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}}},
					}}
				}
				backend := &pausedInputRuntime{
					steeringRuntime: &steeringRuntime{Runtime: base},
					entered:         make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}),
				}
				workspace, state := t.TempDir(), t.TempDir()
				if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("notes"), 0o600); err != nil {
					t.Fatal(err)
				}
				host, stop := runUIWithState(t, backend, workspace, "", state)
				host.Shows(t, "Ask flame")
				sessionID := firstRuntimeSession(t, base)
				if command == "steer" {
					host.Type("start long work")
					host.Press(input.Enter)
					host.Shows(t, "thinking")
				}
				host.Type("/attach notes.txt")
				host.Press(input.Enter)
				host.Shows(t, "attached notes.txt")
				text := "preserve this instruction"
				if command == "steer" {
					text = "/steer " + text
				}
				host.Type(text)
				host.Press(input.Enter)
				awaitSignal(t, backend.entered, "attachment preparation")
				var originalID agent.CommandID
				if command == "start" {
					observer, err := workbenchstate.Open(state)
					if err != nil {
						t.Fatal(err)
					}
					queued := observer.PendingRuns(sessionID)
					if len(queued) != 1 || queued[0].State != workbench.PendingRunQueued {
						t.Fatalf("preparation changed durable queue admission: %+v", queued)
					}
					originalID = queued[0].Command.CommandID
					if err := observer.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if !host.Resize(32, 10) || !host.Repaint() || !host.Resize(96, 28) {
					t.Fatal("attachment preparation blocked the terminal loop")
				}
				if stopPreparation == "cancel" {
					host.Press(input.Esc)
					host.Shows(t, "input preparation canceled")
				}
				stop()
				awaitSignal(t, backend.canceled, "preparation cancellation")
				store, err := workbenchstate.Open(state)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				if command == "start" {
					pending := store.PendingRuns(sessionID)
					if len(pending) != 1 || pending[0].State != workbench.PendingRunQueued ||
						pending[0].Command.Message.Text != text || len(pending[0].Command.Message.Attachments) != 1 ||
						pending[0].InputDigest != "" || pending[0].Command.Input != nil || pending[0].Command.CommandID != originalID {
						t.Fatalf("unattempted start was lost or marked dispatched: %+v", pending)
					}
					pending[0].Command.Message.Text = "edited after cancellation"
					if err := store.SavePendingRuns(sessionID, pending); err != nil {
						t.Fatal(err)
					}
					if store.PendingRuns(sessionID)[0].Command.CommandID != originalID {
						t.Fatal("editing an unattempted command replaced its identity")
					}
					snapshot, err := base.GetSession(t.Context(), sessionID)
					if err != nil || len(snapshot.Runs) != 0 {
						t.Fatalf("preparation dispatched a run: %+v, %v", snapshot.Runs, err)
					}
				} else {
					draft, found := store.Draft(sessionID)
					if !found || draft.Text != text || len(draft.Attachments) != 1 {
						t.Fatalf("unattempted steer lost its editable draft: %+v, %t", draft, found)
					}
					if _, found := store.PendingSteer(sessionID); found || backend.lastSteer().CommandID != "" {
						t.Fatal("preparation staged or dispatched a steer")
					}
				}
			})
		}
	}
}

func TestEditingSteerDuringInputPreparationPreservesTheNewDraft(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "thinking", Kind: agent.BlockReasoning}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &pausedInputRuntime{
		steeringRuntime: &steeringRuntime{Runtime: base},
		entered:         make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}),
	}
	workspace, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, workspace, "", state)
	host.Shows(t, "Ask flame")
	sessionID := firstRuntimeSession(t, base)
	host.Type("start long work")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/attach notes.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached notes.txt")
	host.Type("/steer original instruction")
	host.Press(input.Enter)
	awaitSignal(t, backend.entered, "attachment preparation")
	host.Shows(t, "/steer original instruction")
	host.Type(" edited")
	// Host.Type queues keyboard input. Observe the editor update before letting
	// preparation finish so the callback must see the already-edited source.
	host.Shows(t, "/steer original instruction edited")
	close(backend.release)
	host.Shows(t, "steer preparation canceled because the draft changed")
	stop()
	store, err := workbenchstate.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	draft, found := store.Draft(sessionID)
	if !found || draft.Text != "/steer original instruction edited" || len(draft.Attachments) != 1 {
		t.Fatalf("late input preparation replaced the edited draft: %+v, %t", draft, found)
	}
	if _, found := store.PendingSteer(sessionID); found || backend.lastSteer().CommandID != "" {
		t.Fatal("editing the source draft still dispatched its old prepared content")
	}
}
