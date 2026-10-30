package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestInstalledModelDecoratorsObserveFrozenBeforeAfterOrder(t *testing.T) {
	binary := buildCallContributor(t)
	var mu sync.Mutex
	var order, identities []string
	trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var event struct {
			Layer string `json:"layer"`
			Phase string `json:"phase"`
			ID    string `json:"id"`
		}
		if err := json.UnmarshalRead(request.Body, &event); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		order = append(order, event.Layer+":"+event.Phase)
		identities = append(identities, event.ID)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer trace.Close()
	entered, proceed := make(chan struct{}), make(chan struct{})
	finish := sync.OnceFunc(func() { close(proceed) })
	defer finish()
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) > 0 && calls.Add(1) == 1 {
			close(entered)
			select {
			case <-proceed:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return completedTextResponse("decorated result"), nil
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	defer r.close()
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	outer := installCallContributor(t, r, binary, "outer", trace.URL, contract)
	inner := installCallContributor(t, r, binary, "inner", trace.URL, contract)
	first := protocol.PluginCompositionMember{InstallationID: outer.ID, Contribution: "contributor"}
	second := protocol.PluginCompositionMember{InstallationID: inner.ID, Contribution: "contributor"}
	setOrder := func(members []protocol.PluginCompositionMember, key string) {
		t.Helper()
		r.must(delivery.PluginsSetComposition, protocol.SetPluginCompositionRequest{Contract: contract.Identity().String(), Members: members}, key)
	}
	setOrder([]protocol.PluginCompositionMember{first, second}, "original-order")
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "remote order"}, "session").(*protocol.Session)
	start := func(key string) <-chan error {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: key}}}, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		done := make(chan error, 1)
		go func() {
			for _, err := range result.Events {
				if err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
		return done
	}
	wait := func(done <-chan error) {
		t.Helper()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("decorated model did not settle")
		}
	}
	done := start("first")
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("both installed decorators did not enter the model")
	}
	setOrder([]protocol.PluginCompositionMember{second, first}, "next-order")
	finish()
	wait(done)
	wait(start("second"))
	mu.Lock()
	defer mu.Unlock()
	want := []string{"outer:before", "inner:before", "inner:after", "outer:after", "inner:before", "outer:before", "outer:after", "inner:after"}
	if !reflect.DeepEqual(order, want) || calls.Load() != 2 {
		t.Fatalf("installed composition order/calls changed: order=%v calls=%d", order, calls.Load())
	}
	for offset := 0; offset < len(identities); offset += 4 {
		if identities[offset] == "" {
			t.Fatal("remote decorators lost model identity")
		}
		for _, id := range identities[offset : offset+4] {
			if id != identities[offset] {
				t.Fatal("nested decorators replaced original identity")
			}
		}
	}
	if identities[0] == identities[4] {
		t.Fatalf("fresh Run reused an earlier model identity: %s", identities[0])
	}
}
