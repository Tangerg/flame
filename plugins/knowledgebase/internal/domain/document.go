package knowledge

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/invopop/jsonschema"
)

const (
	MaxDocuments    = 32
	maxContentRunes = 16384
	MaxIDRunes      = 128
	maxTitleRunes   = 256
	maxSourceRunes  = 2048
	MaxQueryRunes   = 4096
	chunkRunes      = 1024
	MaxResults      = 8
)

var (
	ErrInvalidDocument  = errors.New("knowledge base: invalid document")
	ErrRevisionConflict = errors.New("knowledge base: document revision changed")
	ErrCapacity         = errors.New("knowledge base: document capacity reached")
	ErrNotFound         = errors.New("knowledge base: document not found")
	ErrRetired          = errors.New("knowledge base: document identity is retired; import under a new identity")
	ErrInvalidIndex     = errors.New("knowledge base: invalid embedding index")
	ErrEmbeddingSpace   = errors.New("knowledge base: embedding space changed; reindex the documents")
)

type Content struct {
	ID     string `json:"id" jsonschema:"minLength=1"`
	Title  string `json:"title" jsonschema:"minLength=1"`
	Source string `json:"source" jsonschema:"minLength=1"`
	Text   string `json:"text" jsonschema:"minLength=1"`
}

func (Content) JSONSchemaExtend(schema *jsonschema.Schema) {
	for field, maximum := range map[string]uint64{
		"id": MaxIDRunes, "title": maxTitleRunes, "source": maxSourceRunes, "text": maxContentRunes,
	} {
		if property, ok := schema.Properties.Get(field); ok {
			property.MaxLength = new(maximum)
		}
	}
}

type Header struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Source   string `json:"source"`
	Revision int64  `json:"revision"`
	Chunks   int    `json:"chunks"`
	Space    string `json:"space"`
}

type Snapshot struct {
	Content  Content     `json:"content"`
	Revision int64       `json:"revision"`
	Space    string      `json:"space"`
	Vectors  [][]float32 `json:"vectors"`
}

type Draft struct {
	content Content
	chunks  []string
}

func NewDraft(content Content) (Draft, error) {
	for _, field := range []struct {
		value   string
		maximum int
	}{
		{content.ID, MaxIDRunes}, {content.Title, maxTitleRunes}, {content.Source, maxSourceRunes}, {content.Text, maxContentRunes},
	} {
		if !utf8.ValidString(field.value) || strings.TrimSpace(field.value) == "" || utf8.RuneCountInString(field.value) > field.maximum {
			return Draft{}, ErrInvalidDocument
		}
	}
	text := []rune(content.Text)
	chunks := make([]string, 0, (len(text)+chunkRunes-1)/chunkRunes)
	for len(text) > 0 {
		end := min(chunkRunes, len(text))
		chunks = append(chunks, string(text[:end]))
		text = text[end:]
	}
	return Draft{content: content, chunks: chunks}, nil
}

func (d Draft) Texts() []string { return slices.Clone(d.chunks) }

func (d Draft) Content() Content { return d.content }

type index struct {
	space   string
	vectors [][]float32
}

type IndexedDraft struct {
	draft Draft
	index index
}

func (d Draft) Index(space string, vectors [][]float32) (IndexedDraft, error) {
	if space == "" || len(vectors) == 0 || len(vectors) != len(d.chunks) {
		return IndexedDraft{}, ErrInvalidIndex
	}
	for _, vector := range vectors {
		if len(vector) != len(vectors[0]) || vectorNorm(vector) == 0 {
			return IndexedDraft{}, ErrInvalidIndex
		}
	}
	return IndexedDraft{draft: d, index: index{space: space, vectors: cloneVectors(vectors)}}, nil
}

type Document struct {
	draft    Draft
	revision int64
	index    index
}

func Restore(snapshot Snapshot) (Document, error) {
	draft, err := NewDraft(snapshot.Content)
	if err != nil {
		return Document{}, err
	}
	indexed, err := draft.Index(snapshot.Space, snapshot.Vectors)
	if err != nil {
		return Document{}, err
	}
	if snapshot.Revision <= 0 || snapshot.Revision > exactint.Maximum {
		return Document{}, ErrInvalidDocument
	}
	return Document{draft: draft, revision: snapshot.Revision, index: indexed.index}, nil
}

func (d Document) Replace(expected int64, indexed IndexedDraft) (Document, error) {
	if len(indexed.draft.chunks) == 0 {
		return Document{}, ErrInvalidDocument
	}
	if err := d.CheckReplacement(expected, indexed.draft.content.ID); err != nil {
		return Document{}, err
	}
	return Document{draft: indexed.draft, revision: d.revision + 1, index: indexed.index}, nil
}

func (d Document) CheckReplacement(expected int64, id string) error {
	if expected != d.revision || d.revision == exactint.Maximum {
		return ErrRevisionConflict
	}
	if d.revision > 0 && d.draft.content.ID != id {
		return ErrInvalidDocument
	}
	return nil
}

func (d Document) CheckDeletion(expected int64) error {
	if d.revision == 0 {
		return ErrNotFound
	}
	if expected != d.revision {
		return ErrRevisionConflict
	}
	return nil
}

func (d Document) PrepareReindex(expected int64) (Draft, error) {
	if d.revision == 0 {
		return Draft{}, ErrNotFound
	}
	if err := d.CheckReplacement(expected, d.draft.content.ID); err != nil {
		return Draft{}, err
	}
	return d.draft, nil
}

func (d Document) Content() Content { return d.draft.content }

func (d Document) Header() Header {
	return Header{ID: d.draft.content.ID, Title: d.draft.content.Title, Source: d.draft.content.Source, Revision: d.revision, Chunks: len(d.draft.chunks), Space: d.index.space}
}

func (d Document) Snapshot() Snapshot {
	return Snapshot{Content: d.draft.content, Revision: d.revision, Space: d.index.space, Vectors: cloneVectors(d.index.vectors)}
}

type Match struct {
	Document Header  `json:"document"`
	Chunk    int     `json:"chunk"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
}

func (d Document) Search(space string, query []float32) ([]Match, error) {
	if space != d.index.space {
		return nil, ErrEmbeddingSpace
	}
	queryNorm := vectorNorm(query)
	if queryNorm == 0 || len(query) != len(d.index.vectors[0]) {
		return nil, ErrInvalidIndex
	}
	matches := make([]Match, 0, len(d.draft.chunks))
	for i, vector := range d.index.vectors {
		var dot float64
		for j, value := range vector {
			dot += float64(value) * float64(query[j])
		}
		score := dot / (queryNorm * vectorNorm(vector))
		matches = append(matches, Match{Document: d.Header(), Chunk: i + 1, Text: d.draft.chunks[i], Score: max(-1, min(1, score))})
	}
	return matches, nil
}

func Rank(matches []Match, limit int) []Match {
	slices.SortFunc(matches, func(a, b Match) int {
		if order := cmp.Compare(b.Score, a.Score); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Document.ID, b.Document.ID); order != 0 {
			return order
		}
		return cmp.Compare(a.Chunk, b.Chunk)
	})
	return matches[:min(limit, len(matches))]
}

func vectorNorm(vector []float32) float64 {
	var sum float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return 0
		}
		sum += float64(value) * float64(value)
	}
	return math.Sqrt(sum)
}

func cloneVectors(vectors [][]float32) [][]float32 {
	result := make([][]float32, len(vectors))
	for i, vector := range vectors {
		result[i] = slices.Clone(vector)
	}
	return result
}
