package plugin_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestExecutableBackendStreamsDecorationThroughOriginalModelNext(t *testing.T) {
	method, err := pluginsdk.ModelStreamMethod()
	if err != nil {
		t.Fatal(err)
	}
	contract, err := capability.New(capability.Config{Reference: "model-decoration", Composition: capability.Decoration, Scope: "call"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FLAME_TEST_STREAMING_BACKEND") == "1" {
		server := sdk.NewServer(&sdk.Implementation{Name: "public-executable-decorator"}, nil)
		backend, err := pluginsdk.NewBackend(server)
		if err == nil {
			err = method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.Request]) iter.Seq2[pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error] {
				return func(yield func(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error) bool) {
					next, err := pluginsdk.CallbackFromContext(ctx)
					if err != nil {
						yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, err)
						return
					}
					request := command.Input.Clone()
					request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("captured transformed input"))
					if !yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{Chunk: &chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("decorated ")}}}, nil) {
						return
					}
					for value, err := range pluginsdk.StreamModel(ctx, next, command.ID, request) {
						if value.Result != nil && err == nil {
							value.Result.Failure = &pluginsdk.ModelFailure{Code: pluginsdk.ModelFailed, Message: "outer failed after known inner completion"}
						}
						if !yield(value, err) {
							return
						}
					}
				}
			})
		}
		if err == nil {
			err = backend.BindCapability("model-decoration", contract)
		}
		if err == nil {
			err = backend.Publish()
		}
		if err == nil {
			err = server.Run(context.Background(), &sdk.StdioTransport{})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "public-model-caller"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestExecutableBackendStreamsDecorationThroughOriginalModelNext$", "-test.count=1")
	command.Env = append(os.Environ(), "FLAME_TEST_STREAMING_BACKEND=1")
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	callbacks := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := callbacks.Close(); err != nil {
			t.Error(err)
		}
	})
	callbacks.AdmitSession(session)
	var calls atomic.Int32
	next, err := pluginsdk.NewModelStreamCallback(func(_ context.Context, command pluginsdk.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			calls.Add(1)
			if command.ID != "original-executable-model" || command.Input.Messages[0].Text() != "captured transformed input" {
				yield(nil, errors.New("next invocation replaced"))
				return
			}
			yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("known inner")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 13, OutputTokens: 2}}}, nil)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := callbacks.Open(t.Context(), session, pluginsdk.CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, next)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "model-decoration", contract)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := exported.Method(method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	if err != nil {
		t.Fatal(err)
	}
	var observed chat.ResponseAccumulator
	var final *pluginsdk.ModelResult
	for value, err := range method.StreamWithMetadata(t.Context(), binding, callbacks, pluginsdk.CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, "original-executable-model", *request, metadata) {
		if err != nil {
			t.Fatal(err)
		}
		if value.Chunk != nil {
			if err := observed.Add(value.Chunk); err != nil {
				t.Fatal(err)
			}
		} else {
			final = value.Result
		}
	}
	response, err := observed.Response()
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() != "decorated known inner" || final == nil || final.Response.Text() != "known inner" || final.Response.Metadata.Usage.InputTokens != 13 || final.Failure == nil || calls.Load() != 1 {
		t.Fatalf("real executable lost inner evidence: response=%+v result=%+v calls=%d", response, final, calls.Load())
	}
}
