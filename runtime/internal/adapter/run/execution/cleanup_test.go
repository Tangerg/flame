package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
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
		executor.config.ChatResolver = interactionChatResolverFunc(func(ctx context.Context, selection modelchoice.Selection) (modeladapter.ResolvedChat, error) {
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
	synctest.Test(t, func(t *testing.T) {
		trees := testTrees(t)
		entered, proceed := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		defer unblock()
		model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			close(entered)
			<-proceed
			return interactionTextResponse("finished"), nil
		})
		executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ExecutionTrees: trees})
		start := interactionTestStart()
		ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: "exec_unpublished"}
		session, err := executor.assembleInteraction(t.Context(), ref, start, nil)
		if err != nil {
			t.Fatal(err)
		}
		// An interrupted cleanup must retain the same external resources until
		// Scope has joined the callback, even when that callback ignores cancel.
		program, err := interaction.Compile(session.engine.Compiler(), interaction.Config{
			Name: "cleanup", Description: "Exercise external work draining.", Revision: "cleanup",
			Model: interaction.CallModel(model),
		})
		if err != nil {
			t.Fatal(err)
		}
		session.deployment = program.Erase()
		session.state.deployments.root = session.deployment
		compiled, err := newCompiledInteraction(program)
		if err != nil {
			t.Fatal(err)
		}
		session.state.deployments.interactions = map[scoperun.ProgramRef]compiledInteraction{program.ProgramRef(): compiled}
		input, err := program.Descriptor().EncodeInput(interaction.Input{Messages: start.WorkingContext, Options: chat.Options{Model: "test-model"}})
		if err != nil {
			t.Fatal(err)
		}
		process, err := session.engine.Start(t.Context(), session.deployment, scoperun.StartRequest[scoperun.Payload]{
			ID: scoperun.StartID{Scope: ref.SessionID, Key: ref.ExecutorID}, Input: input,
		})
		if err != nil {
			t.Fatal(err)
		}
		session.state.setProcess(process)
		<-entered
		discarded := make(chan error, 1)
		go func() { discarded <- executor.discardInteraction(session) }()
		if err := <-discarded; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("discard with blocked external work = %v", err)
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
		if _, err := session.engine.Recover(t.Context(), program, process.ID()); !errors.Is(err, scoperun.ErrClosed) {
			t.Fatalf("shutdown did not close the runtime: %v", err)
		}
	})
}
