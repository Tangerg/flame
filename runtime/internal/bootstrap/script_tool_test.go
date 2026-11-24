package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// A script sees the Run's read-only Tools under the Run's admission, the model
// sees only what the script returns, and script reads grant no write authority.
func TestRunScriptReadsOnlyThroughRunAdmission(t *testing.T) {
	const source = `
const reads = await Promise.allSettled(["alpha.txt", "beta.txt"].map((path) => tools.read({ path })));
return reads.map((read) => read.status === "fulfilled" ? read.value.content.trim().split(" ").length : read.reason.status);`
	var turns []string
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		results := map[string]*chat.ToolResult{}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					results[part.ToolResult.ID] = part.ToolResult
				}
			}
		}
		var call chat.ToolCall
		switch len(results) {
		case 0:
			arguments, err := json.Marshal(map[string]any{"summary": "Count note words", "script": source, "tools": []string{"read"}})
			if err != nil {
				return nil, err
			}
			call = chat.ToolCall{ID: "script", Name: string(builtin.RunScript), Arguments: string(arguments)}
		case 1:
			text, _ := results["script"].Output.Text()
			turns = append(turns, text)
			call = chat.ToolCall{ID: "edit", Name: "edit", Arguments: `{"path":"alpha.txt","old_string":"first","new_string":"changed"}`}
		default:
			text, _ := results["edit"].Output.Text()
			turns = append(turns, fmt.Sprintf("%t %s", results["edit"].IsError, text))
			return completedTextResponse("done"), nil
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}}, nil
	})}
	_, api, ctx, home := newSessionStateE2ERuntime(t, model)
	for name, text := range map[string]string{"alpha.txt": "first secret line", "beta.txt": "second secret"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}, Title: "script"})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.SetApprovalRule(ctx, protocol.SetApprovalRuleRequest{
		SessionID: session.ID, Scope: protocol.ApprovalRuleScopeSession,
		Tool:    protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: string(builtin.Read)},
		Subject: protocol.ApprovalSubject{Type: protocol.ApprovalSubjectExact, Value: "beta.txt"}, Decision: protocol.ApprovalRuleDecisionDeny,
	}); err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "count"}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "script")

	want := []string{`[3,"rejected"]` + "\n\nTool calls: 1 succeeded, 1 rejected."}
	if len(turns) != 2 || turns[0] != want[0] || !strings.HasPrefix(turns[1], "true ") || strings.Contains(turns[0], "secret") {
		t.Fatalf("model observed %q", turns)
	}
	if content, err := os.ReadFile(filepath.Join(home, "alpha.txt")); err != nil || string(content) != "first secret line" {
		t.Fatalf("alpha.txt = %q, %v", content, err)
	}
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID}})
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(items.Data, func(item protocol.Item) bool {
		return item.Tool != nil && item.Tool.Name == string(builtin.RunScript)
	})
	if index < 0 {
		t.Fatal("run_script item is missing")
	}
	encoded, err := json.Marshal(items.Data[index].Tool.Result)
	if err != nil {
		t.Fatal(err)
	}
	var result builtin.ScriptResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, call := range result.Calls {
		calls = append(calls, fmt.Sprintf("%s %s %s %t", call.Tool, call.Source, call.Status, call.Result != nil))
	}
	if result.Status != builtin.ScriptCompleted || strings.Join(calls, "; ") != "read builtIn:read succeeded true; read builtIn:read rejected false" {
		t.Fatalf("script record = %s", encoded)
	}
}

// An installed package's read-only Tools reach a script through the installed
// process after the user approves the script once; its writes cannot be declared.
func TestRunScriptCallsInstalledReadsAfterOneApproval(t *testing.T) {
	var declared atomic.Value
	declared.Store([2]string{})
	var observed sync.Map
	scriptCall := func(id, summary, source, name string) (*chat.Response, error) {
		arguments, err := json.Marshal(map[string]any{"summary": summary, "script": source, "tools": []string{name}})
		if err != nil {
			return nil, err
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: id, Name: string(builtin.RunScript), Arguments: string(arguments)}))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	}
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					text, _ := part.ToolResult.Output.Text()
					observed.Store(part.ToolResult.ID, fmt.Sprintf("%t %s", part.ToolResult.IsError, text))
				}
			}
		}
		names := declared.Load().([2]string)
		if _, done := observed.Load("read"); done {
			return completedTextResponse("done"), nil
		}
		if _, done := observed.Load("write"); done {
			name, err := json.Marshal(names[0])
			if err != nil {
				return nil, err
			}
			return scriptCall("read", "Inspect issue", fmt.Sprintf(`
let issue;
try { await tools[%[1]s]({ repository: "../private", number: 42 }); } catch (error) { issue = error.status + ": " + error.message; }
return { declared: Object.keys(tools), issue, source: describeTool(%[1]s).source };`, name), names[0])
		}
		return scriptCall("write", "Open issue", "return 1;", names[1])
	})}
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeBalanced
	cfg.modelProviders = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "github"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "github"}
	tools := r.poMCPTools(server, func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "get_issue" }) &&
			slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "create_issue" })
	})
	names := map[string]string{}
	for _, tool := range tools {
		names[tool.Name] = tool.ModelName
	}
	declared.Store([2]string{names["get_issue"], names["create_issue"]})

	meta := protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}}}
	drain := func(result protocol.OperationResult) {
		t.Helper()
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		for _, err := range result.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "installed script"}, "script-session").(*protocol.Session)
	run := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "inspect"}}}, protocol.CallMetadata{IdempotencyKey: "script-run", RequestMeta: meta})
	drain(run)
	started := run.Value.(*protocol.StartRunResponse)
	pending, err := r.api.ListInterrupts(delivery.WithRequestMeta(t.Context(), meta), protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("pending approvals = %+v, %v", pending, err)
	}
	prompt := pending.Data[0].Interrupts[0]
	if prompt.Payload == nil || prompt.Payload.Tool == nil || prompt.Payload.Tool.Name != string(builtin.RunScript) || prompt.Payload.Reason != "Lets a script call tools that need your approval." {
		t.Fatalf("approval prompt = %+v", prompt.Payload)
	}
	drain(r.endpoint.Invoke(t.Context(), delivery.RunsResume, protocol.ResumeRunRequest{RunID: started.RunID, Responses: []protocol.InterruptResponse{{
		ItemID: prompt.ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove},
	}}}, protocol.CallMetadata{IdempotencyKey: "script-approve", RequestMeta: meta}))

	write, _ := observed.Load("write")
	if want := fmt.Sprintf("true invalid effective arguments: tool: invalid arguments: %q is not a read-only tool a script can call", names["create_issue"]); write != want {
		t.Fatalf("write script = %q, want %q", write, want)
	}
	read, _ := observed.Load("read")
	want := fmt.Sprintf(`false {"declared":[%q],"issue":"failed: github: repository or file path is invalid","source":"mcp:installation:%s:github:get_issue"}`, names["get_issue"], installed.ID) + "\n\nTool calls: 1 failed."
	if read != want {
		t.Fatalf("read script = %q, want %q", read, want)
	}
}
