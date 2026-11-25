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
	script := &projectMemoryScript{projectFact: projectFact, systems: map[string]string{}}
	model := delegateRestartModel{chat.ModelFunc(script.Call)}
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

	script.mu.Lock()
	defer script.mu.Unlock()
	if !strings.Contains(script.systems["remember the vendor shortlist"], instructions) {
		t.Fatalf("Project instructions missing from its Run: %q", script.systems["remember the vendor shortlist"])
	}
	if strings.Contains(script.systems["search what this project knows"], instructions) {
		t.Fatal("another Project's instructions reached this Run")
	}
	if script.searched == "" || strings.Contains(script.searched, projectFact) || strings.Contains(script.searched, directoryFact) {
		t.Fatalf("a Project script.searched another scope's memory: %s", script.searched)
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

// projectMemoryScript adds the project fact in one Run and searches memory in
// another, recording each task's system prompt and the search result.
type projectMemoryScript struct {
	projectFact string
	mu          sync.Mutex
	systems     map[string]string
	searched    string
}

func (s *projectMemoryScript) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	task := ""
	for _, message := range request.Messages {
		if message.Role == chat.RoleUser && task == "" {
			task = message.Text()
		}
	}
	s.mu.Lock()
	s.systems[task] = request.Messages[0].Text()
	s.mu.Unlock()
	call := chat.ToolCall{ID: "call", Name: "memory_add_memory", Arguments: `{"scope":"project","content":"` + s.projectFact + `"}`}
	if strings.Contains(task, "search") {
		call = chat.ToolCall{ID: "call", Name: "memory_" + memorycontract.SearchToolName, Arguments: `{"query":"payments vendor shortlist directory release checklist"}`}
	}
	if result := toolResultNamed(request, call.Name); result != nil {
		return s.settle(call.Name, result)
	}
	if len(request.Tools) == 0 {
		return completedTextResponse("no tools"), nil
	}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(deferredToolCall(request, "discover", call)))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
}

func (s *projectMemoryScript) settle(name string, result *chat.ToolResult) (*chat.Response, error) {
	encoded, err := json.Marshal(result.Output)
	if err != nil || result.IsError {
		return nil, fmt.Errorf("%s failed: %s: %v", name, encoded, err)
	}
	if name != "memory_add_memory" {
		s.mu.Lock()
		s.searched = string(encoded)
		s.mu.Unlock()
	}
	return completedTextResponse("done"), nil
}
