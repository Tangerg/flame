package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"iter"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// Two Projects that share one repository must not share what the agent learns or
// is told there, and neither may implicitly read the directory's older memories.
func TestProjectsInOneWorkspaceKeepInstructionsAndMemoryApart(t *testing.T) {
	const (
		directoryFact = "directory release checklist"
		projectFact   = "payments vendor shortlist"
		instructions  = "Answer as the payments review lead."
	)
	var (
		mu       sync.Mutex
		systems  = map[string]string{}
		searched string
	)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		task := ""
		for _, message := range request.Messages {
			if message.Role == chat.RoleUser && task == "" {
				task = message.Text()
			}
		}
		mu.Lock()
		systems[task] = request.Messages[0].Text()
		mu.Unlock()
		name, arguments := "memory_add_memory", `{"scope":"project","content":"`+projectFact+`"}`
		if strings.Contains(task, "search") {
			name, arguments = "memory_"+memorycontract.SearchToolName, `{"query":"payments vendor shortlist directory release checklist"}`
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult == nil || part.ToolResult.Name != name {
					continue
				}
				encoded, err := json.Marshal(part.ToolResult.Output)
				if err != nil || part.ToolResult.IsError {
					return nil, fmt.Errorf("%s failed: %s: %v", name, encoded, err)
				}
				if name != "memory_add_memory" {
					mu.Lock()
					searched = string(encoded)
					mu.Unlock()
				}
				return completedTextResponse("done"), nil
			}
		}
		if len(request.Tools) == 0 {
			return completedTextResponse("no tools"), nil
		}
		call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call = chat.ToolCall{ID: "call", Name: name, Arguments: arguments}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	cfg.approvalMode = approval.ModeYolo
	r := poMCPOpen(t, cfg)
	defer r.close()
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "memory"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(server, memoryToolsReady)
	mustMemoryAdd(r, memoryAddRequest(r, memorycontract.AddInput{Scope: memorycontract.AddInputScopeProject, Cwd: cfg.defaultWorkspacePath, Content: directoryFact}), "directory-fact")

	payments := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Payments", Instructions: instructions}, "payments").(*protocol.Project)
	compliance := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Compliance"}, "compliance").(*protocol.Project)
	run := func(project *protocol.Project, task string) {
		t.Helper()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{
			Title: task, Workspace: &protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}, ProjectID: project.ID,
		}, "session-"+project.ID).(*protocol.Session)
		started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{
			SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: task}},
		}, protocol.CallMetadata{IdempotencyKey: "start-" + project.ID, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if started.Failure != nil {
			t.Fatal(started.Failure)
		}
		events := iter.Seq2[protocol.RunEvent, error](func(yield func(protocol.RunEvent, error) bool) {
			for event, err := range started.Events {
				if !yield(event.(protocol.RunEvent), err) {
					return
				}
			}
		})
		waitForRunEvents(t, collectRunEvents(events), task)
	}
	run(payments, "remember the vendor shortlist")
	run(compliance, "search what this project knows")

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(systems["remember the vendor shortlist"], instructions) {
		t.Fatalf("Project instructions missing from its Run: %q", systems["remember the vendor shortlist"])
	}
	if strings.Contains(systems["search what this project knows"], instructions) {
		t.Fatal("another Project's instructions reached this Run")
	}
	if searched == "" || strings.Contains(searched, projectFact) || strings.Contains(searched, directoryFact) {
		t.Fatalf("a Project searched another scope's memory: %s", searched)
	}
	listed := func(input memorycontract.ListInput) []string {
		t.Helper()
		var contents []string
		for _, item := range readMemoryPage(r, input).Data {
			contents = append(contents, item.Content)
		}
		return contents
	}
	if got := listed(memorycontract.ListInput{Scope: memorycontract.ListInputScopeProject, Project: &payments.ID}); !slices.Equal(got, []string{projectFact}) {
		t.Fatalf("Project memory = %v", got)
	}
	if got := listed(memorycontract.ListInput{Scope: memorycontract.ListInputScopeProject, Cwd: cfg.defaultWorkspacePath}); !slices.Equal(got, []string{directoryFact}) {
		t.Fatalf("directory memory after Project work = %v", got)
	}
}
