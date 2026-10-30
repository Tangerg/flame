package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type pendingCallComposition struct {
	contextCaptureComposition
	t *testing.T
}

type unavailableCallComposition struct {
	contextCaptureComposition
	cause error
}

type failedRestorationComposition struct {
	contextCaptureComposition
	definite bool
	cause    error
}

func (c *failedRestorationComposition) RestorationFailure([]plugins.ContributionBinding) (bool, error) {
	return c.definite, c.cause
}

func TestWaitingContributionProbeUsesTheOriginalRestorationConclusion(t *testing.T) {
	for _, definite := range []bool{false, true} {
		name, cause := "inconclusive", errors.Join(plugin.ErrUnavailable, qimu.ErrRecoveryRequired)
		if definite {
			name, cause = "failed", errors.New("original MCP initialization closed")
		}
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				t.Fatal("restoration probe called the provider")
				return nil, nil
			}), InteractionExecutorConfig{CallComposition: &failedRestorationComposition{definite: definite, cause: cause}})
			continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_restoration", run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}})
			verdict, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
			if definite {
				if err != nil || verdict.Resumable() || verdict.Deferred() || verdict.Loss() != runs.LossConfigurationChanged {
					t.Fatalf("original failed restoration did not lose waiting state: %+v, %v", verdict, err)
				}
			} else if !errors.Is(err, cause) || verdict.Resumable() || verdict.Deferred() || verdict.Loss() != "" {
				t.Fatalf("diagnostic error category overrode the inconclusive restoration: %+v, %v", verdict, err)
			}
		})
	}
}

func (c *unavailableCallComposition) Activations(context.Context, []plugins.ContributionBinding) ([]plugin.Activation, error) {
	return nil, c.cause
}

func TestWaitingContributionProbeDistinguishesWithdrawalFromInconclusiveRead(t *testing.T) {
	for _, cause := range []error{plugin.ErrUnavailable, plugin.ErrStale, errors.New("activation storage unavailable")} {
		t.Run(cause.Error(), func(t *testing.T) {
			workspace := t.TempDir()
			checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				t.Fatal("probe called the provider")
				return nil, nil
			}), InteractionExecutorConfig{CallComposition: &unavailableCallComposition{cause: cause}})
			continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_unavailable", run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}})
			verdict, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
			if cause == plugin.ErrUnavailable || cause == plugin.ErrStale {
				if err != nil || verdict.Resumable() || verdict.Deferred() || verdict.Loss() != runs.LossConfigurationChanged {
					t.Fatalf("definite withdrawal blocked startup or changed capture: %+v, %v", verdict, err)
				}
			} else if !errors.Is(err, cause) || verdict.Resumable() || verdict.Deferred() || verdict.Loss() != "" {
				t.Fatalf("inconclusive read silently lost waiting state: %+v, %v", verdict, err)
			}
		})
	}
}

func TestFailedCompositionAssemblyReleasesResourcesBeforeEngineConstruction(t *testing.T) {
	failure := errors.New("activation storage unavailable")
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		t.Fatal("failed staging called the provider")
		return nil, nil
	}), InteractionExecutorConfig{CallComposition: &unavailableCallComposition{cause: failure}})
	if _, err := executor.StageRoot(t.Context(), interactionTestStart()); !errors.Is(err, failure) || errors.Is(err, agent.ErrInvalidEngineConfig) {
		t.Fatalf("failed pre-engine assembly did not preserve its cause and release resources: %v", err)
	}
	if err := executor.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("failed pre-engine assembly left shutdown resources: %v", err)
	}
}

func (c *pendingCallComposition) Pending(bindings []plugins.ContributionBinding) bool {
	if len(bindings) != 1 || bindings[0].ContributionID != "proposal" {
		c.t.Fatalf("recovery did not inspect its captured contribution: %+v", bindings)
	}
	return true
}

func TestWaitingRecoveryDefersCapturedContributionBeforeRequiringALiveActivation(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	var wire interactionCheckpointPayloadWire
	if err := json.Unmarshal(checkpoint.Payload, &wire); err != nil {
		t.Fatal(err)
	}
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		t.Fatal(err)
	}
	origin, err := mcpserver.InstallationOrigin(dependedInstallation)
	if err != nil {
		t.Fatal(err)
	}
	serverName := testsupport.ServerName("proposal")
	server, err := mcpserver.NewID(origin, serverName)
	if err != nil {
		t.Fatal(err)
	}
	wire.ToolProposals = new(contributionBindingsWire([]plugins.ContributionBinding{{InstallationID: dependedInstallation, Digest: testsupport.Digest("proposal"), ContributionID: "proposal", ServerID: server, ServerName: serverName, Export: "proposal", Contract: contract.Identity()}}))
	checkpoint.Payload, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		t.Error("recovery called the model")
		return nil, errors.New("unexpected model call")
	}), InteractionExecutorConfig{CallComposition: &pendingCallComposition{t: t}})
	executor.config.ChatResolver = interactionChatResolverFunc(func(context.Context, modelchoice.Selection) (modeladapter.ResolvedChat, error) {
		t.Error("recovery required a live activation before checking the pending connection")
		return modeladapter.ResolvedChat{}, errors.New("unexpected assembly")
	})
	continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_pending", run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}})
	verdict, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
	if err != nil || !verdict.Deferred() {
		t.Fatalf("pending contribution became startup failure or lost state: %+v, %v", verdict, err)
	}
}

func TestInteractionWaitingRecoveryPreservesCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name, want := "canceled", context.Canceled
		if deadline {
			name, want = "deadline", context.DeadlineExceeded
		}
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("waiting recovery must not call the model")
			}), InteractionExecutorConfig{
				ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ask")}, manifest: toolset.Manifest{
					Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
				}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_restore", run.Capabilities{
				InterruptKinds: []run.InterruptKind{run.InterruptQuestion},
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
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	failure := errors.New("tree storage unavailable")
	store := &recoveryTreeStore{ExecutionTreeStore: testTrees(t), failure: failure}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("waiting recovery must not call the model")
	}), InteractionExecutorConfig{
		ExecutionTrees: store,
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ask")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_restore", run.Capabilities{
		InterruptKinds: []run.InterruptKind{run.InterruptQuestion},
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
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store := &recoveryTreeStore{ExecutionTreeStore: testTrees(t), afterSave: cancel}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("waiting recovery must not call the model")
	}), InteractionExecutorConfig{
		ExecutionTrees: store,
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ask")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_restore", run.Capabilities{
		InterruptKinds: []run.InterruptKind{run.InterruptQuestion},
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
