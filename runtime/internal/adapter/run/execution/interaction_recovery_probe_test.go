package execution

import (
	"context"
	"errors"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionWaitingRecoveryPreservesCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name, want := "canceled", context.Canceled
		if deadline {
			name, want = "deadline", context.DeadlineExceeded
		}
		t.Run(name, func(t *testing.T) {
			checkpoint := captureInteractionQuestionCheckpoint(t, t.TempDir())
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("waiting recovery must not call the model")
			}), InteractionExecutorConfig{
				ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{
					Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
				}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			continuation := rootInteractionWaitingContinuation(checkpoint, "exec_restore", run.Capabilities{
				InterruptKinds: []interrupt.Kind{interrupt.Question},
			})
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if deadline {
				ctx, cancel = context.WithDeadline(t.Context(), time.Unix(1, 0))
				defer cancel()
			}
			resumption, err := executor.CanResumeWaitingExecution(ctx, continuation)
			resumable := resumption.Resumable()
			if resumable || !errors.Is(err, want) {
				t.Errorf("canceled recovery probe = %t, %v, want %v", resumable, err, want)
			}
			_, err = executor.RestoreWaitingExecution(ctx, continuation)
			if !errors.Is(err, want) || errors.Is(err, runs.ErrExecutorStateLost) {
				t.Errorf("canceled recovery = %v, want %v without state loss", err, want)
			}
			resumption, err = executor.CanResumeWaitingExecution(t.Context(), continuation)
			resumable = resumption.Resumable()
			if err != nil || !resumable {
				t.Fatalf("recovery after canceled probe = %t, %v, want true", resumable, err)
			}
		})
	}
}

func TestInteractionWaitingRecoveryPreservesActivationFailure(t *testing.T) {
	checkpoint := captureInteractionQuestionCheckpoint(t, t.TempDir())
	failure := errors.New("tree storage unavailable")
	store := &recoveryTreeStore{ExecutionTreeStore: testTrees(t), failure: failure}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("waiting recovery must not call the model")
	}), InteractionExecutorConfig{
		ExecutionTrees: store,
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	continuation := rootInteractionWaitingContinuation(checkpoint, "exec_restore", run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	})
	head, found, err := store.LoadExecutionTree(t.Context(), continuation.SessionID, checkpoint.RootMemberID)
	if err != nil || !found {
		t.Fatalf("LoadExecutionTree before recovery = %t, %v", found, err)
	}
	resumption, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
	resumable := resumption.Resumable()
	if err != nil || !resumable {
		t.Fatalf("read-only recovery probe = %t, %v, want true", resumable, err)
	}
	_, err = executor.RestoreWaitingExecution(t.Context(), continuation)
	if !errors.Is(err, failure) || errors.Is(err, runs.ErrExecutorStateLost) {
		t.Errorf("recovery activation = %v, want storage failure without state loss", err)
	}
	after, found, err := store.LoadExecutionTree(t.Context(), continuation.SessionID, checkpoint.RootMemberID)
	if err != nil || !found || !head.SameCommit(after) {
		t.Fatal("failed recovery replaced the durable tree or writer")
	}
	store.failure = nil
	ref, err := executor.RestoreWaitingExecution(t.Context(), continuation)
	if err != nil {
		t.Fatalf("recovery after storage repair = %v", err)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionWaitingRecoveryPreservesCancellationAfterActivation(t *testing.T) {
	checkpoint := captureInteractionQuestionCheckpoint(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store := &recoveryTreeStore{ExecutionTreeStore: testTrees(t), afterSave: cancel}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("waiting recovery must not call the model")
	}), InteractionExecutorConfig{
		ExecutionTrees: store,
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	continuation := rootInteractionWaitingContinuation(checkpoint, "exec_restore", run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	})
	_, err := executor.RestoreWaitingExecution(ctx, continuation)
	if !errors.Is(err, context.Canceled) || errors.Is(err, runs.ErrExecutorStateLost) {
		t.Errorf("canceled inspection after activation = %v, want cancellation without state loss", err)
	}
	store.afterSave = nil
	ref, err := executor.RestoreWaitingExecution(t.Context(), continuation)
	if err != nil {
		t.Fatalf("recovery after canceled inspection = %v", err)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

type recoveryTreeStore struct {
	runs.ExecutionTreeStore
	failure   error
	afterSave func()
}

func (s *recoveryTreeStore) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if s.failure != nil {
		return s.failure
	}
	if err := s.ExecutionTreeStore.SaveExecutionTree(ctx, update); err != nil {
		return err
	}
	if s.afterSave != nil {
		s.afterSave()
	}
	return nil
}
