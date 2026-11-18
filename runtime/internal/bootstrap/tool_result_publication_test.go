package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/scope/core/chat"
)

func TestOffloadedToolResultCommitsWithItsItem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	body := strings.Repeat("Retain this original Tool output. ", 128)
	if err := os.WriteFile(filepath.Join(home, "large.txt"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	directory, err := localruntime.DataDirectoryAt(home)
	if err != nil {
		t.Fatal(err)
	}
	probe := openCommandEffects(t, directory.DatabasePath())
	var sessionID atomic.Value
	var detachedBody atomic.Bool
	transact := stores.Transactor
	stores.Transactor = func(ctx context.Context, apply func(context.Context) error) error {
		if owner, active := sessionID.Load().(string); active {
			var bodies int
			if err := probe.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_result_blobs WHERE session_id = ?`, owner).Scan(&bodies); err != nil {
				return err
			}
			if bodies > 0 {
				items, err := stores.Transcript.ListStored(ctx, owner)
				if err != nil {
					return err
				}
				var named int
				for _, item := range items {
					if call, ok := item.ToolCall(); ok && call.Invocation.Offload != nil {
						named++
					}
				}
				if bodies > named {
					detachedBody.Store(true)
				}
			}
		}
		return transact(ctx, apply)
	}
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		call := chat.ToolCall{ID: "read_original", Name: string(builtin.Read), Arguments: `{"path":"large.txt"}`}
		for _, message := range slices.Backward(request.Messages) {
			for _, part := range slices.Backward(message.Parts) {
				if part.ToolResult == nil {
					continue
				}
				text, _ := part.ToolResult.Output.Text()
				switch part.ToolResult.Name {
				case string(builtin.ReadToolResult):
					if !strings.Contains(text, body) {
						return nil, fmt.Errorf("read_tool_result did not retrieve the original file")
					}
					return completedTextResponse("The file was read."), nil
				case string(builtin.Read):
					_, marker, found := strings.Cut(text, `"result_id":"`)
					id, _, closed := strings.Cut(marker, `"`)
					if !found || !closed || strings.Contains(text, body) {
						return nil, fmt.Errorf("model did not receive a bounded preview with a result ID")
					}
					call = chat.ToolCall{ID: "retrieve_original", Name: string(builtin.ReadToolResult), Arguments: fmt.Sprintf(`{"result_id":%q}`, id)}
				}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}}, nil
	})}
	cfg := protocolRuntimeConfig(t, stores, model)
	cfg.toolResultOffloadThreshold = new(512)
	host, api := buildProtocolRuntime(t, cfg, home)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "Offloaded Tool result",
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID.Store(session.ID)
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Read the large file."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "offloaded Tool result")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run did not complete: %+v, %v", finished, err)
	}
	published, err := stores.ToolResults.List(ctx, session.ID)
	if err != nil || len(published) != 1 || !strings.Contains(published[0].Body, body) {
		t.Fatalf("expected one complete, Item-owned body: count=%d, error=%v", len(published), err)
	}
	if detachedBody.Load() {
		t.Fatal("a storage transaction observed the Tool body before its owning Item committed")
	}

	original := published[0]
	assertOriginal := func(sessionID string) {
		t.Helper()
		results, err := stores.ToolResults.List(ctx, sessionID)
		if err != nil || len(results) != 1 {
			t.Fatalf("session %s has %d bodies: %v", sessionID, len(results), err)
		}
		got := results[0]
		if got.ID != original.ID || got.Body != original.Body || !got.CreatedAt.Equal(original.CreatedAt) {
			t.Fatalf("session %s changed the offloaded body, ID, or creation time", sessionID)
		}
	}
	assertRemoved := func(sessionID string) {
		t.Helper()
		if _, found, err := stores.ToolResults.Fetch(ctx, sessionID, original.ID); found || err != nil {
			t.Fatalf("retired Item's body is still readable: found=%v, error=%v", found, err)
		}
	}

	forked, err := api.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	assertOriginal(forked.ID)
	exported, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: session.ID})
	if err != nil || exported.Artifact == nil {
		t.Fatalf("expected a portable Session artifact: %v", err)
	}
	if _, err := api.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	assertRemoved(session.ID)
	if _, err := api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *exported.Artifact}); err != nil {
		t.Fatal(err)
	}
	assertOriginal(session.ID)
	if err := api.DeleteSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	assertRemoved(session.ID)
	assertOriginal(forked.ID)
}
