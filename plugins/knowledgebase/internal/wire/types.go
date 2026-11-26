package wire

import (
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"

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

// Every read sees the shared library plus, when Project is set, that Project's
// own documents.
type ListInput struct {
	Project string `json:"project,omitzero"`
}

type ReadInput struct {
	ID       string `json:"id" jsonschema:"minLength=1"`
	Project  string `json:"project,omitzero"`
	Revision int64  `json:"revision,omitzero" jsonschema:"minimum=1"`
	Page     int    `json:"page,omitzero" jsonschema:"minimum=1"`
}

func (ReadInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"id": {MaxLength: new(uint64(knowledge.MaxIDRunes))},
	}}
}

// Model Tools read within the Run's captured Project, never one named in
// arguments.
type ToolReadInput struct {
	ID       string `json:"id" jsonschema:"minLength=1"`
	Revision int64  `json:"revision,omitzero" jsonschema:"minimum=1"`
	Page     int    `json:"page,omitzero" jsonschema:"minimum=1"`
}

func (ToolReadInput) JSONSchemaConstraints() jsonschema.Constraints {
	return ReadInput{}.JSONSchemaConstraints()
}

type DocumentRead struct {
	Document knowledge.Header `json:"document"`
	Text     string           `json:"text"`
	Page     int              `json:"page,omitzero"`
	Pages    int              `json:"pages"`
}

func (DocumentInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"id": {MaxLength: new(uint64(knowledge.MaxIDRunes))},
	}}
}

type MoveInput struct {
	ID               string `json:"id" jsonschema:"minLength=1,maxLength=128"`
	ExpectedRevision int64  `json:"expectedRevision" jsonschema:"minimum=1"`
	Project          string `json:"project" jsonschema:"minLength=1"`
	TargetProject    string `json:"targetProject" jsonschema:"minLength=1"`
}

type SearchInput struct {
	Query   string `json:"query" jsonschema:"minLength=1"`
	Limit   int    `json:"limit" jsonschema:"minimum=1"`
	Project string `json:"project,omitzero"`
}

func (SearchInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"query": {MaxLength: new(uint64(knowledge.MaxQueryRunes))},
		"limit": {Maximum: new(float64(knowledge.MaxResults))},
	}}
}

type ToolSearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1"`
	Limit int    `json:"limit" jsonschema:"minimum=1"`
}

func (ToolSearchInput) JSONSchemaConstraints() jsonschema.Constraints {
	return SearchInput{}.JSONSchemaConstraints()
}
