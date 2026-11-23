package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestUnsettledModelPreviewStaysIncomplete(t *testing.T) {
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
	questionCrashDatabase(t, home, `CREATE TRIGGER refuse_failed_model_publication BEFORE UPDATE OF state ON model_invocations
		WHEN NEW.state = 'failed' AND OLD.endpoint_json IS NULL
		BEGIN SELECT RAISE(ABORT, 'failed model publication refused'); END`)
	ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), 25*time.Second)
	defer cancel()
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "keep unsettled output incomplete",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "produce only a fragment"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	previews := make(map[protocol.ItemType]string)
	var lost bool
	for event, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		if err := protocol.ValidateWireTree(event.Event); err != nil {
			t.Fatal(err)
		}
		switch event.Event.Type {
		case protocol.StreamItemStarted:
			item := event.Event.Item
			if item.Type == protocol.ItemTypeAgentMessage || item.Type == protocol.ItemTypeReasoning {
				if previews[item.Type] != "" {
					t.Fatalf("stream opened more than one %s preview", item.Type)
				}
				previews[item.Type] = item.ID
			}
		case protocol.StreamSegmentFinished:
			lost = event.Event.Run.Outcome != nil && event.Event.Run.Outcome.Type == protocol.OutcomeLost
		}
	}
	if !lost || len(previews) != 2 {
		t.Fatalf("stream did not publish both previews and the lost Run: previews=%d, lost=%t", len(previews), lost)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER refuse_failed_model_publication")
	reopened, reader := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	finished, err := reader.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeLost {
		t.Fatalf("run got %+v, error %v, want Lost with the refused model settlement", finished, err)
	}
	calls, err := reader.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: started.RunID})
	if err != nil || len(calls.Data) != 1 || calls.Data[0].State != protocol.ModelInvocationUnknown {
		t.Fatalf("unsettled logical call = %+v, error %v", calls, err)
	}
	items, err := reader.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{
		Type: protocol.ItemScopeRun, RunID: started.RunID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	observations := 0
	for _, item := range items.Data {
		switch item.Type {
		case protocol.ItemTypeAgentMessage, protocol.ItemTypeReasoning:
			observations++
			if item.ID != previews[item.Type] {
				t.Errorf("recovered %s has Item ID %q, want original preview %q", item.Type, item.ID, previews[item.Type])
			}
			if item.Status != protocol.ItemStatusIncomplete {
				t.Errorf("unsettled model %s persisted as %s, want incomplete", item.Type, item.Status)
			}
		case protocol.ItemTypeToolCall:
			t.Fatal("unsettled tool proposal became executable")
		}
	}
	if observations != 2 {
		t.Fatalf("unsettled text and reasoning = %d, want 2", observations)
	}
}
