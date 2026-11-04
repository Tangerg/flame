package wire

import (
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"

	"github.com/Tangerg/scope/core/jsonschema"
)

type ImportInput struct {
	Content          knowledge.Content `json:"content"`
	ExpectedRevision int64             `json:"expectedRevision" jsonschema:"minimum=0"`
}

type DocumentInput struct {
	ID               string `json:"id" jsonschema:"minLength=1"`
	ExpectedRevision int64  `json:"expectedRevision" jsonschema:"minimum=1"`
}

type ReadInput struct {
	ID string `json:"id" jsonschema:"minLength=1"`
}

func (ReadInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"id": {MaxLength: new(uint64(knowledge.MaxIDRunes))},
	}}
}

type DocumentRead struct {
	Document knowledge.Header `json:"document"`
	Text     string           `json:"text"`
}

func (DocumentInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"id": {MaxLength: new(uint64(knowledge.MaxIDRunes))},
	}}
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1"`
	Limit int    `json:"limit" jsonschema:"minimum=1"`
}

func (SearchInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"query": {MaxLength: new(uint64(knowledge.MaxQueryRunes))},
		"limit": {Maximum: new(float64(knowledge.MaxResults))},
	}}
}
