package maintenance

import (
	"strings"
	"testing"
	"unicode/utf8"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestCompactionChunksPreserveAllBytesAndRuneBoundaries(t *testing.T) {
	text := strings.Repeat("α reference line\n", 10_000) + strings.Repeat("界", 10_000)
	chunks := compactionChunks(text, 4096)
	if len(chunks) < 2 || strings.Join(chunks, "") != text {
		t.Fatal("chunking omitted or duplicated transcript bytes")
	}
	for _, chunk := range chunks {
		if len(chunk) == 0 || len(chunk) > 4096 || !utf8.ValidString(chunk) {
			t.Fatal("chunk exceeded capacity or split a UTF-8 rune")
		}
	}
}

func TestCompactionSummarizesEveryPortionWithinTheAuxiliaryEnvelope(t *testing.T) {
	first := "FIRST_PORTION_FACT"
	last := "LAST_PORTION_FACT"
	messages := []chat.Message{chat.NewUserMessage(chat.NewTextPart(first + strings.Repeat("界", 300_000) + last))}
	model := newTextStubModel("retained portion facts")
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	compactor := mustNewCompactor(t, newCompactionTestStore(), constClient(client), nil, CompactionPolicyValues{})
	if _, err := compactor.summarize(t.Context(), messages); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) < 3 {
		t.Fatal("large transcript was summarized as a cropped single request")
	}
	var portions strings.Builder
	for _, request := range model.requests[:len(model.requests)-1] {
		input := request.Messages[1].Text()
		if len(input)+len(request.Messages[0].Text()) > maxModelInputBytes {
			t.Fatal("partial summary exceeded its captured input envelope")
		}
		_, body, found := strings.Cut(input, "\n")
		if !found {
			t.Fatal("partial summary lost its portion framing")
		}
		portions.WriteString(body)
	}
	if portions.String() != modeladapter.RenderCompactionTranscript(messages) {
		t.Fatal("partial summary requests did not contain the complete original text")
	}
}

func TestCompactionRetainsOffloadHandleWhenSummaryOmitsIt(t *testing.T) {
	ref := transcript.ToolResultRef{ID: "BLOB234"}
	// The real fresh preview is larger than the former 2,000-byte trim cap.
	output, err := modeladapter.WithToolResultReference(toolcontract.NewTextOutput(strings.Repeat("x", 2194)), ref, "read_tool_result", modeladapter.ToolResultRange{LengthBytes: 32000})
	if err != nil {
		t.Fatal(err)
	}
	history := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("read the file")),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "read-1", Name: "read", Arguments: `{}`})),
		chat.NewToolMessage(chat.ToolResult{ID: "read-1", Name: "read", Output: output}),
		chat.NewUserMessage(chat.NewTextPart("continue")),
	}
	const sessionID = "session:retained-reference"
	store := newCompactionTestStore()
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	model := newTextStubModel("Read completed. Continue the task.")
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	compactor := mustNewCompactor(t, store, constClient(client), nil, CompactionPolicyValues{MaxTokens: intPointer(500)})
	result, err := compactor.CompactModelContext(t.Context(), durableContextRequest(t, sessionID, history, 0, nil))
	if err != nil {
		t.Fatal(err)
	}
	reminder, found, err := modeladapter.RetainedToolResultReferences(result.Messages())
	if err != nil || !found || !strings.Contains(reminder.Text(), `"result_id":"BLOB234"`) || !strings.Contains(reminder.Text(), "read_tool_result") || !strings.Contains(reminder.Text(), `"range":{"offset_bytes":0,"length_bytes":32000}`) {
		t.Fatalf("summary discarded its typed reader handle: %s %t %v", reminder.Text(), found, err)
	}
	if !result.Summarized() || strings.Contains(result.Summary(), ref.ID.String()) {
		t.Fatal("fixture did not prove retention independent of the LLM summary")
	}
}
