package agentexec

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionShutdownJoinsAssemblyBeforeReleasingResources(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, proceed := make(chan struct{}), make(chan struct{})
		model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			t.Error("staging called the model")
			return nil, errors.New("unexpected model call")
		})
		executor := newTestInteractionExecutor(t, model)
		resolver := executor.config.ChatResolver
		executor.config.ChatResolver = interactionChatResolverFunc(func(ctx context.Context, selection modelref.Selection) (modeladapter.ResolvedChat, error) {
			close(entered)
			<-proceed
			return resolver.ResolveChat(ctx, selection)
		})
		staged := make(chan error, 1)
		go func() {
			_, err := executor.StageRoot(t.Context(), interactionTestStart())
			staged <- err
		}()
		<-entered
		executor.BeginShutdown()
		closed := make(chan error, 1)
		go func() { closed <- executor.AwaitShutdown(t.Context()) }()
		synctest.Wait()
		select {
		case err := <-closed:
			t.Fatalf("shutdown abandoned in-flight assembly: %v", err)
		default:
		}
		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := executor.AwaitShutdown(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("interrupted assembly join = %v", err)
		}
		close(proceed)
		if err := <-staged; err == nil {
			t.Fatal("assembly published after shutdown admission closed")
		}
		if err := <-closed; err != nil {
			t.Fatalf("shutdown after assembly rollback: %v", err)
		}
	})
}

func TestInteractionFailedDiscardRemainsOwnedUntilShutdown(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	synctest.Test(t, func(t *testing.T) {
		executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			t.Error("restored waiting tree called the model")
			return nil, errors.New("unexpected model call")
		}), InteractionExecutorConfig{
			ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)}}},
			ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
		})
		start := interactionTestStart()
		start.CWD, start.WorkspaceCWD = workspace, workspace
		start.ModelSelection, start.Limits = checkpoint.ModelSelection, checkpoint.Limits
		start.InterruptKinds = []interrupt.Kind{interrupt.Question}
		state, err := decodeExecutorCheckpoint(checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		start.WorkingContext = cloneChatMessages(state.instructions)
		ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: "exec_unpublished"}
		session, err := executor.assembleInteraction(t.Context(), ref, start)
		if err != nil {
			t.Fatal(err)
		}
		if err := session.engine.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
		entered, proceed := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		defer unblock()
		var once sync.Once
		session.engine, err = agent.NewEngine(agent.EngineConfig{
			DeploymentResolver: session.state.deployments,
			EventListeners: []agent.EventListener{agent.EventListenerFunc(func(_ context.Context, event agent.Event) {
				if _, terminal := event.ProcessFinished(); terminal {
					once.Do(func() { close(entered) })
					<-proceed
				}
			})},
		})
		if err != nil {
			t.Fatal(err)
		}
		process, err := session.engine.RestoreTree(t.Context(), session.deployment, state.tree)
		if err != nil {
			t.Fatal(err)
		}
		session.state.setProcess(process)
		discarded := make(chan error, 1)
		go func() { discarded <- executor.discardInteraction(session) }()
		<-entered
		if err := <-discarded; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("discard with blocked terminal observer = %v", err)
		}
		if _, err := executor.session(ref); !errors.Is(err, runs.ErrExecutorNotLive) {
			t.Fatalf("failed assembly became callable: %v", err)
		}
		closed := make(chan error, 1)
		go func() { closed <- executor.AwaitShutdown(t.Context()) }()
		synctest.Wait()
		select {
		case err := <-closed:
			t.Fatalf("shutdown abandoned failed cleanup: %v", err)
		default:
		}
		unblock()
		if err := <-closed; err != nil {
			t.Fatalf("shutdown retry: %v", err)
		}
		if _, err := session.engine.RestoreTree(t.Context(), session.deployment, state.tree); !errors.Is(err, agent.ErrEngineClosed) {
			t.Fatalf("shutdown did not close recovered engine: %v", err)
		}
	})
}
