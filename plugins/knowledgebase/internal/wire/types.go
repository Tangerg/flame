package wire

import (
	"strconv"

	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/invopop/jsonschema"
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

func (ReadInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	if id, ok := schema.Properties.Get("id"); ok {
		id.MaxLength = new(uint64(knowledge.MaxIDRunes))
	}
}

type DocumentRead struct {
	Document knowledge.Header `json:"document"`
	Text     string           `json:"text"`
}

func (DocumentInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	if id, ok := schema.Properties.Get("id"); ok {
		id.MaxLength = new(uint64(knowledge.MaxIDRunes))
	}
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1"`
	Limit int    `json:"limit" jsonschema:"minimum=1"`
}

func (SearchInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	if query, ok := schema.Properties.Get("query"); ok {
		query.MaxLength = new(uint64(knowledge.MaxQueryRunes))
	}
	if limit, ok := schema.Properties.Get("limit"); ok {
		limit.Maximum = jsonvalue.Number(strconv.Itoa(knowledge.MaxResults))
	}
}
