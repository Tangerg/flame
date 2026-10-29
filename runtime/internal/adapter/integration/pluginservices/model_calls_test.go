package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

type routedCallResolver struct {
	model      chat.Model
	mu         sync.Mutex
	selections []modelchoice.Selection
}

func (r *routedCallResolver) ResolveChat(_ context.Context, selection modelchoice.Selection) (models.ResolvedChat, error) {
	r.mu.Lock()
	r.selections = append(r.selections, selection)
	r.mu.Unlock()
	return models.NewResolvedChat(r.model, nil)
}
func selectedCallInput(model string) pluginsdk.ModelCallRequest {
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	request.Options.Model = model
	request.Options.MaxOutputTokens = new(int64(64))
	request.Tools = []chat.ToolDefinition{{Name: "echo", Description: "Echo.", InputSchema: jsontext.Value(`{"type":"object"}`)}}
	return pluginsdk.ModelCallRequest{Selection: pluginsdk.ModelSelection{Provider: "anthropic", Model: model}, Budget: pluginsdk.ModelCallBudget{MaxRequestBytes: 4096, MaxOutputTokens: 64}, Request: *request}
}
func selectedCallRef(t *testing.T, composition *CallComposition, resolver models.ChatModelResolver) (qimumcp.CallbackRef, func()) {
	t.Helper()
	callbacks, err := composition.ModelCallbacks(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.callbacks.PublishRPC(http.NotFoundHandler(), callbacks...); err != nil {
		t.Fatal(err)
	}
	session := new(sdk.ClientSession)
	if err := composition.callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "original-plugin-operation", Origin: extension.Model, Parent: "original-effect", Target: portable.Target{Scope: extension.ScopeCall, Reference: "original-plugin-operation"}})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := composition.callbacks.OpenRPC(ctx, session, qimumcp.CallbackLimits{MaxRequestBytes: 64 << 10, MaxResponseBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return ref, release
}
func TestSelectedModelCallsHaveIndependentRoutesBudgetsAndDurableEvidence(t *testing.T) {
	for _, outerFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(outerFailure), func(t *testing.T) {
			mode := "transparent"
			if outerFailure {
				mode = "outer-failure"
			}
			composition, _, _ := installedObserverFixture(t, mode)
			path := filepath.Join(t.TempDir(), "model-calls.db")
			db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			var physical atomic.Int32
			endpoint := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				physical.Add(1)
				inv, err := qimumcp.InvocationFromContext(ctx)
				if err != nil || inv.Parent != "original-plugin-operation" || inv.Origin != extension.Model || inv.ID != inv.Target.Reference || len(request.Tools) != 1 {
					return nil, errors.New("independent model lost original caller or complete request")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("known " + request.Options.Model))
				return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-" + request.Options.Model, Usage: &chat.Usage{InputTokens: 11, OutputTokens: 3}}}, nil
			})
			routing := &routedCallResolver{model: endpoint}
			observed, err := models.NewObservedResolver(routing, db.ModelInvocations, func(_ string, _ string, _ *chat.Usage) (accounting.Cost, error) {
				return accounting.NewCost(1.5)
			})
			if err != nil {
				t.Fatal(err)
			}
			ref, release := selectedCallRef(t, composition, observed)
			input := selectedCallInput("route-a")
			original := input.Request.Clone()
			for i := range 3 {
				input.Selection.Model = fmt.Sprintf("route-%d", i)
				input.Request.Options.Model = input.Selection.Model
				response, err := pluginsdk.CallSelectedModel(t.Context(), ref, fmt.Sprintf("independent-%d", i), input)
				if response == nil || response.Text() != "known "+input.Selection.Model || (err != nil) != outerFailure {
					t.Fatalf("independent outcome %d: %+v %v", i, response, err)
				}
			}
			input.Request.Options.Model = original.Options.Model
			if !reflect.DeepEqual(&input.Request, original) {
				t.Fatal("independent route mutated caller request")
			}
			input.Selection.Model = "route-0"
			input.Request.Options.Model = "route-0"
			if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "independent-0", input); err == nil || physical.Load() != 3 {
				t.Fatalf("redelivered model executed: %d %v", physical.Load(), err)
			}
			input.Request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
			if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "independent-0", input); err == nil || physical.Load() != 3 {
				t.Fatalf("changed identity executed: %d %v", physical.Load(), err)
			}
			if len(composition.HeldDependencies()) != 0 {
				t.Fatal("finished model calls retained contributions")
			}
			release()
			if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "after-release", input); !errors.Is(err, qimumcp.ErrStaleCall) {
				t.Fatalf("released operation remained callable: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = persistencefixture.Open(t, t.Context(), filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := db.ModelInvocations
			rows, err := store.PageEndpointInvocations(t.Context(), "original-plugin-operation", 0, "", 10)
			if err != nil || len(rows) != 3 {
				t.Fatalf("durable independent endpoints: %+v %v", rows, err)
			}
			for _, row := range rows {
				if row.Invocation.Caller.Parent != "original-plugin-operation" || row.Invocation.Caller.Origin != extension.Model || row.Invocation.Model == "" || row.Response.Metadata.Model != "served-"+row.Invocation.Model || row.Response.Metadata.Usage.InputTokens != 11 || row.CostUSD == nil || *row.CostUSD != 1.5 {
					t.Fatalf("independent evidence replaced: %+v", row)
				}
			}
		})
	}
}

func TestSelectedModelFinalRequestBudgetRefusesBeforeProviderDispatch(t *testing.T) {
	for _, mode := range []string{"success", "media", "initial-output", "initial-bytes", "final-bytes"} {
		t.Run(mode, func(t *testing.T) {
			contract, err := pluginsdk.ModelInputContract()
			if err != nil {
				t.Fatal(err)
			}
			composition, _ := installedCallCompositionFixture(t, contract, []string{"converter"}, func(backend *qimumcp.Backend) {
				method, _ := pluginsdk.ModelInputMethod()
				err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[pluginsdk.ModelInputRequest]) (chat.Request, error) {
					request := command.Input.Request.Clone()
					request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("converted"))
					if mode == "final-bytes" {
						request.Messages[0] = chat.NewUserMessage(chat.NewTextPart(string(make([]byte, 4096))))
					}
					return *request, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
			var calls atomic.Int32
			routing := &routedCallResolver{model: chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls.Add(1)
				if request.Messages[0].Text() != "converted" || len(request.Tools) != 1 {
					t.Error("provider did not receive complete transformed request")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("known"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
			})}
			ref, _ := selectedCallRef(t, composition, routing)
			input := selectedCallInput("configured-model")
			if mode == "media" {
				image, err := media.NewBytes("image/png", []byte("source image"))
				if err != nil {
					t.Fatal(err)
				}
				input.Request.Messages[0] = chat.NewUserMessage(chat.NewMediaPart(image))
				input.Selection = pluginsdk.ModelSelection{Provider: "alibaba", Model: "qwen-mt-plus"}
				input.Request.Options.Model = input.Selection.Model
			}
			if mode == "initial-output" {
				input.Budget.MaxOutputTokens = 63
			}
			if mode == "initial-bytes" {
				input.Budget.MaxRequestBytes = 1
			}
			response, err := pluginsdk.CallSelectedModel(t.Context(), ref, "final-budget", input)
			if mode == "success" || mode == "media" {
				if err != nil || response == nil || calls.Load() != 1 {
					t.Fatalf("admitted request: %+v %d %v", response, calls.Load(), err)
				}
			} else if err == nil || calls.Load() != 0 {
				t.Fatalf("budget reached provider: %d %v", calls.Load(), err)
			}
			if len(composition.HeldDependencies()) != 0 {
				t.Fatal("refused model retained dependency")
			}
		})
	}
}

type selectedStreamModel struct {
	calls  atomic.Int32
	joined chan struct{}
}

func (m *selectedStreamModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unary path used for stream")
}
func (m *selectedStreamModel) Stream(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		m.calls.Add(1)
		defer close(m.joined)
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("first ")}, Metadata: &chat.ResponseMetadata{Model: "served-stream", Usage: &chat.Usage{InputTokens: 11, OutputTokens: 1}}}, nil) {
			return
		}
		if ctx.Err() != nil {
			yield(nil, ctx.Err())
			return
		}
		yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("second")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 11, OutputTokens: 2}}}, nil)
	}
}
func TestSelectedModelStreamIsLazyPreservesEvidenceAndJoinsOnRelease(t *testing.T) {
	for _, stopEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(stopEarly), func(t *testing.T) {
			composition, _, _ := installedObserverFixture(t, "transparent")
			db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "stream.db")))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			endpoint := &selectedStreamModel{joined: make(chan struct{})}
			observed, err := models.NewObservedResolver(&routedCallResolver{model: endpoint}, db.ModelInvocations, nil)
			if err != nil {
				t.Fatal(err)
			}
			ref, release := selectedCallRef(t, composition, observed)
			input := selectedCallInput("stream-model")
			sequence := pluginsdk.StreamSelectedModel(t.Context(), ref, "independent-stream", input)
			input.Request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
			if endpoint.calls.Load() != 0 {
				t.Fatal("unconsumed stream started provider")
			}
			var result *pluginsdk.ModelResult
			for event, err := range sequence {
				if err != nil {
					t.Fatal(err)
				}
				if stopEarly {
					break
				}
				if event.Result != nil {
					result = event.Result
				}
			}
			release()
			<-endpoint.joined
			if endpoint.calls.Load() != 1 || len(composition.HeldDependencies()) != 0 {
				t.Fatal("stream source or dependency leaked")
			}
			if !stopEarly && (result == nil || result.Failure != nil || result.Response.Text() != "first second" || result.Response.Metadata.Usage.OutputTokens != 2) {
				t.Fatalf("canonical final outcome: %+v", result)
			}
			row, found, err := db.ModelInvocations.EndpointForCaller(t.Context(), "independent-stream")
			if err != nil || !found || row.Invocation.Caller.Parent != "original-plugin-operation" || row.Invocation.Caller.Origin != extension.Model || row.Invocation.Request.Messages[0].Text() != "original input" {
				t.Fatalf("stream original evidence: %+v %v", row, err)
			}
		})
	}
}

func TestSelectedModelResolvesInstalledSelectionForEveryIndependentCall(t *testing.T) {
	contract, err := pluginsdk.ModelSelectionContract()
	if err != nil {
		t.Fatal(err)
	}
	var choices atomic.Int32
	composition, _ := installedCallCompositionFixture(t, contract, []string{"selector"}, func(backend *qimumcp.Backend) {
		method, _ := pluginsdk.ModelSelectionMethod()
		err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[pluginsdk.ModelSelectionRequest]) (pluginsdk.ModelSelection, error) {
			parent, err := qimumcp.InvocationFromContext(ctx)
			if err != nil || parent.Parent != fmt.Sprintf("selected-route-%d", choices.Load()) || parent.Origin != extension.Model || command.Input.Request.Options.Model != "proposed-model" {
				return pluginsdk.ModelSelection{}, errors.New("selector lost original independent request")
			}
			if command.Input.Context.Use != modelchoice.Independent || command.Input.Context.Limits == nil || *command.Input.Context.Limits != (modelchoice.SelectionLimits{InputKind: modelchoice.RequestBytes, MaxInputBytes: selectedCallInput("proposed-model").Budget.MaxRequestBytes, MaxOutputTokens: selectedCallInput("proposed-model").Budget.MaxOutputTokens}) {
				return pluginsdk.ModelSelection{}, errors.New("selector lost independent caller envelope")
			}
			chosen := choices.Add(1)
			return pluginsdk.ModelSelection{Provider: "anthropic", Model: fmt.Sprintf("chosen-%d", chosen)}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	routing := &routedCallResolver{model: chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		message := chat.NewAssistantMessage(chat.NewTextPart(request.Options.Model))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})}
	ref, _ := selectedCallRef(t, composition, routing)
	input := selectedCallInput("proposed-model")
	for i := range 3 {
		response, err := pluginsdk.CallSelectedModel(t.Context(), ref, fmt.Sprintf("selected-route-%d", i), input)
		if err != nil || response == nil || response.Text() != fmt.Sprintf("chosen-%d", i+1) {
			t.Fatalf("fresh selection %d: %+v %v", i, response, err)
		}
	}
	if choices.Load() != 3 || input.Request.Options.Model != "proposed-model" {
		t.Fatal("independent selection reused capture or mutated caller")
	}
}

func TestIndependentModelCallbackRetainsExecutingOwnerBeforeProviderDispatch(t *testing.T) {
	composition, _, _ := installedObserverFixture(t, "transparent")
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "executing-owner.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sessionID := "ses_executing_owner"
	if err := db.Sessions.Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{ID: sessionID})); err != nil {
		t.Fatal(err)
	}
	draft := testsupport.RunDraft(run.Draft{RunID: "run_executing_owner", SessionID: sessionID, SegmentID: "seg_executing_owner", CreatedAt: time.Unix(100, 0).UTC()})
	runStore := db.Runs
	if err := runStore.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	journal := db.ModelInvocations
	if err := journal.StartModelInvocation(t.Context(), sessionID, draft.RunID, draft.SegmentID, "logical_executing_owner", draft.CreatedAt, run.ModelConversation); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	observed, err := models.NewObservedResolver(&routedCallResolver{model: chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		inv, _ := qimumcp.InvocationFromContext(ctx)
		row, found, err := journal.EndpointForCaller(ctx, inv.ID)
		if err != nil || !found || row.RunID != draft.RunID || row.SegmentID != draft.SegmentID {
			return nil, errors.New("provider started without its actual execution association")
		}
		calls.Add(1)
		message := chat.NewAssistantMessage(chat.NewTextPart("known"))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})}, journal, nil)
	if err != nil {
		t.Fatal(err)
	}
	callbacks, err := composition.ModelCallbacks(observed)
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.callbacks.PublishRPC(http.NotFoundHandler(), callbacks...); err != nil {
		t.Fatal(err)
	}
	sdkSession := new(sdk.ClientSession)
	if err := composition.callbacks.AdmitSession(sdkSession); err != nil {
		t.Fatal(err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: sessionID})
	ctx = executionctx.WithInvocationOwner(ctx, "logical_executing_owner")
	ctx = executionctx.WithModelCalls(ctx, sourceAdmissionPublisher{journal: journal, draft: draft})
	ctx, err = qimumcp.WithInvocation(ctx, portable.Invocation{ID: "original-plugin-operation", Origin: extension.Model, Parent: "logical_executing_owner", Target: portable.Target{Scope: extension.ScopeCall, Reference: "original-plugin-operation"}})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := composition.callbacks.OpenRPC(ctx, sdkSession, qimumcp.CallbackLimits{MaxRequestBytes: 64 << 10, MaxResponseBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	input := selectedCallInput("independent-route")
	forged := executionctx.WithInvocationOwner(t.Context(), "foreign-wire-owner")
	forged = executionctx.WithScope(forged, runs.ExecutionScope{SessionID: "ses_foreign"})
	forged, err = qimumcp.WithInvocation(forged, portable.Invocation{ID: "foreign-wire-owner", Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeCall, Reference: "foreign-wire-owner"}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "logical_executing_owner", input); err == nil || calls.Load() != 0 {
		t.Fatal("independent request claimed primary invocation")
	}
	if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "original-plugin-operation", input); err == nil || calls.Load() != 0 {
		t.Fatal("independent request relabeled original operation")
	}
	response, err := pluginsdk.CallSelectedModel(forged, ref, "independent-owned", input)
	if err != nil || response == nil || calls.Load() != 1 {
		t.Fatalf("captured executing owner: %+v %d %v", response, calls.Load(), err)
	}
	resolved, err := observed.ResolveChat(ctx, testsupport.MustModelSelection("anthropic", "auxiliary-route"))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := models.NewSelectedChat(testsupport.MustModelSelection("anthropic", "auxiliary-route"), resolved)
	if err != nil {
		t.Fatal(err)
	}
	auxiliary, closeAux, err := composition.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		return selected, func() {}, nil
	})(ctx, models.AuxiliaryBudget{MaxInputBytes: 4096, MaxOutputTokens: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer closeAux()
	if _, err := auxiliary.Model().Call(executionctx.WithInvocationOwner(ctx, "original-plugin-operation"), &input.Request); err == nil {
		t.Fatal("auxiliary capture claimed its executing owner identity")
	}
	fresh, err := qimumcp.WithInvocation(ctx, portable.Invocation{ID: "auxiliary-owned", Origin: extension.Model, Parent: "original-plugin-operation", Target: portable.Target{Scope: extension.ScopeCall, Reference: "auxiliary-owned"}})
	if err != nil {
		t.Fatal(err)
	}
	request := input.Request.Clone()
	request.Tools = nil
	request.Options.Model = "auxiliary-route"
	if _, err := auxiliary.Model().Call(fresh, request); err != nil {
		t.Fatal(err)
	}
	auxSource, found, err := journal.EndpointForCaller(t.Context(), "auxiliary-owned")
	if err != nil || !found || auxSource.RunID != draft.RunID || auxSource.Invocation.Caller.Parent != "original-plugin-operation" || calls.Load() != 2 {
		t.Fatalf("auxiliary ownership = %+v %v", auxSource, err)
	}
	closeAux()
	row, found, err := journal.EndpointForCaller(t.Context(), "independent-owned")
	if err != nil || !found || row.Invocation.Caller.Parent != "original-plugin-operation" || row.Invocation.Caller.Origin != extension.Model {
		t.Fatalf("wire replaced captured origin: %+v %v", row, err)
	}
	active, _, err := runStore.Run(t.Context(), draft.RunID)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := active.Suspend(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := runStore.Suspend(t.Context(), waiting, draft.SegmentID, runs.CommitID{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "independent-after-wait", input); err == nil || calls.Load() != 2 {
		t.Fatalf("retired execution dispatched provider: %d %v", calls.Load(), err)
	}
	if len(composition.HeldDependencies()) != 0 {
		t.Fatal("refused owner retained installed model dependency")
	}
}

// This fixture verifies physical admission; the real Run pump is covered separately.
type sourceAdmissionPublisher struct {
	journal *persistence.ModelInvocationStore
	draft   run.Draft
}

func (p sourceAdmissionPublisher) Start(ctx context.Context, id string, selection modelchoice.Selection) (func(context.Context, error) error, error) {
	if err := p.journal.StartModelInvocation(ctx, p.draft.SessionID, p.draft.RunID, p.draft.SegmentID, id, time.Now().UTC(), run.ModelIndependent); err != nil {
		return nil, err
	}
	return func(context.Context, error) error { return nil }, nil
}
