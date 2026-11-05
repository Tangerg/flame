package execution

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestRichToolOffloadPreservesParallelDataAndOneBlobWithExactRanges(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image fixture"))
	if err != nil {
		t.Fatal(err)
	}
	first, last := strings.Repeat("界", 1500), strings.Repeat("last ", 1000)
	output := toolcontract.Output{Details: jsontext.Value(`{"revision":9007199254740993}`), Content: []content.Block{
		{Kind: content.Text, Text: first},
		{Kind: content.Media, Media: image},
		{Kind: content.Text, Text: "quoted source", Citations: []content.Citation{{Source: content.CitationSource{Kind: content.CitationSourceReference, Value: "source"}, Quote: "quoted source"}}},
		{Kind: content.Text, Text: strings.Repeat("indexed text ", 300), Metadata: metadata.Map{"positions": jsontext.Value(`[1,2,3]`)}},
		{Kind: content.Text, Text: last},
	}}
	before := output.Clone()
	store := new(fakeOffloader)
	policy, err := newToolResultOffloadPolicy(ToolResultOffloadPolicyValues{Threshold: intPointer(1000), ReaderName: testToolResultReaderName})
	if err != nil {
		t.Fatal(err)
	}
	projected, reference := evictToolContent(t.Context(), store, policy, "session:rich", "inspect", output)
	if reference == nil || !reference.ModelOnly || store.calls != 1 || store.lastStage.Body != first+last {
		t.Fatalf("rich offload lost its one text owner: reference=%+v calls=%d bytes=%d", reference, store.calls, len(store.lastStage.Body))
	}
	if !reflect.DeepEqual(output, before) || string(projected.Details) != string(output.Details) || len(projected.Content) != len(output.Content) {
		t.Fatal("offload mutated original evidence or parallel Details")
	}
	for _, index := range []int{1, 2, 3} {
		if !reflect.DeepEqual(projected.Content[index], output.Content[index]) {
			t.Fatalf("media, citation or opaque metadata changed at block %d", index)
		}
	}
	for _, index := range []int{0, 4} {
		if len(projected.Content[index].Text) >= len(output.Content[index].Text) || !strings.Contains(projected.Content[index].Text, reference.ID.String()) || len(projected.Content[index].Metadata) == 0 {
			t.Fatalf("block %d lost its bounded preview and typed reader", index)
		}
	}
	if !strings.Contains(projected.Content[4].Text, fmt.Sprintf(`"offset_bytes":%d`, len(first))) {
		t.Fatal("second block did not identify its exact byte offset")
	}
	reminder, found, err := modeladapter.RetainedToolResultReferences([]chat.Message{chat.NewToolMessage(chat.ToolResult{ID: "inspect", Name: "inspect", Output: projected})})
	if err != nil || !found || strings.Count(reminder.Text(), reference.ID.String()) != 2 ||
		!strings.Contains(reminder.Text(), fmt.Sprintf(`"range":{"offset_bytes":0,"length_bytes":%d}`, len(first))) ||
		!strings.Contains(reminder.Text(), fmt.Sprintf(`"range":{"offset_bytes":%d,"length_bytes":%d}`, len(first), len(last))) {
		t.Fatalf("compaction lost original block ranges within the shared blob: %q %v", reminder.Text(), err)
	}

	store.err = errors.New("storage unavailable")
	failed, reference := evictToolContent(t.Context(), store, policy, "session:rich", "inspect", output)
	if reference != nil || !reflect.DeepEqual(failed, output) {
		t.Fatal("failed staging replaced the original rich result")
	}
}
