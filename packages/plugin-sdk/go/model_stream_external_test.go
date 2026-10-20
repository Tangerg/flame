package plugin_test

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestExecutableConsumesFrozenRemainingModelStream(t *testing.T) {
	if encoded := os.Getenv("FLAME_TEST_MODEL_STREAM_CALLBACK"); encoded != "" {
		var ref pluginsdk.CallbackRef
		if err := json.Unmarshal([]byte(encoded), &ref); err != nil {
			t.Fatal(err)
		}
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("transformed request")))
		if err != nil {
			t.Fatal(err)
		}
		var accumulated chat.ResponseAccumulator
		var final *pluginsdk.ModelResult
		for value, err := range pluginsdk.StreamModel(t.Context(), ref, "frozen-model-call", request) {
			if err != nil {
				t.Fatal(err)
			}
			if value.Chunk != nil {
				if err := accumulated.Add(value.Chunk); err != nil {
					t.Fatal(err)
				}
			} else {
				final = value.Result
			}
		}
		response, err := accumulated.Response()
		if err != nil {
			t.Fatal(err)
		}
		if final == nil || final.Failure != nil || final.Response.Text() != "first second base" || response.Text() != final.Response.Text() || response.Metadata.Usage.InputTokens != 7 {
			t.Fatalf("remaining frozen stream lost: %+v", final)
		}
		return
	}
	host := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	var calls atomic.Int32
	base := chat.StreamerFunc(func(_ context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			calls.Add(1)
			if request.Messages[0].Text() != "transformed request" {
				yield(nil, errors.New("transformed input replaced"))
				return
			}
			yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("base")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 7, OutputTokens: 3}}}, nil)
		}
	})
	prefix := func(text string) chat.StreamMiddleware {
		return func(next chat.Streamer) chat.Streamer {
			return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
				return func(yield func(*chat.ResponseDelta, error) bool) {
					if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta(text)}}, nil) {
						return
					}
					for delta, err := range next.Stream(ctx, request) {
						if !yield(delta, err) {
							return
						}
					}
				}
			})
		}
	}
	order := []chat.StreamMiddleware{prefix("first "), prefix("second ")}
	remaining := chat.WrapStream(base, order...)
	callback, err := pluginsdk.NewModelStreamCallback(func(ctx context.Context, command pluginsdk.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
		if command.ID != "frozen-model-call" {
			return func(yield func(*chat.ResponseDelta, error) bool) { yield(nil, errors.New("call identity replaced")) }
		}
		return remaining.Stream(ctx, &command.Input)
	})
	if err != nil {
		t.Fatal(err)
	}
	order[0] = prefix("replacement ")
	ref, release, err := host.Open(t.Context(), session, pluginsdk.CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExecutableConsumesFrozenRemainingModelStream$", "-test.count=1")
	command.Env = append(os.Environ(), "FLAME_TEST_MODEL_STREAM_CALLBACK="+string(encoded))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("public remaining stream failed: %s: %v", output, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("remaining chain repeated model work: %d", calls.Load())
	}
}
