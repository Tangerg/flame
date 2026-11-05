package model

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
)

func TestRequestDiagnosticsSeparateHistoryMutationFromTailGrowth(t *testing.T) {
	request, _ := chat.NewRequest(chat.NewSystemMessage("stable instructions"), chat.NewUserMessage(chat.NewTextPart("private user text")))
	before, err := DescribeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	tail := request.Clone()
	tail.Messages = append(tail.Messages, chat.NewUserMessage(chat.NewTextPart("ephemeral reference")))
	after, err := DescribeRequest(tail)
	if err != nil || before.FirstChangedMessage(after) != 2 || before.SystemDigest != after.SystemDigest || before.ToolsDigest != after.ToolsDigest {
		t.Fatalf("tail growth was misidentified as a stable-prefix mutation: %+v %v", after, err)
	}
	changed := request.Clone()
	changed.Messages[0] = chat.NewSystemMessage("changed instructions")
	changed.Options.Temperature = new(0.2)
	altered, err := DescribeRequest(changed)
	if err != nil || before.FirstChangedMessage(altered) != 0 || before.SystemDigest == altered.SystemDigest || before.OptionsDigest == altered.OptionsDigest {
		t.Fatalf("instruction/options mutation was not diagnosed: %+v %v", altered, err)
	}
	if again, err := DescribeRequest(request.Clone()); err != nil || before.FirstChangedMessage(again) != -1 || before.MessagesDigest != again.MessagesDigest {
		t.Fatal("identical request changed its derived diagnostic")
	}
}

func TestCacheDiagnosticsDoNotInventMissingUsageOrExposePromptText(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("private user text")))
	invocation := modelref.EndpointInvocation{ID: "endpoint-fixture", Request: modelref.ChatEndpointRequest{Chat: request}}
	logRequestDiagnostics(t.Context(), invocation)
	logResponseDiagnostics(t.Context(), invocation, nil)
	if text := logs.String(); strings.Contains(text, "private user text") || strings.Contains(text, "cache.read.tokens") || !strings.Contains(text, `"usage.reported":false`) {
		t.Fatalf("diagnostics disclosed input or manufactured zero usage: %s", text)
	}
	logs.Reset()
	logResponseDiagnostics(t.Context(), invocation, &chat.Response{Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{CacheReadInputTokens: new(int64(0))}}})
	if text := logs.String(); !strings.Contains(text, `"cache.read.reported":true`) || !strings.Contains(text, `"cache.read.tokens":0`) || strings.Contains(text, "cache.write.tokens") {
		t.Fatalf("reported zero and unknown cache usage were conflated: %s", text)
	}
}
