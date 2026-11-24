package bootstrap

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"

	"github.com/Tangerg/scope/core/chat"
)

// A reference reaches the model framed apart from what the user typed, and the
// Session keeps it exactly as the user sent it, across a restart.
func TestReferencedInputReachesTheModelAndSurvivesRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	var mu sync.Mutex
	var seen []string
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		for _, message := range request.Messages {
			if message.Role != chat.RoleUser {
				continue
			}
			mu.Lock()
			for _, part := range message.Parts {
				seen = append(seen, part.Text)
			}
			mu.Unlock()
		}
		return completedTextResponse("done"), nil
	})}
	input := []protocol.ContentBlock{
		{Type: protocol.ContentBlockText, Text: "Shorten this title."},
		{Type: protocol.ContentBlockReference, Label: `Slide 4 · "Revenue" chart`, Text: "Deck d1 revision r12, slide s4, element 7."},
	}
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{ProtocolVersion: protocol.Version})
	first, api := openProtocolRuntime(t, model)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}, Title: "reference"})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "referenced run")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	want := []string{"Shorten this title.", "[Reference attached by the user: \"Slide 4 · \\\"Revenue\\\" chart\"]\nDeck d1 revision r12, slide s4, element 7.\n[End reference]"}
	mu.Lock()
	received := slices.Clone(seen)
	mu.Unlock()
	if len(received) < len(want) || !slices.Equal(received[:len(want)], want) {
		t.Fatalf("model received %q, want %q", received, want)
	}

	restarted, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID}})
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(items.Data, func(item protocol.Item) bool { return item.Type == protocol.ItemTypeUserMessage })
	if index < 0 || !slices.Equal(items.Data[index].Content, input) {
		t.Fatalf("restored items = %+v", items.Data)
	}
}
