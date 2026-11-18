package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"

	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	coreinvocation "github.com/Tangerg/scope/core/invocation"
)

func connectProvider(t *testing.T, path string) (*store, *sdk.ClientSession, func()) {
	t.Helper()
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "provider-test", Version: "1"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
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
	ctx, err := pluginmcp.WithInvocation(t.Context(), framecall.Invocation{ID: "provider-test", Origin: extension.Automatic, Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: "providers"}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func providerMethod[P, R any](t *testing.T, session *sdk.ClientSession, method *pluginmcp.Method[P, R]) *pluginmcp.BoundMethod[P, R] {
	t.Helper()
	contract, err := modelprovider.Contract()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := pluginmcp.NewCapabilityBinding(session, "model-provider", contract)
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
			} else if result.Failure == nil || result.Failure.Execution != coreinvocation.FailureUnknown {
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
