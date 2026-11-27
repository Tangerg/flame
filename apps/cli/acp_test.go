package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/acp"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestProductionACPAgent(t *testing.T) {
	binary := buildProductionBinary(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const cancelPrompt = "ACP streaming cancellation probe"
	const fragment = "visible before cancellation"
	providerStopped := make(chan struct{}, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request struct {
			Stream   bool             `json:"stream"`
			Messages jsontext.Value   `json:"messages"`
			Tools    []jsontext.Value `json:"tools"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
			return
		}
		if request.Stream && len(request.Tools) > 0 && bytes.Contains(request.Messages, []byte(cancelPrompt)) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"id\":\"chatcmpl_cancel\",\"object\":\"chat.completion.chunk\",\"model\":\"deepseek-chat\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":null}]}\n\n", fragment)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				select {
				case providerStopped <- struct{}{}:
				case <-ctx.Done():
				}
			case <-ctx.Done():
			}
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		runtimefixture.ServeDelegatedApproval(w, r)
	}))
	t.Cleanup(provider.Close)
	environment := terminalTestEnvironment(t, map[string]string{
		"FLAME_PROVIDER": "deepseek", "FLAME_MODEL": "deepseek-chat",
		"FLAME_BASEURL": provider.URL, "FLAME_APIKEY": "test-key",
		"FLAME_RUNTIME_CONFIG_DIR": t.TempDir(), "FLAME_RUNTIME_TOKEN": "",
	})
	workspace := t.TempDir()

	updates := make(chan *acp.SessionNotification, 128)
	permissions := make(chan *acp.RequestPermissionRequest, 4)
	var holdPermission atomic.Bool
	client, err := acp.NewClient(&acp.ClientConfig{
		SessionUpdate: func(ctx context.Context, _ *acp.ClientConn, note *acp.SessionNotification) {
			select {
			case updates <- note:
			case <-ctx.Done():
			}
		},
		RequestPermission: func(ctx context.Context, _ *acp.ClientConn, request *acp.RequestPermissionRequest) (*acp.RequestPermissionResponse, error) {
			select {
			case permissions <- request:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if holdPermission.Load() {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			for _, option := range request.Options {
				if option.Kind == acp.PermissionOptionKindAllowOnce {
					return &acp.RequestPermissionResponse{Outcome: &acp.SelectedPermissionOutcome{OptionID: option.OptionID}}, nil
				}
			}
			return nil, errors.New("permission request has no allow-once option")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connect := func() *acp.ClientConn {
		t.Helper()
		command := exec.Command(binary, "--runtime-mode", "embedded", "acp")
		command.Dir, command.Env, command.Stderr = workspace, environment, os.Stderr
		conn, err := client.Connect(ctx, acp.NewCommandTransport(&acp.CommandConfig{Command: command}))
		if err != nil {
			t.Fatalf("initialize flame acp: %v", err)
		}
		t.Cleanup(func() {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		})
		return conn
	}
	conn := connect()
	info, _ := conn.Negotiated().AgentInfo.Get()
	if info.Name != "flame" {
		t.Fatalf("agent info = %+v", info)
	}

	// A session setup request must not install or run editor-supplied commands.
	if _, _, err := conn.NewSession(ctx, &acp.NewSessionRequest{
		Cwd: workspace, McpServers: []acp.McpServer{&acp.McpServerStdio{
			Name: "unapproved", Command: filepath.Join(workspace, "mcp-server"), Args: []string{}, Env: []acp.EnvVariable{},
		}},
	}); !errors.Is(err, &acp.Error{Code: acp.ErrorCodeInvalidParams}) {
		t.Fatalf("session with a client-supplied MCP server = %v", err)
	}

	open := func(cwd string) *acp.ClientSession {
		t.Helper()
		session, _, err := conn.NewSession(ctx, &acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
		if err != nil {
			t.Fatalf("create ACP session: %v", err)
		}
		return session
	}
	session := open(workspace)
	padded := filepath.Join(t.TempDir(), "workspace ")
	if err := os.Mkdir(padded, 0700); err != nil {
		t.Fatal(err)
	}
	listOnly := open(padded)
	paddedPage, err := conn.ListSessions(ctx, &acp.ListSessionsRequest{Cwd: acp.OptValue(padded)})
	if err != nil || len(paddedPage.Sessions) != 1 || paddedPage.Sessions[0].SessionID != listOnly.ID() {
		t.Fatalf("list ACP sessions in workspace with trailing space = %+v, %v", paddedPage, err)
	}
	page, err := conn.ListSessions(ctx, &acp.ListSessionsRequest{Cwd: acp.OptValue(workspace)})
	if err != nil || len(page.Sessions) != 1 || page.Sessions[0].SessionID != session.ID() {
		t.Fatalf("list ACP sessions = %+v, %v", page, err)
	}

	reply, err := session.Prompt(ctx, &acp.PromptParams{Prompt: []acp.ContentBlock{
		&acp.TextContent{Text: runtimefixture.DelegateApprovalProbe},
		&acp.ResourceLink{Name: "Reference", URI: "https://example.invalid/reference"},
	}})
	if err != nil || reply.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("approved ACP prompt = %+v, %v", reply, err)
	}
	permission := receiveACP(t, ctx, permissions)
	if permission.SessionID != session.ID() {
		t.Fatalf("permission session = %s, want %s", permission.SessionID, session.ID())
	}
	turn := drainACPUpdates(updates, session.ID())
	if text := acpAssistantText(turn); text != "root complete" {
		t.Fatalf("ACP assistant chunks = %q", text)
	}
	if !slices.ContainsFunc(turn, func(update acp.SessionUpdate) bool {
		change, ok := update.(*acp.ToolCallUpdate)
		if !ok {
			return false
		}
		status, _ := change.Status.Get()
		content, _ := change.Content.Get()
		return change.ToolCallID == permission.ToolCall.ToolCallID && status == acp.ToolCallStatusCompleted &&
			slices.ContainsFunc(content, func(entry acp.ToolCallContent) bool {
				result, ok := entry.(*acp.Content)
				return ok && strings.Contains(acpContentText(result.Content), "approved")
			})
	}) {
		t.Fatalf("ACP tool result did not complete the approved call %s", permission.ToolCall.ToolCallID)
	}

	streaming := open(workspace)
	startACPCancelPrompt(t, ctx, streaming, cancelPrompt, func() {
		for {
			note := receiveACP(t, ctx, updates)
			update, ok := note.Update.(*acp.AgentMessageChunk)
			if note.SessionID == streaming.ID() && ok && acpContentText(update.Content) == fragment {
				return
			}
		}
	})
	receiveACP(t, ctx, providerStopped)

	holdPermission.Store(true)
	waiting := open(workspace)
	startACPCancelPrompt(t, ctx, waiting, runtimefixture.DelegateApprovalProbe, func() {
		if request := receiveACP(t, ctx, permissions); request.SessionID != waiting.ID() {
			t.Fatalf("permission session = %s, want %s", request.SessionID, waiting.ID())
		}
	})
	holdPermission.Store(false)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range []struct {
		name string
		id   acp.SessionID
		want protocol.RunOutcomeType
	}{
		{"streaming", streaming.ID(), protocol.OutcomeLost},
		{"waiting", waiting.ID(), protocol.OutcomeCanceled},
	} {
		runs := decodeCommandJSON[struct {
			Items []struct {
				Status  protocol.RunStatus `json:"status"`
				Outcome struct {
					Status protocol.RunOutcomeType `json:"status"`
				} `json:"outcome"`
			} `json:"items"`
		}](t, runTestBinary(t, binary, environment, "runs", "ls", "--session", string(scenario.id), "--json"), "cancelled ACP run")
		if len(runs.Items) != 1 || runs.Items[0].Status != protocol.RunStatusFinished || runs.Items[0].Outcome.Status != scenario.want {
			t.Fatalf("durable run after cancelling a %s ACP session %s = %+v, want %s", scenario.name, scenario.id, runs.Items, scenario.want)
		}
	}

	conn = connect()
	if _, _, err := conn.LoadSession(ctx, &acp.LoadSessionRequest{
		SessionID: session.ID(), Cwd: t.TempDir(), McpServers: []acp.McpServer{},
	}); !errors.Is(err, &acp.Error{Code: acp.ErrorCodeInvalidParams}) {
		t.Fatalf("load with a different cwd = %v", err)
	}
	if _, _, err := conn.LoadSession(ctx, &acp.LoadSessionRequest{SessionID: session.ID(), Cwd: workspace, McpServers: []acp.McpServer{}}); err != nil {
		t.Fatalf("load ACP session after restart: %v", err)
	}
	replay := drainACPUpdates(updates, session.ID())
	if text := acpAssistantText(replay); text != "root complete" {
		t.Fatalf("replayed ACP assistant chunks = %q", text)
	}
	if !slices.ContainsFunc(replay, func(update acp.SessionUpdate) bool {
		message, ok := update.(*acp.UserMessageChunk)
		return ok && strings.Contains(acpContentText(message.Content), "https://example.invalid/reference")
	}) {
		t.Fatal("replayed ACP history omitted the original resource link")
	}
	if _, _, err := conn.ResumeSession(ctx, &acp.ResumeSessionRequest{
		SessionID: session.ID(), Cwd: workspace, McpServers: []acp.McpServer{},
	}); err != nil {
		t.Fatalf("resume ACP session: %v", err)
	}
	if updates := drainACPUpdates(updates, session.ID()); len(updates) != 0 {
		t.Fatalf("resuming an ACP session replayed %d updates", len(updates))
	}
}

func startACPCancelPrompt(t *testing.T, ctx context.Context, session *acp.ClientSession, text string, waitForTurn func()) {
	t.Helper()
	signal := make(chan error, 1)
	go func() {
		reply, err := session.Prompt(ctx, &acp.PromptParams{Prompt: []acp.ContentBlock{&acp.TextContent{Text: text}}})
		if err == nil && reply.StopReason != acp.StopReasonCancelled {
			err = fmt.Errorf("stop reason = %s, want cancelled", reply.StopReason)
		}
		signal <- err
	}()
	waitForTurn()
	if err := session.Cancel(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := receiveACP(t, ctx, signal); err != nil {
		t.Fatalf("wait for cancelled ACP prompt: %v", err)
	}
}

func receiveACP[T any](t *testing.T, ctx context.Context, events <-chan T) T {
	t.Helper()
	select {
	case value := <-events:
		return value
	case <-ctx.Done():
		t.Fatalf("waiting for ACP event: %v", ctx.Err())
		var zero T
		return zero
	}
}

func drainACPUpdates(events <-chan *acp.SessionNotification, session acp.SessionID) (updates []acp.SessionUpdate) {
	for {
		select {
		case note := <-events:
			if note.SessionID == session {
				updates = append(updates, note.Update)
			}
		default:
			return updates
		}
	}
}

func acpAssistantText(updates []acp.SessionUpdate) string {
	var text strings.Builder
	for _, update := range updates {
		if chunk, ok := update.(*acp.AgentMessageChunk); ok {
			text.WriteString(acpContentText(chunk.Content))
		}
	}
	return text.String()
}

func acpContentText(content acp.ContentBlock) string {
	if text, ok := content.(*acp.TextContent); ok {
		return text.Text
	}
	return ""
}
