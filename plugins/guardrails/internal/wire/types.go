package wire

import (
	"github.com/Tangerg/flame/plugins/guardrails/internal/guardrail"
	"github.com/Tangerg/scope/core/jsonschema"
)

type ReplaceInput struct {
	ExpectedRevision int64    `json:"expectedRevision" jsonschema:"minimum=1"`
	Phrases          []string `json:"phrases"`
}

func (ReplaceInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"phrases": {MaxItems: new(uint64(guardrail.MaxPhrases))},
	}}
}
