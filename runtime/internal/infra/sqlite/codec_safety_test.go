package sqlite

import (
	"bytes"
	"strings"
	"testing"
	"time"

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

func TestTranscriptCodecPreservesEpochFinishTime(t *testing.T) {
	finishedAt := time.Unix(0, 0).UTC()
	item, err := transcript.RestoreItem(transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: "session_1", RunID: "run_1", ItemID: "item_1",
			OccurredAt: finishedAt.Add(-time.Second),
		},
		Status: transcript.ItemIncomplete, Kind: transcript.ToolCall,
		FinishedAt: finishedAt,
		Tool:       &transcript.ToolInvocation{Name: "shell"},
	})
	if err != nil {
		t.Fatalf("RestoreItem: %v", err)
	}

	encoded, err := encodeTranscriptItem(item)
	if err != nil {
		t.Fatalf("encodeTranscriptItem: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"finishedAt":0`)) {
		t.Fatalf("encoded item = %s, want explicit epoch finish time", encoded)
	}
	decoded, err := decodeTranscriptItem(encoded)
	if err != nil {
		t.Fatalf("decodeTranscriptItem: %v", err)
	}
	if !decoded.FinishedAt.Equal(finishedAt) {
		t.Fatalf("decoded finish time = %v, want %v", decoded.FinishedAt, finishedAt)
	}
}
