package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/scope/core/chat"
	scopefs "github.com/Tangerg/scope/tools/fs"
)

func TestApplyPatchReceiptReportsTheAppliedFileAndHunks(t *testing.T) {
	workspace, git := taskWorktreeTestRepository(t)
	const name = "material.txt"
	file := filepath.Join(workspace, name)
	if err := os.WriteFile(file, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", name)
	git("commit", "--quiet", "-m", "original material")
	patch := fmt.Sprintf("--- %[1]s\n+++ %[1]s\n@@ -1 +1 @@\n-original\n+updated\n", name)
	arguments, err := json.Marshal(scopefs.ApplyPatchRequest{Patch: patch})
	if err != nil {
		t.Fatal(err)
	}
	const readID, patchID = "read_material", "patch_material"
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("Patch material"), nil
		}
		call := chat.ToolCall{ID: readID, Name: string(builtin.Read), Arguments: fmt.Sprintf(`{"path":%q}`, name)}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult == nil {
					continue
				}
				switch part.ToolResult.ID {
				case readID:
					call = chat.ToolCall{ID: patchID, Name: string(builtin.ApplyPatch), Arguments: string(arguments)}
				case patchID:
					return completedTextResponse("Updated the material."), nil
				}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}}, nil
	})}
	_, runtime, requestContext, _ := newSessionStateE2ERuntime(t, model)
	ctx, cancel := context.WithTimeout(requestContext, lifecycleWaitBudget(t))
	defer cancel()
	api := runtime.api
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: workspace}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Update the material."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "apply the patch")
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID}})
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(items.Data, func(item protocol.Item) bool {
		return item.Tool != nil && item.Tool.Name == string(builtin.ApplyPatch)
	})
	if index < 0 {
		t.Fatal("the applied patch has no Tool receipt")
	}
	encoded, err := json.Marshal(items.Data[index].Tool.Result)
	if err != nil {
		t.Fatal(err)
	}
	if got := git("diff", "--numstat", "HEAD", "--", name); got != "1\t1\t"+name {
		t.Fatalf("Git found %q, Tool returned %s with %+v; want one replaced line in %q", got, encoded, items.Data[index].Error, name)
	}
	var receipt builtin.PatchResult
	want := []builtin.AppliedChange{{Path: name, Status: builtin.Modified, Hunks: 1}}
	if err := json.Unmarshal(encoded, &receipt, json.RejectUnknownMembers(true)); err != nil || !slices.Equal(receipt.Changes, want) {
		t.Fatalf("patch receipt %s, %v; want one applied hunk in %q", encoded, err, name)
	}
}
