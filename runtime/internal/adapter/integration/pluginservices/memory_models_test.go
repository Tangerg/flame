package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	memory "github.com/Tangerg/flame/plugins/memory/backend/domain"
	"github.com/Tangerg/flame/plugins/memory/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/memorycontract"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func textModel(call func(context.Context, *chat.Request) (string, error)) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		text, err := call(ctx, request)
		if err != nil {
			return nil, err
		}
		message := chat.NewAssistantMessage(chat.NewTextPart(text))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})
}
func testModelRequest(prompt rpc.Prompt) *chat.Request {
	return &chat.Request{Messages: []chat.Message{chat.NewSystemMessage(prompt.SystemPrompt), chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt))}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens, OutputFormat: &chat.OutputFormat{Type: chat.OutputFormatText}}}
}

func testPrompt() rpc.Prompt {
	return rpc.Prompt{Operation: "memory.extract", SystemPrompt: "extract durable facts", UserPrompt: "captured transcript", MaxInputBytes: 128, MaxOutputTokens: 64}
}

func TestMemoryGrantKeepsCompleteModelEvidenceWithKnownResponseAndFailure(t *testing.T) {
	callbacks := pluginsdk.NewCallbacks()
	defer callbacks.Close()
	session := new(sdk.ClientSession)
	callbacks.AdmitSession(session)
	message := chat.NewAssistantMessage(chat.NewTextPart("fact"), chat.NewReasoningPart("basis", nil))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "captured-model", Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: new(int64(0))}}}
	grant, release, err := GrantMemoryModels(t.Context(), callbacks, session, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response, errors.New("outer observation failed")
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	actual, err := pluginsdk.CallModel(t.Context(), grant, "original", testModelRequest(testPrompt()))
	if err == nil || !reflect.DeepEqual(actual, response) {
		t.Fatalf("complete model evidence=%+v %v", actual, err)
	}
}

func TestModelTransportFailurePreservesCauseWithoutCapability(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	token := strings.Repeat("a", 64)
	_, err := (rpc.RemoteModels{Grant: pluginsdk.CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: token, Call: token, MaxResponseBytes: 1024}}).Complete(ctx, testPrompt())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled transport lost its cause: %v", err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatal("transport failure exposed the model capability")
	}
}

func TestModelGrantBindsSessionBudgetAndRevocation(t *testing.T) {
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	broker.AdmitSession(session)
	var calls atomic.Int32
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(context.Context, *chat.Request) (string, error) { calls.Add(1); return "fact", nil }), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	foreign := grant
	foreign.Credential = strings.Repeat("0", 64)
	if _, err := (rpc.RemoteModels{Grant: foreign}).Complete(t.Context(), testPrompt()); err == nil {
		t.Fatal("another connection borrowed model authority")
	}
	remote := rpc.RemoteModels{Grant: grant}
	for range 2 {
		value, err := remote.Complete(t.Context(), testPrompt())
		if err != nil || value != "fact" {
			t.Fatalf("completion=%q %v", value, err)
		}
	}
	if _, err := remote.Complete(t.Context(), testPrompt()); err == nil {
		t.Fatal("model budget exceeded")
	}
	release()
	release()
	if _, err := remote.Complete(t.Context(), testPrompt()); err == nil {
		t.Fatal("released grant accepted")
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestModelInvocationIdentityFencesExternalCalls(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint("response lost ", lost), func(t *testing.T) {
			callbacks := pluginsdk.NewCallbacks()
			t.Cleanup(func() {
				if err := callbacks.Close(); err != nil {
					t.Error(err)
				}
			})
			session := new(sdk.ClientSession)
			callbacks.AdmitSession(session)
			var calls atomic.Int32
			grant, release, err := GrantMemoryModels(t.Context(), callbacks, session, textModel(func(context.Context, *chat.Request) (string, error) {
				calls.Add(1)
				if lost {
					return "", errors.New("provider observation was lost")
				}
				return "original value", nil
			}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			method, err := pluginsdk.ModelMethod()
			if err != nil {
				t.Fatal(err)
			}
			if method.Descriptor().Kind != capability.Mutation {
				t.Fatal("external model call was declared as a query")
			}
			value, err := pluginsdk.CallModel(t.Context(), grant, "first", testModelRequest(testPrompt()))
			if !lost && (err != nil || value == nil || value.Output.Message.Text() != "original value") {
				t.Fatalf("original value=%+v error=%v", value, err)
			}
			if lost && err == nil {
				t.Fatal("lost provider result became known")
			}
			if _, err := pluginsdk.CallModel(t.Context(), grant, "first", testModelRequest(testPrompt())); err == nil {
				t.Fatal("original external call was redelivered")
			}
			changed := testPrompt()
			changed.UserPrompt = "changed input"
			_, err = pluginsdk.CallModel(t.Context(), grant, "first", testModelRequest(changed))
			failure, known := errors.AsType[*capability.Refusal](err)
			if !known || failure.Code != "command_conflict" {
				t.Fatalf("changed original input did not conflict: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("original identity caused %d provider calls", calls.Load())
			}
			_, _ = pluginsdk.CallModel(t.Context(), grant, "second", testModelRequest(testPrompt()))
			if calls.Load() != 2 {
				t.Fatalf("independent call did not consume the remaining budget: %d", calls.Load())
			}
			if _, err := pluginsdk.CallModel(t.Context(), grant, "third", testModelRequest(testPrompt())); err == nil || calls.Load() != 2 {
				t.Fatalf("budget exceeded: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}
func TestGrantRetirementCancelsAndJoinsAdmittedModelCall(t *testing.T) {
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	broker.AdmitSession(session)
	entered := make(chan struct{})
	settled := make(chan struct{})
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(ctx context.Context, _ *chat.Request) (string, error) {
		close(entered)
		<-ctx.Done()
		close(settled)
		return "", ctx.Err()
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() { _, err := (rpc.RemoteModels{Grant: grant}).Complete(t.Context(), testPrompt()); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("model failed before admission: %v", err)
	}
	broker.RevokeSession(session)
	select {
	case <-settled:
	default:
		t.Fatal("retirement returned before model call settled")
	}
	if err := <-done; err == nil {
		t.Fatal("canceled completion succeeded")
	}
}
func TestModelCapabilityDuringCurrentMCPRequest(t *testing.T) {
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	server := sdk.NewServer(&sdk.Implementation{Name: "memory"}, nil)
	binding, err := pluginsdk.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginsdk.RegisterServer(binding, "flame.memory/test.v1", func(error) string { return "model" }, func(ctx context.Context, _ *sdk.ServerSession, grant pluginsdk.CallbackRef) (string, error) {
		return (rpc.RemoteModels{Grant: grant}).Complete(ctx, testPrompt())
	}); err != nil {
		t.Fatal(err)
	}
	if err := binding.Publish(); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker.AdmitSession(session)
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(context.Context, *chat.Request) (string, error) { return "durable fact", nil }), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	value, err := pluginsdk.Call[pluginsdk.CallbackRef, string](t.Context(), session, "flame.memory/test.v1", grant)
	if err != nil || value != "durable fact" {
		t.Fatalf("round trip=%q %v", value, err)
	}
}
func TestCanceledInvocationRefusesNewModelCalls(t *testing.T) {
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	session := new(sdk.ClientSession)
	broker.AdmitSession(session)
	grant, release, err := GrantMemoryModels(ctx, broker, session, textModel(func(context.Context, *chat.Request) (string, error) { return "", errors.New("must not be called") }), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cancel()
	if _, err := (rpc.RemoteModels{Grant: grant}).Complete(t.Context(), testPrompt()); err == nil {
		t.Fatal("canceled invocation retained model authority")
	}
}

type boundedEmbeddings struct{ calls int }

func (*boundedEmbeddings) ID() string { return "fixture" }
func (e *boundedEmbeddings) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls++
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{1}
	}
	return vectors, nil
}

func TestEmbeddingCapabilityAcceptsMaximumEscapedActiveCorpus(t *testing.T) {
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	embedder := new(boundedEmbeddings)
	session := new(sdk.ClientSession)
	broker.AdmitSession(session)
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, nil, embedder)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	texts := make([]string, memory.MaxVisiblePerTarget*2)
	for i := range texts {
		texts[i] = strings.Repeat("\x00", memory.MaxContentCharacters)
	}
	vectors, err := (rpc.RemoteModels{Grant: grant, Space: embedder.ID()}).Embed(t.Context(), texts)
	if err != nil || len(vectors) != len(texts) || embedder.calls != 1 {
		t.Fatalf("maximum corpus: vectors=%d calls=%d error=%v", len(vectors), embedder.calls, err)
	}
}

func TestRetiredConnectionRefusesLateModelGrant(t *testing.T) {
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	broker.AdmitSession(session)
	broker.RevokeSession(session)
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, nil, nil)
	if err == nil {
		release()
		t.Fatalf("retired connection issued model capability %+v", grant)
	}
	if grant != (pluginsdk.CallbackRef{}) || release != nil {
		t.Fatal("refused grant retained model authority")
	}
}

func TestModelGrantRefusesOperationsOutsideCapturedAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*rpc.Prompt)
	}{
		{"unknown operation", func(p *rpc.Prompt) { p.Operation = "runtime.execute" }},
		{"ungranted embedding", func(p *rpc.Prompt) { p.Operation = "memory.embed"; p.UserPrompt = `["fact"]` }},
		{"unbounded input", func(p *rpc.Prompt) { p.MaxInputBytes = memorycontract.MaxModelInputBytes + 1 }},
		{"unbounded output", func(p *rpc.Prompt) { p.MaxOutputTokens = 2049 }},
		{"input exceeds envelope", func(p *rpc.Prompt) { p.MaxInputBytes = len(p.UserPrompt) - 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			broker := pluginsdk.NewCallbacks()
			t.Cleanup(func() {
				if err := broker.Close(); err != nil {
					t.Error(err)
				}
			})
			session := new(sdk.ClientSession)
			broker.AdmitSession(session)
			var calls atomic.Int32
			grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(context.Context, *chat.Request) (string, error) {
				calls.Add(1)
				return "fact", nil
			}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			prompt := testPrompt()
			test.edit(&prompt)
			if _, err := (rpc.RemoteModels{Grant: grant}).Complete(t.Context(), prompt); err == nil {
				t.Fatal("operation outside captured authority succeeded")
			}
			if calls.Load() != 0 {
				t.Fatal("refused operation reached the completion model")
			}
		})
	}
}
