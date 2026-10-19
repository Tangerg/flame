package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestDocumentOwnsRevisionChunksAndVectorCorrespondence(t *testing.T) {
	content := Content{ID: "guide", Title: "Guide", Source: "docs/guide.md", Text: strings.Repeat("界", ChunkRunes+17)}
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
	matches, err := document.Search("space-a", []float32{1, 0})
	if err != nil || len(matches) != 2 || matches[0].Score != 1 || matches[0].Text != draft.Texts()[0] {
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
