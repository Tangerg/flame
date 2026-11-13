package pluginservices

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type failedModelSettlement struct {
	failure  error
	finished bool
	observed error
}

func (s *failedModelSettlement) Start(context.Context, string, modelchoice.Selection) (func(context.Context, error) error, error) {
	return func(_ context.Context, outcome error) error {
		s.finished = true
		s.observed = outcome
		return s.failure
	}, nil
}

func TestModelStreamSettlesBeforePublishingItsTerminalFailure(t *testing.T) {
	composition, _, _ := installedObserverFixture(t, "transparent")
	admissionFailure := errors.New("captured request admission failed")
	settlementFailure := errors.New("model fact settlement failed")
	endpoint := &selectedStreamModel{joined: make(chan struct{})}
	resolved, err := models.NewResolvedChat(endpoint, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := selectedCallInput("stream-model")
	selection, err := modelchoice.New(input.Selection.Provider, input.Selection.Model)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := models.NewSelectedChat(selection, resolved)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{
		ID: "terminal-stream", Origin: extension.Model, Parent: "original-operation",
		Target: portable.Target{Scope: extension.ScopeCall, Reference: "terminal-stream"},
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &failedModelSettlement{failure: settlementFailure}
	ctx = executionctx.WithModelCalls(ctx, publisher)
	captured, release, err := composition.capture(ctx, selected, modelchoice.Independent, func() error { return nil }, func(*chat.Request) error { return admissionFailure })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Error(err)
		}
	})
	streamer := captured.Model().(chat.Streamer)
	var terminal error
	for _, err := range streamer.Stream(ctx, &input.Request) {
		if err == nil {
			continue
		}
		if !publisher.finished {
			t.Error("terminal failure preceded model fact settlement")
		}
		terminal = err
		break
	}
	if !errors.Is(terminal, admissionFailure) || !errors.Is(terminal, settlementFailure) {
		t.Fatalf("terminal failure lost an original cause: %v", terminal)
	}
	if !publisher.finished || !errors.Is(publisher.observed, admissionFailure) {
		t.Fatalf("model settlement did not retain the admission outcome: %v", publisher.observed)
	}
	if endpoint.calls.Load() != 0 {
		t.Fatal("refused request reached the model")
	}
}
