package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
)

func connectProvider(t *testing.T, path string) (*store, *sdk.ClientSession, func()) {
	t.Helper()
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(store)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "provider-test", Version: "1"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	connection, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	close := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), connection.Close(), store.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(close)
	return store, session, close
}

func providerContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "provider-test", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "providers"}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func providerMethod[P, R any](t *testing.T, session *sdk.ClientSession, method *qimumcp.Method[P, R]) *qimumcp.BoundMethod[P, R] {
	t.Helper()
	contract, err := modelprovider.Contract()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := qimumcp.NewCapabilityBinding(session, "model-provider", contract)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := method.Bind(capability)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func openChat(t *testing.T, session *sdk.ClientSession, provider, endpoint string) modelprovider.Chat {
	t.Helper()
	method, err := modelprovider.OpenChatMethod()
	if err != nil {
		t.Fatal(err)
	}
	value, err := providerMethod(t, session, method).CallCommand(providerContext(t), "open", modelprovider.Open{Connection: modelprovider.Connection{Provider: provider, APIKey: "captured-key", BaseURL: endpoint}, Model: "model-test"})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func providerRequest(handle modelprovider.Handle) modelprovider.ChatRequest {
	return modelprovider.ChatRequest{Handle: handle, Request: chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("question"))}, Options: chat.Options{MaxOutputTokens: new(int64(32))}}}
}

func writeChatReply(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprint(w, "data: "+`{"id":"reply","model":"served-model","choices":[{"index":0,"delta":{"role":"assistant","content":"ans"}}]}`+"\n\n")
	_, _ = fmt.Fprint(w, "data: "+`{"id":"reply","model":"served-model","choices":[{"index":0,"delta":{"content":"wer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`+"\n\ndata: [DONE]\n\n")
}

func TestProviderProfilesPublishTheInstalledCatalogAndActualCapabilities(t *testing.T) {
	_, session, _ := connectProvider(t, filepath.Join(t.TempDir(), "providers.db"))
	method, err := modelprovider.ProfilesMethod()
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := providerMethod(t, session, method).Call(providerContext(t), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, profile := range profiles {
		if err := profile.Validate(); err != nil {
			t.Fatal(err)
		}
		if profile.ID == "openai-compatible" {
			found = profile.EndpointRequired && profile.ModelSource == modelprovider.EndpointModels && profile.DefaultModel == ""
		}
	}
	if !found {
		t.Fatal("configured endpoint policy was lost")
	}
	opened := openChat(t, session, "anthropic", "https://provider.invalid")
	if !opened.Streaming || !opened.TokenCounting {
		t.Fatal("native provider capabilities were lost")
	}
}

func TestOriginalChatOutcomesAndCredentialSnapshotSurviveRestart(t *testing.T) {
	for _, outcome := range []string{"completed", "reply-lost"} {
		t.Run(outcome, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer captured-key" {
					t.Error("captured credential changed")
				}
				if outcome == "reply-lost" {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
				w.Header().Set("Content-Type", "application/json")
				writeChatReply(w)
			}))
			defer upstream.Close()
			path := filepath.Join(t.TempDir(), "providers.db")
			_, session, stop := connectProvider(t, path)
			opened := openChat(t, session, "openai-compatible", upstream.URL)
			if !opened.Streaming || opened.TokenCounting {
				t.Fatal("compatible provider invented or lost an optional capability")
			}
			stop()
			_, session, stop = connectProvider(t, path)
			method, err := modelprovider.CallMethod()
			if err != nil {
				t.Fatal(err)
			}
			request := providerRequest(opened.Handle)
			result, err := providerMethod(t, session, method).CallCommand(providerContext(t), "exchange", request)
			initialCalls := calls.Load()
			if err != nil || result.Validate() != nil || initialCalls == 0 || (outcome == "completed" && initialCalls != 1) {
				t.Fatalf("provider exchange = %+v %v, calls=%d", result, err, calls.Load())
			}
			if outcome == "completed" {
				if result.Failure != nil || result.Response.Metadata.Usage.InputTokens != 7 || result.Response.Metadata.Model != "served-model" {
					t.Fatalf("provider evidence lost: %+v", result)
				}
			} else if result.Failure == nil || result.Failure.Execution != chat.CallFailureUnknown {
				t.Fatalf("lost exchange became confirmed: %+v", result)
			}
			stop()
			_, session, _ = connectProvider(t, path)
			for range 2 {
				repeated, err := providerMethod(t, session, method).CallCommand(providerContext(t), "exchange", request)
				if err != nil || calls.Load() != initialCalls || (repeated.Failure == nil) != (result.Failure == nil) {
					t.Fatalf("receipt repeated provider work: %+v %v calls=%d", repeated, err, calls.Load())
				}
			}
			request.Request.Messages = []chat.Message{chat.NewUserMessage(chat.NewTextPart("different question"))}
			if _, err := providerMethod(t, session, method).CallCommand(providerContext(t), "exchange", request); err == nil || calls.Load() != initialCalls {
				t.Fatal("changed original request was dispatched")
			}
		})
	}
}

func TestPendingExchangeNeverDispatchesAndClosedHandleCannotDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeChatReply(w) }))
	defer upstream.Close()
	store, session, _ := connectProvider(t, filepath.Join(t.TempDir(), "providers.db"))
	opened := openChat(t, session, "openai-compatible", upstream.URL)
	method, _ := modelprovider.CallMethod()
	request := providerRequest(opened.Handle)
	input, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pluginstorage.AdmitCommand(t.Context(), store.db, method.Name(), "pending", input, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := providerMethod(t, session, method).CallCommand(providerContext(t), "pending", request); err == nil || calls.Load() != 0 {
		t.Fatal("pending exchange was dispatched")
	}
	close, _ := modelprovider.CloseMethod()
	if _, err := providerMethod(t, session, close).CallCommand(providerContext(t), "close", opened.Handle); err != nil {
		t.Fatal(err)
	}
	result, err := providerMethod(t, session, method).CallCommand(providerContext(t), "after-close", request)
	if err != nil || result.Failure == nil || result.Failure.Execution != chat.CallFailureNotExecuted || calls.Load() != 0 {
		t.Fatalf("closed handle dispatched: %+v %v", result, err)
	}
}

func TestReopeningUsesTheOriginalPrivateConstructionAfterCloseAndRestart(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer captured-key" {
			t.Error("reopened model used another credential snapshot")
		}
		writeChatReply(w)
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "providers.db")
	_, session, stop := connectProvider(t, path)
	original := openChat(t, session, "openai-compatible", upstream.URL)
	closeMethod, _ := modelprovider.CloseMethod()
	if _, err := providerMethod(t, session, closeMethod).CallCommand(providerContext(t), "close", original.Handle); err != nil {
		t.Fatal(err)
	}
	stop()
	_, session, _ = connectProvider(t, path)
	reopen, _ := modelprovider.ReopenChatMethod()
	restored, err := providerMethod(t, session, reopen).CallCommand(providerContext(t), "reopen", modelprovider.Reopen{Original: original.Original})
	if err != nil || restored.Validate() != nil || restored.Original != original.Original || restored.ID == original.ID || restored.Streaming != original.Streaming || restored.TokenCounting != original.TokenCounting || calls.Load() != 0 {
		t.Fatalf("original construction was not restored: %+v %v, calls=%d", restored, err, calls.Load())
	}
	call, _ := modelprovider.CallMethod()
	result, err := providerMethod(t, session, call).CallCommand(providerContext(t), "resumed-exchange", providerRequest(restored.Handle))
	if err != nil || result.Failure != nil || result.Response == nil || calls.Load() != 1 {
		t.Fatalf("reopened client failed: %+v %v", result, err)
	}
}

func TestCountAndEmbeddingPreserveNativeRequestsAndCompleteResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/count_tokens") {
			_, _ = fmt.Fprint(w, `{"input_tokens":61}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/embeddings") {
			var body struct {
				Input []string `json:"input"`
				Model string   `json:"model"`
			}
			if err := json.UnmarshalRead(r.Body, &body); err != nil || body.Model != "embed-test" || len(body.Input) != 2 {
				t.Errorf("embedding input lost: %+v %v", body, err)
			}
			_, _ = fmt.Fprint(w, `{"object":"list","model":"served-embed","data":[{"object":"embedding","index":0,"embedding":[0.25,0.75]},{"object":"embedding","index":1,"embedding":[0.5,0.5]}],"usage":{"prompt_tokens":13,"total_tokens":13}}`)
			return
		}
		t.Errorf("unexpected request: %s", r.URL.Path)
	}))
	defer upstream.Close()
	_, session, _ := connectProvider(t, filepath.Join(t.TempDir(), "providers.db"))
	opened := openChat(t, session, "anthropic", upstream.URL)
	count, _ := modelprovider.CountMethod()
	value, err := providerMethod(t, session, count).CallCommand(providerContext(t), "count", providerRequest(opened.Handle))
	if err != nil || value.Validate() != nil || value.Count == nil || *value.Count != 61 {
		t.Fatalf("native count = %+v %v", value, err)
	}
	open, _ := modelprovider.OpenEmbeddingMethod()
	embedder, err := providerMethod(t, session, open).CallCommand(providerContext(t), "open-embedding", modelprovider.Open{Connection: modelprovider.Connection{Provider: "openai", APIKey: "captured-key", BaseURL: upstream.URL}, Model: "embed-test"})
	if err != nil || embedder.Space != embeddingSpaceID("openai", "embed-test", upstream.URL) {
		t.Fatalf("embedding space = %+v %v", embedder, err)
	}
	request := embedding.Request{Texts: []string{"first", "second"}, Options: embedding.Options{Model: "embed-test"}}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	embed, _ := modelprovider.EmbedMethod()
	result, err := providerMethod(t, session, embed).CallCommand(providerContext(t), "embed", modelprovider.EmbeddingRequest{Handle: embedder.Handle, Request: encoded})
	if err != nil || result.Failure != nil || result.Validate() != nil {
		t.Fatalf("embedding result = %+v %v", result, err)
	}
	response, err := result.Native()
	if err != nil || response.Metadata.Model != "served-embed" || response.Metadata.Usage.InputTokens != 13 || len(response.Outputs) != 2 || response.Outputs[0].Embedding[0] != 0.25 {
		t.Fatalf("complete embedding response lost: %+v %v", response, err)
	}
}

func TestStreamsRetainOriginalDeltasAndSettleWithoutReplayAfterConsumptionStops(t *testing.T) {
	for _, stopEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(stopEarly), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeChatReply(w)
			}))
			defer upstream.Close()
			_, session, _ := connectProvider(t, filepath.Join(t.TempDir(), "providers.db"))
			opened := openChat(t, session, "openai-compatible", upstream.URL)
			method, err := modelprovider.StreamMethod()
			if err != nil {
				t.Fatal(err)
			}
			contract, _ := modelprovider.Contract()
			exported, err := qimumcp.NewCapabilityBinding(session, "model-provider", contract)
			if err != nil {
				t.Fatal(err)
			}
			binding, err := exported.Method(method.Descriptor().Name)
			if err != nil {
				t.Fatal(err)
			}
			callbacks := qimumcp.NewCallbacks()
			defer func() { _ = callbacks.Close() }()
			if err := callbacks.AdmitSession(session); err != nil {
				t.Fatal(err)
			}
			request := providerRequest(opened.Handle)
			limits := qimumcp.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}
			chunks, completed := 0, false
			for value, err := range method.StreamWithMetadata(providerContext(t), binding, callbacks, limits, "original-stream", request, sdk.ParamsBase{}) {
				if err != nil {
					t.Fatal(err)
				}
				if value.Chunk != nil {
					chunks++
					if stopEarly {
						break
					}
				} else {
					completed = value.Result != nil && value.Result.Failure == nil && value.Result.Response.Metadata.Usage.InputTokens == 7
				}
			}
			if chunks == 0 || (!stopEarly && !completed) || calls.Load() != 1 {
				t.Fatalf("stream lost its original outcome: chunks=%d completed=%v calls=%d", chunks, completed, calls.Load())
			}
			input, _ := jsonvalue.Encode(request)
			receipt, found, err := binding.ReadReceipt(providerContext(t), "original-stream", input)
			if err != nil || !found {
				t.Fatalf("stream receipt missing: %v %v", found, err)
			}
			var original modelprovider.ChatResult
			if err := json.Unmarshal(receipt.JSON(), &original); err != nil {
				t.Fatal(err)
			}
			if stopEarly && (original.Failure == nil || original.Failure.Execution != chat.CallFailureUnknown) {
				t.Fatalf("stopped stream manufactured completion: %+v", original)
			}
			for value, err := range method.StreamWithMetadata(providerContext(t), binding, callbacks, limits, "original-stream", request, sdk.ParamsBase{}) {
				if err != nil || value.Chunk != nil || value.Result == nil || (value.Result.Failure != nil) != stopEarly {
					t.Fatalf("stream receipt replayed or changed: %+v %v", value, err)
				}
			}
			if calls.Load() != 1 {
				t.Fatal("stream recovery dispatched another exchange")
			}
		})
	}
}

func TestCloseJoinsAnAcceptedExchangeAndFencesNewDispatch(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		writeChatReply(w)
	}))
	defer upstream.Close()
	releaseProvider := sync.OnceFunc(func() { close(release) })
	defer releaseProvider()
	store, session, _ := connectProvider(t, filepath.Join(t.TempDir(), "providers.db"))
	opened := openChat(t, session, "openai-compatible", upstream.URL)
	call, _ := modelprovider.CallMethod()
	ctx := providerContext(t)
	called := make(chan error, 1)
	go func() {
		_, err := providerMethod(t, session, call).CallCommand(ctx, "accepted", providerRequest(opened.Handle))
		called <- err
	}()
	<-entered
	closeMethod, _ := modelprovider.CloseMethod()
	closed := make(chan error, 1)
	go func() {
		_, err := providerMethod(t, session, closeMethod).CallCommand(ctx, "close", opened.Handle)
		closed <- err
	}()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		var snapshot sql.NullString
		if err := store.db.QueryRowContext(t.Context(), `SELECT snapshot FROM clients WHERE id=?`, opened.ID).Scan(&snapshot); err != nil {
			t.Fatal(err)
		}
		if !snapshot.Valid {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("close did not fence new work")
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case err := <-closed:
		t.Fatalf("close returned before accepted work joined: %v", err)
	default:
	}
	refused, err := providerMethod(t, session, call).CallCommand(providerContext(t), "refused", providerRequest(opened.Handle))
	if err != nil || refused.Failure == nil || refused.Failure.Execution != chat.CallFailureNotExecuted {
		t.Fatalf("new work crossed close fence: %+v %v", refused, err)
	}
	releaseProvider()
	if err := errors.Join(<-called, <-closed); err != nil {
		t.Fatal(err)
	}
}
