package conversation

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestConversationPreservesScopeToolCallIDs(t *testing.T) {
	for _, id := range []string{" call\u200b1\n", strings.Repeat("界", 513)} {
		messages := []chat.Message{
			chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: id, Name: "inspect", Arguments: `{}`})),
			chat.NewToolMessage(chat.ToolResult{ID: id, Name: "inspect", Output: chat.NewTextToolOutput("contents")}),
		}
		for _, message := range messages {
			if err := message.Validate(); err != nil {
				t.Fatalf("invalid Scope fixture: %v", err)
			}
		}
		history, err := New(messages)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(history.Messages(), messages) {
			t.Fatal("conversation changed a provider correlation ID")
		}
	}
}

func TestConversationRejectsMissingScopeToolCallIDs(t *testing.T) {
	for _, test := range []struct {
		name    string
		message chat.Message
		want    error
	}{
		{"call", chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{Name: "inspect", Arguments: `{}`})), chat.ErrInvalidToolCall},
		{"result", chat.NewToolMessage(chat.ToolResult{Name: "inspect", Output: chat.NewTextToolOutput("contents")}), chat.ErrInvalidToolResult},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New([]chat.Message{test.message}); !errors.Is(err, test.want) {
				t.Fatalf("New error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestConversationOwnsSequenceTransitions(t *testing.T) {
	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
	}
	history, err := New(seed)
	if err != nil {
		t.Fatal(err)
	}
	if history.Count() != 2 {
		t.Fatalf("count = %d, want 2", history.Count())
	}
	history, err = history.Append(chat.NewAssistantMessage(chat.NewTextPart("four")))
	if err != nil {
		t.Fatal(err)
	}
	messages := history.Messages()
	if len(messages) != 3 || messages[0].Text() != "one" || messages[2].Text() != "four" {
		t.Fatalf("messages = %#v", messages)
	}
	messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
	if history.Messages()[0].Text() != "one" {
		t.Fatal("Messages leaked aggregate ownership")
	}
}

func TestCloseOpenToolCallsClosesOnlyLatestUnresolvedGenerations(t *testing.T) {
	history, err := New([]chat.Message{
		chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "call_reused", Name: "first"}),
			chat.NewToolCallPart(chat.ToolCall{ID: "call_parallel", Name: "parallel"}),
		),
		chat.NewToolMessage(chat.ToolResult{ID: "call_reused", Name: "first", Output: chat.NewTextToolOutput("done")}),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_reused", Name: "second"})),
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, appended, err := history.CloseOpenToolCalls("execution was lost")
	if err != nil {
		t.Fatal(err)
	}
	if history.Count() != 3 || closed.Count() != 4 || len(appended) != 1 {
		t.Fatalf("counts/appended = %d/%d/%d, want 3/4/1", history.Count(), closed.Count(), len(appended))
	}
	want := chat.NewToolMessage(
		chat.ToolResult{ID: "call_parallel", Name: "parallel", Output: chat.NewTextToolOutput("execution was lost"), IsError: true},
		chat.ToolResult{ID: "call_reused", Name: "second", Output: chat.NewTextToolOutput("execution was lost"), IsError: true},
	)
	if !reflect.DeepEqual(appended[0], want) || !reflect.DeepEqual(closed.Messages()[3], want) {
		t.Fatalf("closure = %#v, want %#v", appended, want)
	}
	appended[0] = chat.NewToolMessage(chat.ToolResult{ID: "changed", Name: "changed"})
	if !reflect.DeepEqual(closed.Messages()[3], want) {
		t.Fatal("CloseOpenToolCalls leaked aggregate ownership")
	}
}

func TestCloseOpenToolCallsIsNoOpWhenConversationIsClosed(t *testing.T) {
	history, err := New([]chat.Message{
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call", Name: "read"})),
		chat.NewToolMessage(chat.ToolResult{ID: "call", Name: "read", Output: chat.NewTextToolOutput("done")}),
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, appended, err := history.CloseOpenToolCalls("unused")
	if err != nil || len(appended) != 0 || !reflect.DeepEqual(closed.Messages(), history.Messages()) {
		t.Fatalf("closed/appended/error = %#v/%#v/%v", closed.Messages(), appended, err)
	}
}

func TestCloseOpenToolCallsWithResultsPreservesProviderOrder(t *testing.T) {
	history, err := New([]chat.Message{chat.NewAssistantMessage(
		chat.NewToolCallPart(chat.ToolCall{ID: "first", Name: "read", Arguments: `{}`}),
		chat.NewToolCallPart(chat.ToolCall{ID: "second", Name: "glob", Arguments: `{}`}),
	)})
	if err != nil {
		t.Fatal(err)
	}
	closed, appended, err := history.CloseOpenToolCallsWithResults(
		"canceled",
		[]chat.ToolResult{{ID: "second", Name: "glob", Output: chat.NewTextToolOutput("known")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Count() != 2 || len(appended) != 1 || len(appended[0].Parts) != 2 {
		t.Fatalf("closed/appended = %d/%#v", closed.Count(), appended)
	}
	first := appended[0].Parts[0].ToolResult
	second := appended[0].Parts[1].ToolResult
	if first == nil || second == nil {
		t.Fatalf("ordered terminal results = %#v", appended[0].Parts)
	}
	firstText, firstTextual := first.Output.Text()
	secondText, secondTextual := second.Output.Text()
	if first.ID != "first" || !first.IsError || !firstTextual || firstText != "canceled" ||
		second.ID != "second" || second.IsError || !secondTextual || secondText != "known" {
		t.Fatalf("ordered terminal results = %#v", appended[0].Parts)
	}
}
