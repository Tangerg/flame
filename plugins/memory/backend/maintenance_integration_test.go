package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	app "github.com/Tangerg/flame/plugins/memory/internal/application"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

// promptModel adapts a prompt-oriented completion to the chat.Model the host
// grant now expects. The maintenance operation is not on the wire, so it is
// recovered from the unique extract/curate system-prompt markers.
func promptModel(call func(context.Context, app.Prompt) (string, error)) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		var prompt app.Prompt
		if len(request.Messages) == 2 {
			prompt.SystemPrompt = request.Messages[0].Text()
			prompt.UserPrompt = request.Messages[1].Text()
		}
		switch {
		case strings.Contains(prompt.SystemPrompt, "NO_FACTS"):
			prompt.Operation = app.OperationExtract
		case strings.Contains(prompt.SystemPrompt, "NO_MEMORY"):
			prompt.Operation = app.OperationCurate
		}
		text, err := call(ctx, prompt)
		if err != nil {
			return nil, err
		}
		message := chat.NewAssistantMessage(chat.NewTextPart(text))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})
}

func TestBackendMinesAndPublishesOnlyItsPrivateMemoryThroughCapturedModels(t *testing.T) {
	store, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	project, err := (filesystem.DirectoryResolver{}).ResolveExistingDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	operations := []string{}
	grant, release, err := openTestModel(t.Context(), broker, session, promptModel(func(_ context.Context, prompt app.Prompt) (string, error) {
		operations = append(operations, prompt.Operation)
		if prompt.Operation == app.OperationExtract {
			return "- use local workspace mode\n- verify backend restart", nil
		}
		if prompt.Operation == app.OperationCurate && strings.Contains(prompt.UserPrompt, "use local workspace mode") {
			return "- use local workspace mode\n- verify backend restart", nil
		}
		t.Errorf("unexpected model request: %s", prompt.Operation)
		return "", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	input := rpc.MaintenanceInput{SessionID: "ses_mining", Project: project, Compacted: true, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}}
	if _, err := callMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, memoryContract(t), "memory", rpc.Maintain, "mine", input, sdk.ParamsBase{}); err == nil {
		t.Fatal("fresh compacted maintenance accepted missing callback metadata")
	}
	_, err = invokeMaintenance(t, t.Context(), session, "mine", input, grant)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(operations, ",") != "memory.extract,memory.curate" {
		t.Fatalf("model lifecycle=%v", operations)
	}
	state, err := store.State(t.Context(), project)
	if err != nil || state.Watermark != 2 {
		t.Fatalf("private generation=%+v %v", state, err)
	}
	page, err := callMemoryCapability[rpc.ListInput, rpc.Page](t.Context(), session, memoryContract(t), "memory", rpc.List, "", rpc.ListInput{Scope: memory.ScopeProject, CWD: project}, sdk.ParamsBase{})
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("published proposals=%+v %v", page, err)
	}
	for _, item := range page.Rows {
		if item.Status != memory.StatusPending || item.Origin != memory.OriginAuto {
			t.Fatalf("proposal skipped human review=%+v", item)
		}
	}
	release()

	_, err = callMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, memoryContract(t), "memory", rpc.Maintain, "no-compaction", rpc.MaintenanceInput{SessionID: "ses_mining", Project: project, Compacted: false, Conversation: rpc.Conversation{MessageCount: 4}}, sdk.ParamsBase{})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.State(t.Context(), project)
	if err != nil || unchanged != state {
		t.Fatalf("non-compaction advanced memory=%+v %v", unchanged, err)
	}
}

func TestRevokedCurationJoinsModelsAndRestoresPendingLedgerWithoutOverwritingHumanFacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "memory.db")
	project, err := (filesystem.DirectoryResolver{}).ResolveExistingDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, session, closeBackend := connectMemoryBackend(t, path)
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	curating := make(chan struct{})
	joined := make(chan struct{})
	grant, release, err := openTestModel(ctx, broker, session, promptModel(func(ctx context.Context, prompt app.Prompt) (string, error) {
		if prompt.Operation == app.OperationExtract {
			return "- automatic proposal", nil
		}
		if prompt.Operation != app.OperationCurate {
			return "", errors.New("unexpected model operation")
		}
		close(curating)
		<-ctx.Done()
		close(joined)
		return "", ctx.Err()
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	go func() {
		_, err := invokeMaintenance(t, ctx, session, "maintenance", rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}}, grant)
		result <- err
	}()
	select {
	case <-curating:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	added, err := callMemoryCapability[rpc.AddInput, rpc.AddResult](ctx, session, memoryContract(t), "memory", rpc.Add, "human-add", rpc.AddInput{Scope: memory.ScopeProject, CWD: project, Content: "human fact"}, sdk.ParamsBase{})
	if err != nil {
		t.Fatal(err)
	}
	content, pinned := "human edit during curation", true
	human, err := callMemoryCapability[rpc.UpdateInput, memory.ItemSnapshot](ctx, session, memoryContract(t), "memory", rpc.Update, "human-edit", rpc.UpdateInput{ID: added.Item.ID.String(), Content: &content, Pinned: &pinned}, sdk.ParamsBase{})
	if err != nil {
		t.Fatal(err)
	}
	broker.RevokeSession(session)
	select {
	case <-joined:
	default:
		t.Fatal("backend revocation returned before joining model work")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("revoked curation published success")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	state, err := store.State(ctx, project)
	if err != nil || state.Watermark != 0 {
		t.Fatalf("revoked generation=%+v err=%v", state, err)
	}
	pending, err := store.PendingLedger(ctx, project, state.Watermark, memory.MaxLedgerFoldFacts)
	if err != nil || len(pending) != 1 {
		t.Fatalf("revocation lost committed extraction: %+v %v", pending, err)
	}
	closeBackend()
	store, session, _ = connectMemoryBackend(t, path)
	broker.AdmitSession(session)
	originalInput := rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}}
	if _, found, err := receiptMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, memoryContract(t), "memory", rpc.Maintain, "maintenance", originalInput); err != nil || found {
		t.Fatalf("canceled model work became known receipt: %t, %v", found, err)
	}
	if _, err := callMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, memoryContract(t), "memory", rpc.Maintain, "maintenance", originalInput, sdk.ParamsBase{}); err == nil {
		t.Fatal("pending original maintenance restarted model work")
	}
	_, err = invokeMaintenance(t, ctx, session, "uncompacted", rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true}, grant)
	if err == nil {
		t.Fatal("new backend generation reused a revoked model grant")
	}
	fresh, releaseFresh, err := openTestModel(ctx, broker, session, promptModel(func(_ context.Context, prompt app.Prompt) (string, error) {
		if prompt.Operation != app.OperationCurate {
			t.Errorf("pending recovery repeated extraction: %s", prompt.Operation)
		}
		return "- automatic proposal", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFresh()
	input := rpc.MaintenanceInput{SessionID: "ses_revoked", Project: project, Compacted: true}
	published, err := invokeMaintenance(t, ctx, session, "recover-pending-ledger", input, fresh)
	if err != nil || !published.Published {
		t.Fatalf("pending recovery=%+v %v", published, err)
	}
	recovered, err := store.State(ctx, project)
	if err != nil || recovered.Watermark != pending[0].Sequence {
		t.Fatalf("recovery watermark=%+v %v", recovered, err)
	}
	page, err := callMemoryCapability[rpc.ListInput, rpc.Page](ctx, session, memoryContract(t), "memory", rpc.List, "", rpc.ListInput{Scope: memory.ScopeProject, CWD: project}, sdk.ParamsBase{})
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("recovered private facts=%+v %v", page, err)
	}
	foundHuman, foundProposal := false, false
	for _, item := range page.Rows {
		if item.ID == human.ID {
			foundHuman = item.Content == human.Content && item.Pinned && item.Origin == memory.OriginUser && item.Status == memory.StatusActive
		} else {
			foundProposal = item.Content == "automatic proposal" && item.Status == memory.StatusPending && item.Origin == memory.OriginAuto
		}
	}
	if !foundHuman || !foundProposal {
		t.Fatalf("curation replaced human authority: %+v", page.Rows)
	}
	repeated, err := callMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, memoryContract(t), "memory", rpc.Maintain, "recover-pending-ledger", input, sdk.ParamsBase{})
	if err != nil || repeated != published {
		t.Fatalf("settled recovery repeated publication: %+v %v", repeated, err)
	}
	final, err := store.State(ctx, project)
	if err != nil || final != recovered {
		t.Fatalf("settled recovery advanced generation: %+v %v", final, err)
	}
	broker.RevokeSession(session)
}

func TestMaintenancePublicationAndReceiptFailTogetherWithoutLosingLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	project := t.TempDir()
	store, session, _ := connectMemoryBackend(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fail_maintenance_receipt BEFORE UPDATE OF reply ON commands BEGIN SELECT RAISE(ABORT,'receipt failed'); END`); err != nil {
		t.Fatal(err)
	}
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	defer broker.RevokeSession(session)
	var calls atomic.Int32
	grant, release, err := openTestModel(t.Context(), broker, session, promptModel(func(context.Context, app.Prompt) (string, error) {
		calls.Add(1)
		return "- durable ledger fact", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	input := rpc.MaintenanceInput{SessionID: "ses_receipt_failure", Project: project, Compacted: true, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured"}}
	if _, err := invokeMaintenance(t, t.Context(), session, "original", input, grant); err == nil {
		t.Fatal("receipt failure returned successful publication")
	}
	state, err := store.State(t.Context(), project)
	if err != nil || state.Watermark != 0 {
		t.Fatalf("receipt failure advanced watermark: %+v, %v", state, err)
	}
	pending, err := store.PendingLedger(t.Context(), project, 0, memory.MaxLedgerFoldFacts)
	if err != nil || len(pending) != 1 {
		t.Fatalf("receipt failure discarded committed ledger: %+v, %v", pending, err)
	}
	page, err := callMemoryCapability[rpc.ListInput, rpc.Page](t.Context(), session, memoryContract(t), "memory", rpc.List, "", rpc.ListInput{Scope: memory.ScopeProject, CWD: project}, sdk.ParamsBase{})
	if err != nil || len(page.Rows) != 0 {
		t.Fatalf("publication survived failed receipt: %+v, %v", page, err)
	}
	if _, found, err := receiptMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, memoryContract(t), "memory", rpc.Maintain, "original", input); err != nil || found {
		t.Fatalf("failed publication became known: %t, %v", found, err)
	}
	if _, err := callMemoryCapability[rpc.MaintenanceInput, rpc.MaintenanceResult](t.Context(), session, memoryContract(t), "memory", rpc.Maintain, "original", input, sdk.ParamsBase{}); err == nil || calls.Load() != 2 {
		t.Fatalf("pending admission reran models: %d, %v", calls.Load(), err)
	}
}

func invokeMaintenance(t *testing.T, ctx context.Context, session *sdk.ClientSession, id string, input rpc.MaintenanceInput, ref pluginsdk.CallbackRef) (rpc.MaintenanceResult, error) {
	t.Helper()
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	exported, err := pluginsdk.NewCapabilityBinding(session, "memory", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := exported.Method(rpc.Maintain)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	reply, err := binding.InvokeWithMetadata(ctx, id, admitted, metadata)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	var result rpc.MaintenanceResult
	err = json.Unmarshal(reply.JSON(), &result, json.RejectUnknownMembers(true))
	return result, err
}

func TestSearchRequiresMetadataForConfiguredEmbeddingAndRejectsFormerGrantInput(t *testing.T) {
	_, session, _ := connectMemoryBackend(t, filepath.Join(t.TempDir(), "memory.db"))
	input := rpc.SearchInput{Project: t.TempDir(), Query: "fact", TopK: 5, EmbeddingSpace: "configured-space"}
	if _, err := callMemoryCapability[rpc.SearchInput, []memory.ItemSnapshot](t.Context(), session, memoryContract(t), "memory", rpc.Search, "", input, sdk.ParamsBase{}); err == nil {
		t.Fatal("configured embedding silently omitted required callback metadata")
	}
	input.EmbeddingSpace = ""
	if _, err := callMemoryCapability[rpc.SearchInput, []memory.ItemSnapshot](t.Context(), session, memoryContract(t), "memory", rpc.Search, "", input, sdk.ParamsBase{}); err != nil {
		t.Fatalf("absent embedding role requires a callback: %v", err)
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		descriptor extension.MethodDescriptor
		input      any
	}{{contracts.Search.Descriptor(), input}, {contracts.Maintain.Descriptor(), rpc.MaintenanceInput{SessionID: "ses_former", Project: input.Project, Compacted: true, Conversation: rpc.Conversation{MessageCount: 4, Transcript: "captured"}}}} {
		method, err := extension.AdmitMethod(call.descriptor)
		if err != nil {
			t.Fatal(err)
		}
		admitted, err := jsonvalue.Encode(call.input)
		if err != nil {
			t.Fatal(err)
		}
		if err := method.ValidateInput(admitted); err != nil {
			t.Fatalf("valid business input: %v", err)
		}
		var former map[string]any
		if err := json.Unmarshal(admitted.JSON(), &former); err != nil {
			t.Fatal(err)
		}
		former["Grant"] = pluginsdk.CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: strings.Repeat("a", 64), Call: strings.Repeat("b", 64), MaxResponseBytes: 1024}
		admitted, err = jsonvalue.Encode(former)
		if err != nil {
			t.Fatal(err)
		}
		if err := method.ValidateInput(admitted); err == nil {
			t.Fatalf("%s accepts former callback Grant as business input", call.descriptor.Name)
		}
	}
}

func memoryContract(t *testing.T) *extension.Capability {
	t.Helper()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}

// The callback fixture supplies the public model exchange. Runtime owns admission
// and provider budgets; these tests assert Memory's private publication lifetime.
func openTestModel(ctx context.Context, callbacks *pluginsdk.Callbacks, session *sdk.ClientSession, model chat.Model) (pluginsdk.CallbackRef, func(), error) {
	completion, err := pluginsdk.NewModelCallback(func(ctx context.Context, command pluginsdk.Command[chat.Request]) (*chat.Response, error) {
		return model.Call(ctx, command.Input.Clone())
	})
	if err != nil {
		return pluginsdk.CallbackRef{}, nil, err
	}
	policy := rpc.ModelCallerPolicy()
	return callbacks.Open(ctx, session, pluginsdk.CallbackLimits{MaxRequestBytes: policy.MaxCallbackInputBytes, MaxResponseBytes: policy.MaxCallbackOutputBytes}, completion)
}

func callMemoryCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *extension.Capability, export, method, id string, input P, metadata sdk.ParamsBase) (R, error) {
	var zero R
	exported, err := pluginsdk.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, err
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return zero, err
	}
	result, err := bound.InvokeWithMetadata(ctx, id, admitted, metadata)
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(result.JSON(), &zero, json.RejectUnknownMembers(true))
	return zero, err
}

func receiptMemoryCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *extension.Capability, export, method, id string, input P) (R, bool, error) {
	var zero R
	exported, err := pluginsdk.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, false, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, false, err
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return zero, false, err
	}
	result, found, err := bound.ReadReceipt(ctx, id, admitted)
	if err != nil || !found {
		return zero, found, err
	}
	err = json.Unmarshal(result.JSON(), &zero, json.RejectUnknownMembers(true))
	return zero, true, err
}
