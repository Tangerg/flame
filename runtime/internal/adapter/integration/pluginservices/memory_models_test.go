package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
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
func testModelRequest() *chat.Request {
	return &chat.Request{
		Messages: []chat.Message{chat.NewSystemMessage("extract durable facts"), chat.NewUserMessage(chat.NewTextPart("captured transcript"))},
		Options:  chat.Options{MaxOutputTokens: new(int64(64)), OutputFormat: &chat.OutputFormat{Type: chat.OutputFormatText}},
	}
}

func TestMemoryGrantKeepsCompleteModelEvidenceWithKnownResponseAndFailure(t *testing.T) {
	callbacks := qimumcp.NewCallbacks()
	defer callbacks.Close()
	session := new(sdk.ClientSession)
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("fact"), chat.NewReasoningPart("basis", nil))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "captured-model", Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: new(int64(0))}}}
	grant, release, err := GrantMemoryModels(t.Context(), callbacks, session, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response, errors.New("outer observation failed")
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	actual, err := pluginsdk.CallModel(t.Context(), grant, "original", testModelRequest())
	if err == nil || !reflect.DeepEqual(actual, response) {
		t.Fatalf("complete model evidence=%+v %v", actual, err)
	}
}

func TestModelTransportFailurePreservesCauseWithoutCapability(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	token := strings.Repeat("a", 64)
	_, err := pluginsdk.CallModel(ctx, qimumcp.CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: token, Call: token, MaxResponseBytes: 1024}, "original", testModelRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled transport lost its cause: %v", err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatal("transport failure exposed the model capability")
	}
}

func TestModelGrantBindsSessionBudgetAndRevocation(t *testing.T) {
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(context.Context, *chat.Request) (string, error) { calls.Add(1); return "fact", nil }), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	foreign := grant
	foreign.Credential = strings.Repeat("0", 64)
	if _, err := pluginsdk.CallModel(t.Context(), foreign, "foreign", testModelRequest()); err == nil {
		t.Fatal("another connection borrowed model authority")
	}
	for index := range memorycontract.MaxModelCalls {
		value, err := pluginsdk.CallModel(t.Context(), grant, fmt.Sprintf("original-%d", index), testModelRequest())
		if err != nil || value == nil || value.Output.Message.Text() != "fact" {
			t.Fatalf("completion=%+v %v", value, err)
		}
	}
	if _, err := pluginsdk.CallModel(t.Context(), grant, "outside-budget", testModelRequest()); err == nil {
		t.Fatal("model budget exceeded")
	}
	release()
	release()
	if _, err := pluginsdk.CallModel(t.Context(), grant, "after-release", testModelRequest()); err == nil {
		t.Fatal("released grant accepted")
	}
	if calls.Load() != memorycontract.MaxModelCalls {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestModelInvocationIdentityFencesExternalCalls(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint("response lost ", lost), func(t *testing.T) {
			callbacks := qimumcp.NewCallbacks()
			t.Cleanup(func() {
				if err := callbacks.Close(); err != nil {
					t.Error(err)
				}
			})
			session := new(sdk.ClientSession)
			if err := callbacks.AdmitSession(session); err != nil {
				t.Fatal(err)
			}
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
			if method.Descriptor().Kind != portable.Mutation {
				t.Fatal("external model call was declared as a query")
			}
			value, err := pluginsdk.CallModel(t.Context(), grant, "first", testModelRequest())
			if !lost && (err != nil || value == nil || value.Output.Message.Text() != "original value") {
				t.Fatalf("original value=%+v error=%v", value, err)
			}
			if lost && err == nil {
				t.Fatal("lost provider result became known")
			}
			if _, err := pluginsdk.CallModel(t.Context(), grant, "first", testModelRequest()); err == nil {
				t.Fatal("original external call was redelivered")
			}
			changed := testModelRequest()
			changed.Messages[1] = chat.NewUserMessage(chat.NewTextPart("changed input"))
			_, err = pluginsdk.CallModel(t.Context(), grant, "first", changed)
			failure, known := errors.AsType[*portable.Refusal](err)
			if !known || failure.Code != "command_conflict" {
				t.Fatalf("changed original input did not conflict: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("original identity caused %d provider calls", calls.Load())
			}
			_, _ = pluginsdk.CallModel(t.Context(), grant, "second", testModelRequest())
			if calls.Load() != memorycontract.MaxModelCalls {
				t.Fatalf("independent call did not consume the remaining budget: %d", calls.Load())
			}
			if _, err := pluginsdk.CallModel(t.Context(), grant, "third", testModelRequest()); err == nil || calls.Load() != memorycontract.MaxModelCalls {
				t.Fatalf("budget exceeded: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}
func TestGrantRetirementCancelsAndJoinsAdmittedModelCall(t *testing.T) {
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
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
	go func() { _, err := pluginsdk.CallModel(t.Context(), grant, "original", testModelRequest()); done <- err }()
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
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	server := sdk.NewServer(&sdk.Implementation{Name: "memory"}, nil)
	binding, err := qimumcp.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	method, err := qimumcp.NewMethod[qimumcp.CallbackRef, string](qimumcp.MethodConfig{Name: "fixture.memory/test.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Exercise the model callback captured by the Host."})
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(binding, func(error) string { return "model" }, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.CallbackRef]) (string, error) {
		response, err := pluginsdk.CallModel(ctx, command.Input, "original-model-call", testModelRequest())
		if err != nil {
			return "", err
		}
		return response.Output.Message.Text(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := binding.Publish(); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
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
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(context.Context, *chat.Request) (string, error) { return "durable fact", nil }), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	value, err := method.Call(t.Context(), session, grant)
	if err != nil || value != "durable fact" {
		t.Fatalf("round trip=%q %v", value, err)
	}
}
func TestCanceledInvocationRefusesNewModelCalls(t *testing.T) {
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	session := new(sdk.ClientSession)
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	grant, release, err := GrantMemoryModels(ctx, broker, session, textModel(func(context.Context, *chat.Request) (string, error) { return "", errors.New("must not be called") }), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cancel()
	if _, err := pluginsdk.CallModel(t.Context(), grant, "original", testModelRequest()); err == nil {
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
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	embedder := new(boundedEmbeddings)
	session := new(sdk.ClientSession)
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, nil, embedder)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	texts := make([]string, memorycontract.MaxEmbeddingTexts)
	// Every NUL expands to six JSON bytes; the array adds three bytes per
	// string and one closing byte. Build the largest evenly sized corpus
	// admitted by the publisher's projected request budget.
	characters := (memorycontract.MaxEmbeddingInputBytes - 1 - 3*len(texts)) / (6 * len(texts))
	for i := range texts {
		texts[i] = strings.Repeat("\x00", characters)
	}
	result, err := pluginsdk.CallTextEmbedding(t.Context(), grant, "original-embedding", texts)
	if err != nil || len(result.Vectors) != len(texts) || result.Space != embedder.ID() || embedder.calls != 1 {
		t.Fatalf("maximum corpus: vectors=%d space=%q calls=%d error=%v", len(result.Vectors), result.Space, embedder.calls, err)
	}
	texts[0] += strings.Repeat("x", memorycontract.MaxEmbeddingInputBytes-(len(texts)*(characters*6+3)+1)+1)
	if _, err := pluginsdk.CallTextEmbedding(t.Context(), grant, "oversized-embedding", texts); err == nil || embedder.calls != 1 {
		t.Fatalf("oversized corpus reached the embedding model: calls=%d error=%v", embedder.calls, err)
	}
}

func TestRetiredConnectionRefusesLateModelGrant(t *testing.T) {
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	broker.RevokeSession(session)
	grant, release, err := GrantMemoryModels(t.Context(), broker, session, nil, nil)
	if err == nil {
		release()
		t.Fatalf("retired connection issued model capability %+v", grant)
	}
	if grant != (qimumcp.CallbackRef{}) || release != nil {
		t.Fatal("refused grant retained model authority")
	}
}

func TestModelGrantRefusesRequestsOutsideCapturedAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*chat.Request)
	}{
		{"foreign model", func(request *chat.Request) { request.Options.Model = "foreign-model" }},
		{"different role", func(request *chat.Request) {
			request.Messages[0] = chat.NewAssistantMessage(chat.NewTextPart("foreign instruction"))
		}},
		{"unbounded output", func(request *chat.Request) {
			request.Options.MaxOutputTokens = new(int64(memorycontract.MaxModelOutputTokens + 1))
		}},
		{"input exceeds envelope", func(request *chat.Request) {
			request.Messages[1] = chat.NewUserMessage(chat.NewTextPart(strings.Repeat("x", memorycontract.MaxModelInputBytes+1)))
		}},
		{"missing captured format", func(request *chat.Request) { request.Options.OutputFormat = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			broker := qimumcp.NewCallbacks()
			t.Cleanup(func() {
				if err := broker.Close(); err != nil {
					t.Error(err)
				}
			})
			session := new(sdk.ClientSession)
			if err := broker.AdmitSession(session); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			grant, release, err := GrantMemoryModels(t.Context(), broker, session, textModel(func(context.Context, *chat.Request) (string, error) {
				calls.Add(1)
				return "fact", nil
			}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			request := testModelRequest()
			test.edit(request)
			if _, err := pluginsdk.CallModel(t.Context(), grant, "outside-envelope", request); err == nil {
				t.Fatal("request outside captured authority succeeded")
			}
			if calls.Load() != 0 {
				t.Fatal("refused request reached the completion model")
			}
		})
	}
}
