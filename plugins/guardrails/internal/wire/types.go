package wire

import (
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	"github.com/invopop/jsonschema"
)

type ReplaceInput struct {
	ExpectedRevision int64    `json:"expectedRevision" jsonschema:"minimum=1"`
	Phrases          []string `json:"phrases"`
}

func (ReplaceInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	phrases, _ := schema.Properties.Get("phrases")
	phrases.MaxItems = new(uint64(guardrail.MaxPhrases))
}
