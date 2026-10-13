package plugin

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelMethodUsesScopeExchangeContract(t *testing.T) {
	method, err := ModelMethod()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []jsontext.Value{[]byte(`{}`), []byte(`{"failure":{"code":"failed","message":" "}}`)} {
		if err := method.contract.ValidateOutput(value); err == nil {
			t.Fatalf("model contract accepted an absent outcome: %s", value)
		}
	}
}

func TestModelCallRejectsInvalidInputBeforeTransport(t *testing.T) {
	if _, err := CallModel(t.Context(), CallbackRef{}, "model-call", &chat.Request{}); !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("invalid input entered model transport: %v", err)
	}
}

func TestCapturedModelCancellationStopsOwnerWork(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	started, stopped := make(chan struct{}), make(chan struct{})
	callback, err := NewModelCallback(func(ctx context.Context, _ Command[chat.Request]) (*chat.Response, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := CallModel(ctx, ref, "cancelable-model-call", request)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("model owner did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("model cancellation lost its cause: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("model caller did not settle")
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled model owner retained work")
	}
}

func TestModelCallbackRetainsCompleteScopeResponseAndAuthority(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	var calls atomic.Int32
	message := chat.NewAssistantMessage(chat.NewTextPart("answer"), chat.NewReasoningPart("basis", nil))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "inner-model", Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: new(int64(0))}}}
	callback, err := NewModelCallback(func(ctx context.Context, command Command[chat.Request]) (*chat.Response, error) {
		if ctx.Value(callbackOwnerKey{}) != "captured-model" || command.ID != "original-model-call" {
			return nil, errors.New("captured model identity changed")
		}
		calls.Add(1)
		return response, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := context.WithValue(t.Context(), callbackOwnerKey{}, "captured-model")
	ref, release, err := host.Open(owner, session, CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := CallModel(t.Context(), ref, "original-model-call", request)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || actual.Metadata.Model != "inner-model" || actual.Metadata.Usage.InputTokens != 12 || actual.Metadata.Usage.CacheReadInputTokens == nil || *actual.Metadata.Usage.CacheReadInputTokens != 0 || len(actual.Output.Message.Parts) != 2 {
		t.Fatalf("complete model evidence lost: %+v", actual)
	}
	actual.Metadata.Usage.InputTokens = 99
	if response.Metadata.Usage.InputTokens != 12 {
		t.Fatal("caller mutated the inner response evidence")
	}
	release()
	if _, err := CallModel(t.Context(), ref, "original-model-call", request); err == nil || calls.Load() != 1 {
		t.Fatal("retired model authority repeated provider I/O")
	}
}
