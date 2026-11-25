package bootstrap

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestRunTelemetryRetainsTheCommittedScopeExit(t *testing.T) {
	observer := &heldScopeObservation{
		started: make(chan struct{}), resume: make(chan struct{}),
		ended: make(chan sdktrace.ReadOnlySpan, 1),
	}
	resumeObserver := sync.OnceFunc(func() { close(observer.resume) })
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(observer))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tracer)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		if err := tracer.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})

	release := make(chan struct{})
	releaseModel := sync.OnceFunc(func() { close(release) })
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		select {
		case <-release:
			return newReplyStub("Finished.").Call(ctx, req)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})}
	cfg := poMCPConfig(t)
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	t.Cleanup(resumeObserver)
	t.Cleanup(releaseModel)
	ctx := protocolLifecycleContext(t.Context())
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "observe durable exit"})
	if err != nil {
		t.Fatal(err)
	}
	started, stream, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Complete the Run."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectRunEvents(stream)
	waitForSignal(t, observer.started, "the live Scope process span")
	releaseModel()
	select {
	case result := <-events:
		if result.err != nil || len(result.events) == 0 {
			t.Errorf("the completed Run stream = %+v", result)
		}
	case <-time.After(8 * time.Second):
		t.Error("the completed Run stream stayed open while its telemetry reader was held")
	}
	completed, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || completed.Outcome == nil || completed.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("the Run did not commit its completion: %+v, %v", completed, err)
	}
	resumeObserver()
	waiting, cancel := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer cancel()
	select {
	case span := <-observer.ended:
		for _, attr := range span.Attributes() {
			if attr.Key == "agent.process.status" && attr.Value.AsString() == "completed" {
				return
			}
		}
		t.Fatalf("the live Scope process span lost its committed exit: status %+v, attributes %v", span.Status(), span.Attributes())
	case <-waiting.Done():
		t.Fatal("the live Scope process span did not finish")
	}
}

type heldScopeObservation struct {
	once    sync.Once
	started chan struct{}
	resume  chan struct{}
	ended   chan sdktrace.ReadOnlySpan
}

func (o *heldScopeObservation) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	if isScopeProcessSpan(span) {
		o.once.Do(func() {
			close(o.started)
			<-o.resume
		})
	}
}

func (o *heldScopeObservation) OnEnd(span sdktrace.ReadOnlySpan) {
	if isScopeProcessSpan(span) {
		o.ended <- span
	}
}

func (*heldScopeObservation) Shutdown(context.Context) error   { return nil }
func (*heldScopeObservation) ForceFlush(context.Context) error { return nil }

func isScopeProcessSpan(span sdktrace.ReadOnlySpan) bool {
	return span.InstrumentationScope().Name == "github.com/Tangerg/scope/otel/agent" &&
		strings.HasPrefix(span.Name(), "invoke_agent ")
}
