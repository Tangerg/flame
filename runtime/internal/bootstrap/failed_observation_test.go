package bootstrap

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
)

func TestFailedStreamObservationSurvivesRuntimeRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	cfg := protocolRuntimeConfig(t, stores, incompleteObservationModel{})
	cfg.modelProviders = observationResolver{modelProviderAdapters: cfg.modelProviders}
	host, api := buildProtocolRuntime(t, cfg, home)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "fail after visible output"}}})
	if err != nil {
		t.Fatal(err)
	}
	for event, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		if err := protocol.ValidateWireTree(event.Event); err != nil {
			t.Fatal(err)
		}
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, reader := openProtocolRuntime(t, newReplyStub("unused"))
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	current, err := reader.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || current.Outcome == nil || current.Outcome.Type != protocol.OutcomeFailed {
		t.Fatalf("reopened run=%+v error=%v", current, err)
	}
	items, err := reader.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}})
	if err != nil {
		t.Fatal(err)
	}
	visible := 0
	for _, item := range items.Data {
		if err := protocol.ValidateWireTree(item); err != nil {
			t.Fatal(err)
		}
		switch item.Type {
		case protocol.ItemTypeAgentMessage:
			visible++
			if item.Status != protocol.ItemStatusIncomplete || len(item.Content) != 1 || item.Content[0].Text != strings.Repeat("prefix ", 2048) {
				t.Fatalf("lost text observation: %+v", item)
			}
		case protocol.ItemTypeReasoning:
			visible++
			if item.Status != protocol.ItemStatusIncomplete || item.Text != "beforeafter" {
				t.Fatalf("lost reasoning observation: %+v", item)
			}
		case protocol.ItemTypeToolCall:
			t.Fatal("partial tool call became an executable transcript item")
		}
	}
	if visible != 2 {
		t.Fatalf("visible incomplete items=%d", visible)
	}
	calls, err := reader.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: started.RunID})
	if err != nil || len(calls.Data) != 1 || calls.Data[0].State != protocol.ModelInvocationFailed {
		t.Fatalf("failed invocation=%+v error=%v", calls, err)
	}
}

type incompleteObservationModel struct{}

func (incompleteObservationModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("expected streaming")
}

func (incompleteObservationModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		const (
			reasoningPartIndex = iota
			textPartIndex
			toolCallPartIndex
		)
		before := chat.NewReasoningDelta("before", nil)
		before.Index = reasoningPartIndex
		text := chat.NewTextDelta("prefix ")
		text.Index = textPartIndex
		after := chat.NewReasoningDelta("after", nil)
		after.Index = reasoningPartIndex
		toolCall := chat.NewToolCallDelta(chat.ToolCallDelta{ID: "unfinished", Name: "shell", Arguments: `{"command":`})
		toolCall.Index = toolCallPartIndex
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{before}}, nil) {
			return
		}
		for range 2048 {
			if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{text}}, nil) {
				return
			}
		}
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{after, toolCall}}, nil) {
			return
		}
		yield(nil, errors.New("provider stream interrupted"))
	}
}

type observationResolver struct{ modelProviderAdapters }

func (observationResolver) OpenChat(context.Context, flamemodel.Selection, modelprovider.Connection) (modeladapter.ResolvedChat, error) {
	return testsupport.OfflineChat(incompleteObservationModel{}, nil, nil, testsupport.Unpriced())
}

func (r observationResolver) RestoreChat(ctx context.Context, selection flamemodel.Selection, _ *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error) {
	return r.OpenChat(ctx, selection, modelprovider.Connection{})
}
