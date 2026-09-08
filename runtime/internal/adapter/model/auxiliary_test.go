package model

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type recordingModel struct {
	request *chat.Request
}

type auxiliaryResponseModel struct {
	response *chat.Response
}

func (m auxiliaryResponseModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return m.response, nil
}

func (r *recordingModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	r.request = request
	message := chat.NewAssistantMessage(chat.NewTextPart("completed"))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

func (r *recordingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return testsupport.StreamResponse(r.Call(ctx, request))
}

func TestCompleteBuildsOneMiddlewareFreePrompt(t *testing.T) {
	model := &recordingModel{}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := fixedAuxiliaryClient(&client).Complete(t.Context(), AuxiliaryPrompt{
		SystemPrompt: "system instructions", UserPrompt: "input",
		MaxInputBytes: 1024, MaxOutputTokens: 123,
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "completed" {
		t.Fatalf("completion = %q, want completed", text)
	}
	if model.request == nil || len(model.request.Messages) != 2 {
		t.Fatalf("request messages = %#v, want system and user", model.request)
	}
	if model.request.Messages[0].Role != chat.RoleSystem || model.request.Messages[0].Text() != "system instructions" {
		t.Fatalf("system message = %#v", model.request.Messages[0])
	}
	if model.request.Messages[1].Role != chat.RoleUser || model.request.Messages[1].Text() != "input" {
		t.Fatalf("user message = %#v", model.request.Messages[1])
	}
	if model.request.Options.MaxOutputTokens == nil || *model.request.Options.MaxOutputTokens != 123 {
		t.Fatalf("MaxOutputTokens = %v, want 123", model.request.Options.MaxOutputTokens)
	}
}

func TestCompleteRejectsMissingClient(t *testing.T) {
	_, err := fixedAuxiliaryClient(nil).Complete(t.Context(), AuxiliaryPrompt{MaxInputBytes: 1, MaxOutputTokens: 1})
	if err == nil || err.Error() != "auxiliary model: client is required" {
		t.Fatalf("Complete nil client error = %v", err)
	}
}

func TestCompleteRejectsInvalidResourceEnvelopeBeforeResolvingModel(t *testing.T) {
	resolver := AuxiliaryResolver(func(context.Context) (*chatclient.Client, error) {
		t.Fatal("invalid resource envelope reached the resolver")
		return nil, nil
	})
	for _, prompt := range []AuxiliaryPrompt{
		{MaxOutputTokens: 1},
		{MaxInputBytes: 1},
		{SystemPrompt: "system", UserPrompt: "input", MaxInputBytes: 5, MaxOutputTokens: 1},
	} {
		if _, err := resolver.Complete(t.Context(), prompt); err == nil {
			t.Fatalf("Complete(%+v) succeeded, want invalid resource envelope", prompt)
		}
	}
}

func TestCompletePreservesResolutionFailure(t *testing.T) {
	failure := errors.New("provider unavailable")
	resolver := AuxiliaryResolver(func(context.Context) (*chatclient.Client, error) {
		return nil, failure
	})
	text, err := resolver.Complete(t.Context(), AuxiliaryPrompt{MaxInputBytes: 1, MaxOutputTokens: 1})
	if text != "" || !errors.Is(err, failure) {
		t.Fatalf("Complete = (%q, %v), want exact resolution failure", text, err)
	}
}

func fixedAuxiliaryClient(client *chatclient.Client) AuxiliaryResolver {
	return func(context.Context) (*chatclient.Client, error) { return client, nil }
}

func TestCompleteRejectsIncompleteTextGenerations(t *testing.T) {
	partial := chat.NewAssistantMessage(chat.NewTextPart("unfinished summary"))
	refusal := chat.NewAssistantMessage(chat.NewRefusalPart("I cannot summarize that."))
	type completionCase struct {
		name       string
		response   *chat.Response
		wantReason string
	}
	cases := []completionCase{
		{
			name: "refusal",
			response: &chat.Response{Output: &chat.Output{
				Message: &refusal, FinishReason: chat.FinishReasonRefusal,
			}},
			wantReason: `finish reason "refusal"`,
		},
		{
			name: "contentless stop",
			response: &chat.Response{Output: &chat.Output{
				FinishReason: chat.FinishReasonStop,
			}},
			wantReason: `finish reason "stop"`,
		},
		{
			name:       "nil response",
			response:   nil,
			wantReason: "invalid response",
		},
	}

	for _, reason := range []chat.FinishReason{
		chat.FinishReasonLength, chat.FinishReasonContentFilter, chat.FinishReasonRefusal,
		chat.FinishReasonToolCalls, chat.FinishReasonOther,
	} {
		cases = append(cases, completionCase{
			name:       string(reason) + " with text",
			response:   &chat.Response{Output: &chat.Output{Message: &partial, FinishReason: reason}},
			wantReason: `finish reason "` + string(reason) + `"`,
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := chatclient.New(auxiliaryResponseModel{response: tc.response}, chatclient.Config{})
			if err != nil {
				t.Fatal(err)
			}
			text, err := fixedAuxiliaryClient(&client).Complete(t.Context(), AuxiliaryPrompt{
				SystemPrompt: "summarize", UserPrompt: "history",
				MaxInputBytes: 1024, MaxOutputTokens: 128,
			})
			if text != "" || err == nil || !strings.Contains(err.Error(), tc.wantReason) {
				t.Fatalf("Complete error = %v, want %q", err, tc.wantReason)
			}
		})
	}
}
