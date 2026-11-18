package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"

	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func textResponseModel(call func(context.Context, *chat.Request) (string, error)) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		text, err := call(ctx, request)
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
	broker := pluginmcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	grant, release, err := openTestModel(t.Context(), broker, session, textResponseModel(func(_ context.Context, request *chat.Request) (string, error) {
		if len(request.Messages) != 2 {
			return "", errors.New("expected system and user messages")
		}
		switch calls.Add(1) {
		case 1:
			if request.Messages[1].Text() != "captured conversation" {
				return "", errors.New("extraction did not receive the captured conversation")
			}
		case 2:
			if !strings.Contains(request.Messages[1].Text(), "use local workspace mode") {
				return "", errors.New("curation did not receive the extracted facts")
			}
		default:
			return "", errors.New("unexpected model call")
		}
		return `["use local workspace mode", "verify backend restart"]`, nil
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
	if calls.Load() != 2 {
		t.Fatalf("model calls=%d", calls.Load())
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
	broker := pluginmcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	curating := make(chan struct{})
	joined := make(chan struct{})
	var calls atomic.Int32
	grant, release, err := openTestModel(ctx, broker, session, textResponseModel(func(ctx context.Context, _ *chat.Request) (string, error) {
		switch calls.Add(1) {
		case 1:
			return `["automatic proposal"]`, nil
		case 2:
		default:
			return "", errors.New("unexpected model call")
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
	human, err := callMemoryCapability[rpc.UpdateInput, memory.ItemSnapshot](ctx, session, memoryContract(t), "memory", rpc.Update, "human-edit", rpc.UpdateInput{ID: added.Item.ID.String(), ExpectedRevision: added.Item.Revision, Content: &content, Pinned: &pinned}, sdk.ParamsBase{})
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
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
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
	var recoveryCalls atomic.Int32
	fresh, releaseFresh, err := openTestModel(ctx, broker, session, textResponseModel(func(_ context.Context, request *chat.Request) (string, error) {
		if recoveryCalls.Add(1) != 1 || len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Text(), "automatic proposal") {
			return "", errors.New("pending recovery did not curate the original ledger")
		}
		return `["automatic proposal"]`, nil
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
	if recoveryCalls.Load() != 1 {
		t.Fatalf("pending recovery model calls=%d", recoveryCalls.Load())
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

func invokeMaintenance(t *testing.T, ctx context.Context, session *sdk.ClientSession, id string, input rpc.MaintenanceInput, ref pluginmcp.CallbackRef) (rpc.MaintenanceResult, error) {
	t.Helper()
	metadata, err := pluginmcp.CallbackMetadata(ref)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	exported, err := pluginmcp.NewCapabilityBinding(session, "memory", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := exported.Method(rpc.Maintain)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	reply, err := binding.InvokeWithMetadata(ctx, pluginmcp.CommandID(id), admitted, metadata)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	var result rpc.MaintenanceResult
	err = json.Unmarshal(reply.Bytes(), &result, json.RejectUnknownMembers(true))
	return result, err
}

func memoryContract(t *testing.T) *plugincontract.Capability {
	t.Helper()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}

// The callback fixture supplies the public model exchange. Runtime owns admission
// and provider budgets; these tests assert Memory's private publication lifetime.
func openTestModel(ctx context.Context, callbacks *pluginmcp.Callbacks, session *sdk.ClientSession, model chat.Model) (pluginmcp.CallbackRef, func(), error) {
	completion, err := pluginsdk.NewModelCallback(func(ctx context.Context, command pluginmcp.Command[scopewire.Request]) (*chat.Response, error) {
		return model.Call(ctx, (*chat.Request)(&command.Input).Clone())
	})
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	policy := rpc.ModelCallerPolicy()
	return callbacks.Open(ctx, session, pluginmcp.CallbackLimits{MaxRequestBytes: policy.MaxCallbackInputBytes, MaxResponseBytes: policy.MaxCallbackOutputBytes}, completion)
}

func callMemoryCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *plugincontract.Capability, export, method, id string, input P, metadata sdk.ParamsBase) (R, error) {
	var zero R
	exported, err := pluginmcp.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, err
	}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return zero, err
	}
	result, err := bound.InvokeWithMetadata(ctx, pluginmcp.CommandID(id), admitted, metadata)
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(result.Bytes(), &zero, json.RejectUnknownMembers(true))
	return zero, err
}

func receiptMemoryCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *plugincontract.Capability, export, method, id string, input P) (R, bool, error) {
	var zero R
	exported, err := pluginmcp.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, false, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, false, err
	}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return zero, false, err
	}
	result, found, err := bound.ReadReceipt(ctx, pluginmcp.CommandID(id), admitted)
	if err != nil || !found {
		return zero, found, err
	}
	err = json.Unmarshal(result.Bytes(), &zero, json.RejectUnknownMembers(true))
	return zero, true, err
}
