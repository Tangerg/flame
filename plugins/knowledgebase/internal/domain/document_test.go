package knowledge

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestDocumentOwnsRevisionChunksAndVectorCorrespondence(t *testing.T) {
	content := Content{ID: "guide", Title: "Guide", Source: "docs/guide.md", Text: strings.Repeat("界", chunkRunes+17)}
	draft, err := NewDraft(content)
	if err != nil {
		t.Fatal(err)
	}
	if texts := draft.Texts(); len(texts) != 2 || strings.Join(texts, "") != content.Text {
		t.Fatalf("source text changed during chunking: %v", texts)
	}
	indexed, err := draft.Index("space-a", [][]float32{{1, 0}, {0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := (Document{}).Replace(0, indexed)
	if err != nil {
		t.Fatal(err)
	}
	if document.Header().Revision != 1 {
		t.Fatal("new document did not start at revision one")
	}
	if _, err := document.Replace(0, indexed); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision = %v", err)
	}
	if err := document.CheckDeletion(2); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale deletion = %v", err)
	}
	snapshot := document.Snapshot()
	snapshot.Vectors[0][0] = -1
	snapshot.ChunkEnds[0] = 1
	matches, err := document.Search("space-a", []float32{1, 0})
	if err != nil || len(matches) != 1 || matches[0].Score != 1 || matches[0].Text != draft.Texts()[0] {
		t.Fatalf("detached snapshot changed the index: %+v, %v", matches, err)
	}
	if _, err := document.Search("space-b", []float32{1, 0}); !errors.Is(err, ErrEmbeddingSpace) {
		t.Fatalf("same-dimensional spaces were mixed: %v", err)
	}
	for _, vectors := range [][][]float32{nil, {{1}}, {{1}, {1, 2}}, {{0, 0}, {1, 0}}, {{float32(math.Inf(1))}, {1}}} {
		if _, err := draft.Index("space-a", vectors); !errors.Is(err, ErrInvalidIndex) {
			t.Fatalf("invalid correspondence admitted: %v, %v", vectors, err)
		}
	}
	restored, err := Restore(document.Snapshot())
	if err != nil || restored.Header() != document.Header() {
		t.Fatalf("restore = %+v, %v", restored, err)
	}
}

func TestDocumentChunksKeepParagraphsAndFencesTogether(t *testing.T) {
	paragraph := strings.Repeat("段", 700) + "\n\n"
	code := "```go\n" + strings.Repeat("// line\n", 70) + "```\n"
	content := Content{ID: "structured", Title: "Structure", Source: "guide.md", Text: paragraph + code + strings.Repeat("尾", 600)}
	draft, err := NewDraft(content)
	if err != nil {
		t.Fatal(err)
	}
	texts := draft.Texts()
	if len(texts) != 3 || texts[0] != paragraph || texts[1] != code || strings.Join(texts, "") != content.Text {
		t.Fatalf("chunking split a fitting source block: %#v", texts)
	}
}

func TestChunkEnvelopePreservesEveryRune(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("界", maxContentRunes),
		strings.Repeat("x\n\n", maxContentRunes/3),
		strings.Repeat(strings.Repeat("x", 510)+"\n\n", maxContentRunes/512),
		"~~~text\n" + strings.Repeat("界", maxContentRunes-12) + "\n~~~",
	} {
		draft, err := NewDraft(Content{ID: "id", Title: "title", Source: "source", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		if len(draft.chunkEnds) > MaxChunks || strings.Join(draft.Texts(), "") != text {
			t.Fatalf("chunk envelope changed the source: %v", draft.chunkEnds)
		}
		last := 0
		for _, end := range draft.chunkEnds {
			if end <= last || end-last > chunkRunes {
				t.Fatalf("invalid admitted chunk range: %d..%d", last, end)
			}
			last = end
		}
	}
}

func TestRestoreUsesAdmittedBoundariesAndRejectsUnboundVectors(t *testing.T) {
	// A retained index may have valid boundaries different from today's chunker.
	// Its vectors must still describe exactly that retained text.
	snapshot := Snapshot{
		Content:  Content{ID: "id", Title: "title", Source: "source", Text: "abcd"},
		Revision: 1, Space: "space", ChunkEnds: []int{2, 4}, Vectors: [][]float32{{1, 0}, {0, 1}},
	}
	document, err := Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := document.Search("space", []float32{0, 1})
	if err != nil || len(matches) != 1 || matches[0].Chunk != 2 || matches[0].Text != "cd" {
		t.Fatalf("restore reinterpreted the original vector source: %+v, %v", matches, err)
	}
	for _, ends := range [][]int{nil, {}, {0, 4}, {3, 2, 4}, {2, 2, 4}, {2, 5}, {2, 3}, {4}, {1, 2, 3, 4}} {
		snapshot.ChunkEnds = ends
		if _, err := Restore(snapshot); !errors.Is(err, ErrInvalidIndex) {
			t.Fatalf("unbound index accepted ranges %v: %v", ends, err)
		}
	}
}

func TestSearchAbstainsFromNonpositiveSimilarity(t *testing.T) {
	draft, err := NewDraft(Content{ID: "id", Title: "title", Source: "source", Text: "source"})
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := draft.Index("space", [][]float32{{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := (Document{}).Replace(0, indexed)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range [][]float32{{0, 1}, {-1, 0}} {
		matches, err := document.Search("space", query)
		if err != nil || len(matches) != 0 {
			t.Fatalf("unrelated or opposed vectors retrieved context: %+v, %v", matches, err)
		}
	}
}
