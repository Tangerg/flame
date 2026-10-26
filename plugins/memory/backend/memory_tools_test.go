package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	tool "github.com/Tangerg/scope/core/tool"
)

func callMemoryModelTool(t *testing.T, session *sdk.ClientSession, project, id, name string, input any) (*sdk.CallToolResult, jsonvalue.Value) {
	t.Helper()
	var meta sdk.Meta
	if err := meta.Set(pluginsdk.InvocationMetadataKey, extension.Invocation{ID: id, Origin: extension.Model, Target: extension.Target{Scope: extension.ScopeInstallation, Reference: "memory-owner"}}); err != nil {
		t.Fatal(err)
	}
	if err := meta.Set(pluginsdk.ToolContextMetadataKey, pluginsdk.ToolContext{CWD: project}); err != nil {
		t.Fatal(err)
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: meta, Name: name, Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	return result, admitted
}

func memoryToolValue[T any](t *testing.T, result *sdk.CallToolResult) T {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool failed: %+v", result)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertMemoryRevisionAdmission[T any](t *testing.T, decision bool) {
	t.Helper()
	executable, err := tool.NewFunc[T, rpc.Ack](tool.FuncConfig{Name: "mutation"}, func(_ context.Context, _ T) (rpc.Ack, error) {
		t.Fatal("admission executed a mutation")
		return rpc.Ack{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := tool.Bind(executable)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"scope": "project", "id": "mem_00000000000000000000000000000001"}
	if decision {
		input["decision"] = "approve"
	}
	for _, revision := range []any{nil, uint64(0), uint64(exactint.Maximum + 1), uint64(1)} {
		if revision != nil {
			input["expected_revision"] = revision
		}
		encoded, err := jsonvalue.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		_, err = bound.Contract().Prepare(chat.ToolCall{ID: "admission", Name: "mutation", Arguments: encoded.String()}, nil)
		if (err == nil) != (revision == uint64(1)) {
			t.Fatalf("revision %v admission: %v", revision, err)
		}
	}
}

func TestMemoryMutationToolsRequireTheObservedExactRevision(t *testing.T) {
	assertMemoryRevisionAdmission[updateMemoryArgs](t, false)
	assertMemoryRevisionAdmission[memoryItemArgs](t, false)
	assertMemoryRevisionAdmission[reviewMemoryArgs](t, true)
}

func TestMemoryContentConflictHasOneDurableRefusalForHumanAndModelCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, session, closeBackend := connectMemoryBackend(t, path)
	now := time.Now().UTC()
	first, _, err := store.Add(t.Context(), memory.ScopeUser, "", "first", now)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.Add(t.Context(), memory.ScopeUser, "", "second", now)
	if err != nil {
		t.Fatal(err)
	}
	humanInput := rpc.UpdateInput{ID: first.ID().String(), ExpectedRevision: first.Revision(), Content: new(second.Content()), Pinned: new(true)}
	_, err = callMemoryCapability[rpc.UpdateInput, memory.ItemSnapshot](t.Context(), session, memoryContract(t), "memory", rpc.Update, "human-collision", humanInput, sdk.ParamsBase{})
	refusal, known := errors.AsType[*extension.Refusal](err)
	if !known || refusal.Code != "duplicate_content" {
		t.Fatalf("human content conflict remained unconfirmed: %v", err)
	}
	model, admitted := callMemoryModelTool(t, session, "", "model-collision", "update_memory", updateMemoryArgs{memoryItemArgs: memoryItemArgs{Scope: memory.ScopeUser, ID: first.ID().String(), ExpectedRevision: first.Revision()}, Content: new(second.Content()), Pinned: new(true)})
	if !model.IsError {
		t.Fatal("model content conflict was accepted")
	}
	before, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model-collision", "update_memory", admitted)
	if _, failed := errors.AsType[*tool.Failure](err); !failed || before == nil {
		t.Fatalf("model content conflict has no original refusal: %+v %v", before, err)
	}
	if err := store.Delete(t.Context(), second.ID(), second.Revision()); err != nil {
		t.Fatal(err)
	}
	closeBackend()
	store, session, _ = connectMemoryBackend(t, path)
	_, found, err := receiptMemoryCapability[rpc.UpdateInput, memory.ItemSnapshot](t.Context(), session, memoryContract(t), "memory", rpc.Update, "human-collision", humanInput)
	refusal, known = errors.AsType[*extension.Refusal](err)
	if !found || !known || refusal.Code != "duplicate_content" {
		t.Fatalf("restart lost the human refusal: found=%v %v", found, err)
	}
	after, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model-collision", "update_memory", admitted)
	if _, failed := errors.AsType[*tool.Failure](err); !failed || !reflect.DeepEqual(after, before) {
		t.Fatalf("restart reevaluated the model refusal: %+v %v", after, err)
	}
	stored, exists, err := store.Get(t.Context(), first.ID())
	if err != nil || !exists || stored.Content() != first.Content() || stored.Revision() != first.Revision() || stored.Pinned() {
		t.Fatalf("content conflict changed the original item: %+v %v", stored, err)
	}
}

func TestMemoryToolsShareTheOwnerAndRecoverOriginalResultsAfterRestart(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(t.TempDir(), "memory.db")
	store, session, closeBackend := connectMemoryBackend(t, path)
	add, addInput := callMemoryModelTool(t, session, project, "add", "add_memory", addMemoryArgs{Scope: memory.ScopeProject, Content: "original memory marker"})
	original := memoryToolValue[memoryView](t, add)
	if original.ID == "" || original.Revision != 1 || original.Status != memory.StatusActive {
		t.Fatalf("added item: %+v", original)
	}
	update, updateInput := callMemoryModelTool(t, session, project, "update", "update_memory", updateMemoryArgs{memoryItemArgs: memoryItemArgs{Scope: original.Scope, ID: original.ID, ExpectedRevision: original.Revision}, Content: new("corrected memory marker"), Pinned: new(true)})
	updated := memoryToolValue[memoryView](t, update)
	if updated.ID != original.ID || updated.Revision != original.Revision+1 || !updated.Pinned {
		t.Fatalf("updated item: %+v", updated)
	}
	human, err := callMemoryCapability[rpc.UpdateInput, memory.ItemSnapshot](t.Context(), session, memoryContract(t), "memory", rpc.Update, "human", rpc.UpdateInput{ID: updated.ID, ExpectedRevision: updated.Revision, Content: new("human memory marker")}, sdk.ParamsBase{})
	if err != nil {
		t.Fatal(err)
	}
	stale, staleInput := callMemoryModelTool(t, session, project, "stale", "update_memory", updateMemoryArgs{memoryItemArgs: memoryItemArgs{Scope: original.Scope, ID: original.ID, ExpectedRevision: updated.Revision}, Content: new("stale replacement")})
	if !stale.IsError {
		t.Fatal("stale model correction overwrote the human edit")
	}
	listed, _ := callMemoryModelTool(t, session, project, "list", "list_memories", listMemoryArgs{Scope: memory.ScopeProject})
	page := memoryToolValue[memoryPage](t, listed)
	if len(page.Memories) != 1 || page.Memories[0].Revision != human.Revision || page.Memories[0].Content != human.Content {
		t.Fatalf("model and human reads disagree: %+v", page)
	}
	search, _ := callMemoryModelTool(t, session, project, "search", "search_memory", agentMemorySearchRequest{Query: "marker"})
	found := memoryToolValue[memoryResults](t, search)
	if len(found.Memories) != 1 || found.Memories[0].ID != original.ID || found.Memories[0].Revision != human.Revision {
		t.Fatalf("search omitted the editable identity or used a different workspace key: %+v", found)
	}
	remove, removeInput := callMemoryModelTool(t, session, project, "delete", "delete_memory", memoryItemArgs{Scope: original.Scope, ID: original.ID, ExpectedRevision: human.Revision})
	memoryToolValue[rpc.Ack](t, remove)
	items, err := store.List(t.Context(), memory.ScopeProject, original.Project)
	if err != nil || len(items) != 0 {
		t.Fatalf("model removal did not reach the owner: %+v, %v", items, err)
	}
	type proof struct {
		id, method string
		input      jsonvalue.Value
		output     *chat.ToolOutput
		failed     bool
	}
	proofs := []proof{{id: "add", method: "add_memory", input: addInput}, {id: "update", method: "update_memory", input: updateInput}, {id: "delete", method: "delete_memory", input: removeInput}, {id: "stale", method: "update_memory", input: staleInput, failed: true}}
	for index := range proofs {
		p := &proofs[index]
		p.output, err = pluginsdk.ReadToolReceipt(t.Context(), session, p.id, p.method, p.input)
		_, failed := errors.AsType[*tool.Failure](err)
		if p.output == nil || failed != p.failed || (err != nil && !failed) {
			t.Fatalf("missing original %s result: %+v, %v", p.id, p.output, err)
		}
	}
	closeBackend()
	_, session, _ = connectMemoryBackend(t, path)
	for _, p := range proofs {
		output, err := pluginsdk.ReadToolReceipt(t.Context(), session, p.id, p.method, p.input)
		_, failed := errors.AsType[*tool.Failure](err)
		if !reflect.DeepEqual(output, p.output) || failed != p.failed || (err != nil && !failed) {
			t.Fatalf("restart changed original %s result: %+v, %v", p.id, output, err)
		}
	}
}

func TestMemoryToolsReviewProposalsAndFenceTheCapturedProject(t *testing.T) {
	project, other := t.TempDir(), t.TempDir()
	store, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	facts, err := store.AppendLedger(t.Context(), memory.FactBatch{Project: canonical, SessionID: "ses_review", Day: now.Format(time.DateOnly), Facts: []string{"proposal to approve", "proposal to reject"}, CapturedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := memory.NewPublication(canonical, memory.State{}, facts[len(facts)-1].Sequence, []string{"proposal to approve", "proposal to reject"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if published, err := store.Reconcile(t.Context(), publication); err != nil || !published {
		t.Fatalf("publish proposals: %v, %v", published, err)
	}
	listed, _ := callMemoryModelTool(t, session, project, "proposals", "list_memories", listMemoryArgs{Scope: memory.ScopeProject})
	page := memoryToolValue[memoryPage](t, listed)
	if len(page.Memories) != 2 {
		t.Fatalf("pending proposals are not manageable: %+v", page)
	}
	for _, item := range page.Memories {
		decision := memory.ReviewApprove
		if strings.Contains(item.Content, "reject") {
			decision = memory.ReviewReject
		}
		result, admitted := callMemoryModelTool(t, session, project, item.ID, "review_memory", reviewMemoryArgs{memoryItemArgs: memoryItemArgs{Scope: item.Scope, ID: item.ID, ExpectedRevision: item.Revision}, Decision: decision})
		memoryToolValue[rpc.Ack](t, result)
		id, err := memory.ParseItemID(item.ID)
		if err != nil {
			t.Fatal(err)
		}
		stored, found, err := store.Get(t.Context(), id)
		status, _ := decision.Result()
		if err != nil || !found || stored.Status() != status || stored.Revision() != item.Revision+1 {
			t.Fatalf("review did not reach the owner: %+v, %v", stored, err)
		}
		if output, err := pluginsdk.ReadToolReceipt(t.Context(), session, item.ID, "review_memory", admitted); err != nil || output == nil {
			t.Fatalf("review receipt is unavailable: %+v, %v", output, err)
		}
	}
	foreign, _, err := store.Add(t.Context(), memory.ScopeProject, other, "foreign fact", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"update_memory", "delete_memory", "review_memory"} {
		input := map[string]any{"scope": "project", "id": foreign.ID().String(), "expected_revision": foreign.Revision()}
		if name == "update_memory" {
			input["content"] = "cross-project overwrite"
		}
		if name == "review_memory" {
			input["decision"] = "reject"
		}
		result, _ := callMemoryModelTool(t, session, project, "foreign-"+name, name, input)
		if !result.IsError {
			t.Fatalf("%s escaped its captured project", name)
		}
	}
	stored, found, err := store.Get(t.Context(), foreign.ID())
	if err != nil || !found || stored.Content() != foreign.Content() || stored.Revision() != foreign.Revision() {
		t.Fatalf("foreign memory changed: %+v, %v", stored, err)
	}
}

func TestMemoryInputRefusalHasAReceiptAndCorruptStateIsNotARefusal(t *testing.T) {
	_, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	result, admitted := callMemoryModelTool(t, session, t.TempDir(), "invalid", "add_memory", addMemoryArgs{Scope: memory.ScopeUser, Content: "   "})
	if !result.IsError {
		t.Fatal("blank memory was admitted")
	}
	output, err := pluginsdk.ReadToolReceipt(t.Context(), session, "invalid", "add_memory", admitted)
	if _, known := errors.AsType[*tool.Failure](err); !known || output == nil {
		t.Fatalf("input refusal became an unknown outcome: %+v, %v", output, err)
	}
	_, corrupt := memory.RestoreItem(memory.ItemSnapshot{})
	if corrupt == nil || classify(corrupt) != "" {
		t.Fatalf("corrupt stored state became a business refusal: %v", corrupt)
	}
}

func TestMemoryToolsResolveWorkspaceAliasesConsistently(t *testing.T) {
	project := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	_, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	added, _ := callMemoryModelTool(t, session, alias, "alias-add", "add_memory", addMemoryArgs{Scope: memory.ScopeProject, Content: "workspace alias marker"})
	item := memoryToolValue[memoryView](t, added)
	result, _ := callMemoryModelTool(t, session, project, "canonical-search", "search_memory", agentMemorySearchRequest{Query: "alias"})
	matched := memoryToolValue[memoryResults](t, result)
	if len(matched.Memories) != 1 || matched.Memories[0].ID != item.ID {
		t.Fatalf("search used a competing workspace key: %+v", matched)
	}
}
