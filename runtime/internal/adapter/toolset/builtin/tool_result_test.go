package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type fakeStore struct {
	body        string
	found       bool
	err         error
	lastSession string
	lastID      transcript.ToolResultID
}

func (f *fakeStore) Fetch(_ context.Context, session string, id transcript.ToolResultID) (string, bool, error) {
	f.lastSession, f.lastID = session, id
	return f.body, f.found, f.err
}

func sessionCtx(session string) context.Context {
	return executionctx.WithScope(context.Background(), runs.ExecutionScope{SessionID: session})
}

func TestRead_ReturnsStoredBody(t *testing.T) {
	store := &fakeStore{body: "ABCDEFGHIJ", found: true}
	tool, _ := NewToolResultReader(store)
	out, err := callTextTool(sessionCtx("sess-1"), tool, `{"result_id":"BLOB234"}`)
	if err != nil {
		t.Fatal(err)
	}
	if store.lastSession != "sess-1" || store.lastID != "BLOB234" {
		t.Fatalf("Fetch called with (%q, %q), want (sess-1, BLOB234)", store.lastSession, store.lastID)
	}
	if !strings.Contains(out, "ABCDEFGHIJ") || !strings.Contains(out, "10 bytes total") {
		t.Fatalf("output missing body or total:\n%s", out)
	}
}

func TestRead_PagesWithOffsetAndLimit(t *testing.T) {
	store := &fakeStore{body: "ABCDEFGHIJ", found: true}
	tool, _ := NewToolResultReader(store)
	out, err := callTextTool(sessionCtx("s"), tool, `{"result_id":"ABCDE234","offset_bytes":2,"limit_bytes":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CDE") || strings.Contains(out, "ABCDEFGHIJ") {
		t.Fatalf("windowed read did not return exactly the slice:\n%s", out)
	}
	// 5 < 10 → the tool should tell the model more remains and where to resume.
	if !strings.Contains(out, "remain") || !strings.Contains(out, `"offset_bytes":5`) {
		t.Fatalf("missing continuation hint:\n%s", out)
	}
}

func TestRead_UnknownIDIsRecoverable(t *testing.T) {
	tool, _ := NewToolResultReader(&fakeStore{found: false})
	out, err := callTextTool(sessionCtx("s"), tool, `{"result_id":"NOPE234"}`)
	if err != nil {
		t.Fatalf("an unknown id must not error: %v", err)
	}
	if !strings.Contains(out, "No stored tool result") {
		t.Fatalf("output = %q, want a not-found message", out)
	}
}

func TestRead_EmptyResultIDRejected(t *testing.T) {
	tool, _ := NewToolResultReader(&fakeStore{})
	if _, err := callTextTool(sessionCtx("s"), tool, `{"result_id":""}`); err == nil {
		t.Fatal("expected an error for an empty result_id")
	}
}

func TestRead_InvalidIDRejectedBeforeStore(t *testing.T) {
	store := new(fakeStore)
	tool, _ := NewToolResultReader(store)
	if _, err := callTextTool(sessionCtx("s"), tool, `{"result_id":"not-valid"}`); err == nil || store.lastID != "" {
		t.Fatalf("invalid result_id error = %v, store id = %q", err, store.lastID)
	}
}

func TestRead_NoSession(t *testing.T) {
	tool, _ := NewToolResultReader(&fakeStore{})
	out, err := callTextTool(context.Background(), tool, `{"result_id":"ABCDE234"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no active session") {
		t.Fatalf("output = %q, want a no-session message", out)
	}
}

func TestRead_RejectsRemovedAndOversizedPagingArguments(t *testing.T) {
	tool, _ := NewToolResultReader(&fakeStore{})
	for _, arguments := range []string{
		`{"id":"ABCDE234"}`,
		`{"result_id":"ABCDE234","offset":1}`,
		`{"result_id":"ABCDE234","limit":1}`,
		`{"result_id":"ABCDE234","limit_bytes":0}`,
		`{"result_id":"ABCDE234","limit_bytes":20001}`,
	} {
		if _, err := callTextTool(sessionCtx("s"), tool, arguments); err == nil {
			t.Fatalf("read_tool_result accepted invalid arguments: %s", arguments)
		}
	}
}
