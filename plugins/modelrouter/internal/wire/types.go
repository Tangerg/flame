package wire

import (
	"github.com/Tangerg/flame/plugins/modelrouter/internal/routing"
	"github.com/Tangerg/scope/core/jsonschema"
)

type ReplaceInput struct {
	ExpectedRevision int64          `json:"expectedRevision" jsonschema:"minimum=1"`
	Rules            []routing.Rule `json:"rules"`
}

func (ReplaceInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"rules": {MaxItems: new(uint64(routing.MaxRules))},
	}}
}
