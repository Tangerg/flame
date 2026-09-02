package sqlite

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func TestItemKindRejectsUnknownPersistentIdentity(t *testing.T) {
	if kind := transcript.ItemKind("unknown"); kind.Valid() {
		t.Fatalf("ItemKind(%q) is valid", kind)
	}
	if !transcript.ToolCall.Valid() {
		t.Fatal("ToolCall is invalid")
	}
}

func TestToolCancellationFailureKindRoundTrips(t *testing.T) {
	encoded := tool.FailureCanceled.String()
	if encoded != "tool_canceled" {
		t.Fatalf("encoded canceled Tool failure = %q", encoded)
	}
	decoded := tool.FailureKind(encoded)
	if decoded != tool.FailureCanceled {
		t.Fatalf("decoded canceled Tool failure = %v", decoded)
	}
}

func TestTranscriptCodecRejectsRemovedToolRetryMetadata(t *testing.T) {
	_, err := decodeTranscriptItem([]byte(`{
		"status":"incomplete",
		"kind":"toolCall",
		"failure":{"kind":"tool_failed","scope":"tool","retryAfterSeconds":1}
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "retryAfterSeconds"`) {
		t.Fatalf("decodeTranscriptItem error = %v, want removed retryAfterSeconds rejection", err)
	}
}
