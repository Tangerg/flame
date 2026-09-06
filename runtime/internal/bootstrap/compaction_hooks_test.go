package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestRuntimeStopsRequiredCompactionWhenHookConfigurationCannotBeRead(t *testing.T) {
	longRun := &longContextModel{}
	opening := make(chan struct{})
	released := make(chan struct{})
	release := sync.OnceFunc(func() { close(released) })
	var first sync.Once
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if !isCompactionRequest(request) {
			first.Do(func() {
				close(opening)
				select {
				case <-released:
				case <-ctx.Done():
				}
			})
		}
		return longRun.Call(ctx, request)
	})}
	stores, api, ctx, home := newSessionStateE2ERuntime(t, model)
	t.Cleanup(release)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "compaction hook configuration failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, sequence, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Continue the long tool loop."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectRunEvents(sequence)
	select {
	case <-opening:
	case <-time.After(5 * time.Second):
		t.Fatal("opening model call did not arrive")
	}
	// Authored configuration can change while the model is running. A later
	// required compaction must resolve its veto policy from that current source.
	if err := os.MkdirAll(filepath.Join(home, ".flame"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".flame", "hooks.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	release()
	waitForRunEvents(t, events, "compaction hook configuration failure")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeFailed ||
		finished.Outcome.Error == nil || !strings.Contains(finished.Outcome.Error.Detail, "hooks") {
		t.Fatalf("unreadable compaction policy did not stop the Run: %+v", finished.Outcome)
	}
	_, summaries, _, _ := longRun.Snapshot()
	if summaries != 0 {
		t.Fatalf("summary model calls = %d, want none before hook policy is known", summaries)
	}
	history, err := stores.ChatHistory.Read(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range history {
		if strings.HasPrefix(message.Text(), "[Earlier conversation summary]") {
			t.Fatal("unreadable hook policy allowed a durable conversation rewrite")
		}
	}
}
