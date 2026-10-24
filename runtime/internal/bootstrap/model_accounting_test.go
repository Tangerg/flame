package bootstrap

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestDispatchedUnknownModelCallInvalidatesRunTotalCost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	model := &knownThenUnknownModel{}
	cfg := protocolRuntimeConfig(t, stores, model)
	cfg.Pricing = func(string, string, *chat.Usage) accounting.Cost {
		cost, _ := accounting.NewCost(.25)
		return cost
	}
	host, api := buildProtocolRuntime(t, cfg, home)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "known then unknown consumption"})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Read my Goal, then answer."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "unknown primary outcome")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := stores.ModelInvocations.ListSessionEndpoints(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := api.SessionUsage(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 || len(sources) != 2 || usage.Coverage.Calls != 2 || usage.Coverage.ReportedCalls != 1 || usage.Total.CostUSD != nil {
		t.Fatalf("expected one reported and one unknown physical call: calls=%d sources=%+v usage=%+v", model.calls, sources, usage)
	}
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeFailed {
		t.Fatalf("unknown provider outcome became success: %+v", finished.Outcome)
	}
	if finished.Metrics.Usage == nil || finished.Metrics.Usage.InputTokens != 7 || finished.Metrics.Usage.OutputTokens != 2 || finished.Metrics.Usage.CostUSD != nil || finished.Metrics.Steps != 2 {
		t.Fatalf("Run omitted dispatched unknown consumption: steps=%d usage=%+v", finished.Metrics.Steps, finished.Metrics.Usage)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, reader := openProtocolRuntime(t, newReplyStub("unused"))
	defer reopened.Close()
	restored, err := reader.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || restored.Metrics.Usage == nil || restored.Metrics.Usage.CostUSD != nil || restored.Metrics.Usage.InputTokens != 7 || restored.Metrics.Steps != 2 {
		t.Fatalf("restart changed committed consumption: %+v %v", restored.Metrics, err)
	}
}

type knownThenUnknownModel struct{ calls int }

func (m *knownThenUnknownModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	m.calls++
	if m.calls > 1 {
		return nil, errors.New("response lost after provider dispatch")
	}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
		ID: "read_goal_before_failure", Name: string(tool.GetGoal), Arguments: `{}`,
	}))
	return &chat.Response{
		Output:   &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls},
		Metadata: &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 7, OutputTokens: 2}},
	}, nil
}

func (m *knownThenUnknownModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(m.Call(ctx, request))
}
