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

func TestTranscriptCodecRejectsRemovedToolFailureMetadata(t *testing.T) {
	for field, metadata := range map[string]string{
		"scope":             `"scope":"tool"`,
		"retryAfterSeconds": `"retryAfterSeconds":1`,
	} {
		t.Run(field, func(t *testing.T) {
			encoded := `{"status":"incomplete","kind":"toolCall","failure":{"kind":"tool_failed",` + metadata + `}}`
			_, err := decodeTranscriptItem([]byte(encoded))
			if err == nil || !strings.Contains(err.Error(), `unknown field "`+field+`"`) {
				t.Fatalf("decodeTranscriptItem error = %v, want removed %s rejection", err, field)
			}
		})
	}
}
